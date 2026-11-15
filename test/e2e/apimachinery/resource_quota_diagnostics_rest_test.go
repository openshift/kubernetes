/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apimachinery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	crdclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/utils/crd"
)

// These tests exercise the production discovery adapter and the production wiring
// against a local HTTP server, with the real client-go REST client, the real discovery
// decoder and the real typed clients. Decoding, error mapping, the retry ceiling and
// the context bounds are therefore the ones that run in CI. No cluster is contacted.

const (
	// quotaDiagRESTSlack is the ceiling used when asserting that a call returned
	// promptly. It is deliberately loose: each assertion is about the contrast with the
	// hour scale delay the server asks for, not about millisecond accuracy.
	quotaDiagRESTSlack = 5 * time.Second
	// quotaDiagRESTAmpleTimeout is for the contexts of calls that are expected to
	// finish, so a deadline is never the thing under test.
	quotaDiagRESTAmpleTimeout = time.Minute
)

// newQuotaDiagRecordingServer starts an httptest server that records the path of every
// request it serves. Handlers never touch *testing.T: they run on their own goroutines
// and may outlive the test that registered them.
func newQuotaDiagRecordingServer(t *testing.T, handle http.HandlerFunc) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		handle(w, r)
	}))
	// Cleanups run last in first out, so client connections are dropped before Close
	// waits for handlers: a handler parked on its request context is released rather
	// than deadlocking the teardown.
	t.Cleanup(server.Close)
	t.Cleanup(server.CloseClientConnections)

	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(paths)
	}
}

// newQuotaDiagRESTGetter hands back the production adapter, wired the way
// recordCustomResourceQuotaUsageFailure wires it: clientset.Discovery().RESTClient().
func newQuotaDiagRESTGetter(t *testing.T, handle http.HandlerFunc) (restAPIResourceGetter, func() []string) {
	t.Helper()
	server, requestedPaths := newQuotaDiagRecordingServer(t, handle)
	clients, err := clientset.NewForConfig(&restclient.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("failed to build a client for %s: %v", server.URL, err)
	}
	return restAPIResourceGetter{client: clients.Discovery().RESTClient()}, requestedPaths
}

// quotaDiagWithin runs call on its own goroutine and fails by name if it does not
// return in time, so a request that escapes its context produces a named failure here
// instead of a go test timeout that condemns the whole package.
func quotaDiagWithin(t *testing.T, what string, call func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		return err
	case <-time.After(quotaDiagRESTSlack):
		t.Fatalf("%s did not return within %s, it is not bound by its context", what, quotaDiagRESTSlack)
		return nil
	}
}

// quotaDiagServeJSON replies with body the way an API server replies.
func quotaDiagServeJSON(code int, body []byte, header map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for name, value := range header {
			w.Header().Set(name, value)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		// The handler runs on its own goroutine, so a write failure is ignored rather
		// than reported through *testing.T; the assertions cover the client side.
		_, _ = w.Write(body)
	}
}

// quotaDiagDiscoveryBody renders a group/version discovery document.
func quotaDiagDiscoveryBody(t *testing.T, resources ...metav1.APIResource) []byte {
	t.Helper()
	list := quotaDiagTestAPIResources(resources...)
	list.TypeMeta = metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"}
	return quotaDiagMarshal(t, list)
}

// quotaDiagStatusBody renders an API error the way an API server does.
func quotaDiagStatusBody(t *testing.T, code int, reason metav1.StatusReason, message string) []byte {
	t.Helper()
	return quotaDiagMarshal(t, &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Code:     int32(code),
		Reason:   reason,
		Message:  message,
	})
}

func quotaDiagMarshal(t *testing.T, object interface{}) []byte {
	t.Helper()
	body, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("failed to marshal %T: %v", object, err)
	}
	return body
}

// quotaDiagServeThrottled replies the way an API server under priority and fairness
// throttling does, asking the client to come back after retryAfter seconds.
func quotaDiagServeThrottled(retryAfter string) http.HandlerFunc {
	return quotaDiagServeJSON(http.StatusTooManyRequests, nil, map[string]string{"Retry-After": retryAfter})
}

// quotaDiagServeBlocked never answers until the client gives up.
func quotaDiagServeBlocked() http.HandlerFunc {
	return func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}
}

// TestRESTAPIResourceGetterDecodesDiscoveryDocument proves the adapter reaches the
// right path and that the discovery client's decoder turns the response into an
// APIResourceList. This is the part of the adapter that no fake can cover.
func TestRESTAPIResourceGetterDecodesDiscoveryDocument(t *testing.T) {
	other := metav1.APIResource{Name: "others", Kind: "Other", Namespaced: true, Verbs: metav1.Verbs{"get"}}
	getter, requestedPaths := newQuotaDiagRESTGetter(t,
		quotaDiagServeJSON(http.StatusOK, quotaDiagDiscoveryBody(t, other, quotaDiagThingResource()), nil))

	ctx, cancel := context.WithTimeout(context.Background(), quotaDiagRESTAmpleTimeout)
	defer cancel()
	got, err := getter.GetAPIResources(ctx, quotaDiagGV)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := []string{"/apis/" + quotaDiagGV}; !slices.Equal(requestedPaths(), want) {
		t.Errorf("got requests %v, want exactly %v", requestedPaths(), want)
	}
	if got.GroupVersion != quotaDiagGV {
		t.Errorf("got groupVersion %q, want %q", got.GroupVersion, quotaDiagGV)
	}
	if want := []metav1.APIResource{other, quotaDiagThingResource()}; !reflect.DeepEqual(got.APIResources, want) {
		t.Errorf("got resources %+v, want %+v", got.APIResources, want)
	}
}

// TestLogGroupVersionDiscoveryOverREST runs the diagnostic step CI runs, with the
// production adapter instead of a fake, and checks the line it writes.
func TestLogGroupVersionDiscoveryOverREST(t *testing.T) {
	getter, requestedPaths := newQuotaDiagRESTGetter(t,
		quotaDiagServeJSON(http.StatusOK, quotaDiagDiscoveryBody(t, quotaDiagThingResource()), nil))
	recorder := &quotaDiagLog{}
	diagnostics := customResourceQuotaDiagnostics{
		logf:           recorder.logf,
		apiResources:   getter,
		budget:         customResourceQuotaDiagnosticsBudget,
		requestTimeout: customResourceQuotaDiagnosticsRequestTimeout,
	}

	ctx, cancel := context.WithTimeout(context.Background(), customResourceQuotaDiagnosticsBudget)
	defer cancel()
	diagnostics.logGroupVersionDiscovery(ctx, quotaDiagGV, quotaDiagPlural)

	quotaDiagAssertContains(t, recorder.joined(),
		`discovery for "example.com/v1" lists "things": kind=Thing namespaced=true verbs=[get,list,watch]`)
	if want := []string{"/apis/" + quotaDiagGV}; !slices.Equal(requestedPaths(), want) {
		t.Errorf("got requests %v, want exactly %v", requestedPaths(), want)
	}
}

// TestRESTAPIResourceGetterPreservesServerErrors proves the adapter returns the API
// error as the client built it, so the reason and the server message survive into the
// diagnostic line.
func TestRESTAPIResourceGetterPreservesServerErrors(t *testing.T) {
	testCases := []struct {
		name    string
		code    int
		reason  metav1.StatusReason
		message string
		check   func(error) bool
	}{
		{
			name:    "not found",
			code:    http.StatusNotFound,
			reason:  metav1.StatusReasonNotFound,
			message: "the server could not find the requested resource",
			check:   apierrors.IsNotFound,
		},
		{
			name:    "forbidden",
			code:    http.StatusForbidden,
			reason:  metav1.StatusReasonForbidden,
			message: "discovery is forbidden",
			check:   apierrors.IsForbidden,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			getter, requestedPaths := newQuotaDiagRESTGetter(t, quotaDiagServeJSON(
				testCase.code, quotaDiagStatusBody(t, testCase.code, testCase.reason, testCase.message), nil))

			ctx, cancel := context.WithTimeout(context.Background(), quotaDiagRESTAmpleTimeout)
			defer cancel()
			got, err := getter.GetAPIResources(ctx, quotaDiagGV)

			if err == nil {
				t.Fatalf("expected an error, got %+v", got)
			}
			if got != nil {
				t.Errorf("expected no resource list alongside the error, got %+v", got)
			}
			if !testCase.check(err) {
				t.Errorf("the API error reason did not survive the adapter: %v", err)
			}
			if !strings.Contains(err.Error(), testCase.message) {
				t.Errorf("expected the server message %q in %v", testCase.message, err)
			}
			if len(requestedPaths()) != 1 {
				t.Errorf("got %d requests %v, want exactly one", len(requestedPaths()), requestedPaths())
			}
		})
	}
}

// TestRESTAPIResourceGetterCapsRetries proves MaxRetries(1) is in force: a throttled
// server that keeps asking for a retry is contacted twice and no more.
func TestRESTAPIResourceGetterCapsRetries(t *testing.T) {
	// Retry-After: 0 makes the attempts back to back, so the count is what is measured.
	getter, requestedPaths := newQuotaDiagRESTGetter(t, quotaDiagServeThrottled("0"))

	ctx, cancel := context.WithTimeout(context.Background(), quotaDiagRESTAmpleTimeout)
	defer cancel()
	if _, err := getter.GetAPIResources(ctx, quotaDiagGV); err == nil {
		t.Fatal("expected an error from a server that only throttles")
	}

	if paths := requestedPaths(); len(paths) != 2 {
		t.Errorf("got %d requests %v, want 2 (the initial attempt plus MaxRetries(1))", len(paths), paths)
	}
}

// TestRESTAPIResourceGetterCannotEscapeItsContext is the regression test for the reason
// this adapter exists. discovery.DiscoveryClient issues ServerResourcesForGroupVersion
// with context.TODO, so an uncapped Retry-After or an unanswered request would outlive
// the diagnostics budget and hold up the cleanup that follows the failed test. Going
// through the REST client with the caller's context must bound both.
func TestRESTAPIResourceGetterCannotEscapeItsContext(t *testing.T) {
	testCases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "server asks for an hour long retry", handler: quotaDiagServeThrottled("3600")},
		{name: "server never answers", handler: quotaDiagServeBlocked()},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			getter, _ := newQuotaDiagRESTGetter(t, testCase.handler)

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			err := quotaDiagWithin(t, "GetAPIResources", func() error {
				_, err := getter.GetAPIResources(ctx, quotaDiagGV)
				return err
			})

			if err == nil {
				t.Fatal("expected an error once the context expired")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("expected the context deadline to be the reason the call ended, got %v", err)
			}
		})
	}
}

// TestLogGroupVersionDiscoveryStaysInsideItsBounds proves that on the production path
// neither the overall budget nor the per request timeout can be overrun by a server
// that does not answer, whichever of the two is the tighter bound.
func TestLogGroupVersionDiscoveryStaysInsideItsBounds(t *testing.T) {
	// One second leaves ample room for the goroutine to reach the request before the
	// bound expires, while staying far below the slack the assertion allows.
	const bound = time.Second
	testCases := []struct {
		name           string
		budget         time.Duration
		requestTimeout time.Duration
	}{
		{name: "the budget is the tighter bound", budget: bound, requestTimeout: time.Hour},
		{name: "the per request timeout is the tighter bound", budget: time.Hour, requestTimeout: bound},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			getter, _ := newQuotaDiagRESTGetter(t, quotaDiagServeBlocked())
			recorder := &quotaDiagLog{}
			diagnostics := customResourceQuotaDiagnostics{
				logf:           recorder.logf,
				apiResources:   getter,
				budget:         testCase.budget,
				requestTimeout: testCase.requestTimeout,
			}

			budgetCtx, cancel := context.WithTimeout(context.Background(), testCase.budget)
			defer cancel()
			_ = quotaDiagWithin(t, "logGroupVersionDiscovery", func() error {
				diagnostics.logGroupVersionDiscovery(budgetCtx, quotaDiagGV, quotaDiagPlural)
				return nil
			})

			quotaDiagAssertContains(t, recorder.joined(),
				`discovery for "example.com/v1" failed:`, "context deadline exceeded")
		})
	}
}

// TestRecordCustomResourceQuotaUsageFailureWiring exercises the function the e2e
// callsite calls, with real typed clients pointed at a local server. It proves the
// wrapper reads the three intended objects, in order, and returns the original failure.
// It logs through framework.Logf, so the production diagnostic lines appear on stdout
// unattributed to any test; the assertions below are on the requests, not on that text.
func TestRecordCustomResourceQuotaUsageFailureWiring(t *testing.T) {
	const (
		crdPath       = "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/" + quotaDiagCRDName
		quotaPath     = "/api/v1/namespaces/" + quotaDiagNamespace + "/resourcequotas/" + quotaDiagQuotaName
		discoveryPath = "/apis/" + quotaDiagGV
	)
	definition := quotaDiagTestCRD()
	definition.TypeMeta = metav1.TypeMeta{Kind: "CustomResourceDefinition", APIVersion: "apiextensions.k8s.io/v1"}
	quota := quotaDiagTestQuota()
	quota.TypeMeta = metav1.TypeMeta{Kind: "ResourceQuota", APIVersion: "v1"}
	bodies := map[string][]byte{
		crdPath:       quotaDiagMarshal(t, definition),
		quotaPath:     quotaDiagMarshal(t, quota),
		discoveryPath: quotaDiagDiscoveryBody(t, quotaDiagThingResource()),
	}

	server, requestedPaths := newQuotaDiagRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		quotaDiagServeJSON(http.StatusOK, body, nil)(w, r)
	})

	config := &restclient.Config{Host: server.URL}
	clients, err := clientset.NewForConfig(config)
	if err != nil {
		t.Fatalf("failed to build a client: %v", err)
	}
	apiExtensionClients, err := crdclientset.NewForConfig(config)
	if err != nil {
		t.Fatalf("failed to build an apiextensions client: %v", err)
	}
	f := &framework.Framework{
		ClientSet: clients,
		Namespace: &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: quotaDiagNamespace}},
	}
	testcrd := &crd.TestCrd{APIExtensionClient: apiExtensionClients, Crd: quotaDiagTestCRD()}

	// A successful wait must not touch the API server at all.
	if got := recordCustomResourceQuotaUsageFailure(context.Background(), f, testcrd, quotaDiagQuotaName, quotaDiagResourceName, nil); got != nil {
		t.Fatalf("expected nil for a successful wait, got %v", got)
	}
	if requested := requestedPaths(); len(requested) != 0 {
		t.Fatalf("expected no requests on success, got %v", requested)
	}

	// A failed wait reads the CRD, the temporary quota and discovery, in that order,
	// even though the caller's context is already done.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := recordCustomResourceQuotaUsageFailure(ctx, f, testcrd, quotaDiagQuotaName, quotaDiagResourceName, errTestQuotaUsage); got != errTestQuotaUsage {
		t.Fatalf("expected the original cause to be returned unchanged, got %v", got)
	}
	if want := []string{crdPath, quotaPath, discoveryPath}; !slices.Equal(requestedPaths(), want) {
		t.Errorf("got requests %v, want exactly %v", requestedPaths(), want)
	}
}
