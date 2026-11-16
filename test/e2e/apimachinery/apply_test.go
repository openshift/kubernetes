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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

func TestCRDDeletionDiagnostic(t *testing.T) {
	t.Parallel()

	const crdName = "noxus.mygroup.example.com"

	deletionTimestamp := metav1.NewTime(time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC))
	terminating := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:              crdName,
			DeletionTimestamp: &deletionTimestamp,
			Finalizers:        []string{"customresourcecleanup.apiextensions.k8s.io"},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "mygroup.example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				// Spec content must never reach the diagnostic.
				Kind: "DiagnosticExcludedKind",
			},
		},
		Status: apiextensionsv1.CustomResourceDefinitionStatus{
			Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
				{
					Type:   apiextensionsv1.Established,
					Status: apiextensionsv1.ConditionTrue,
					Reason: "InitialNamesAccepted",
					// Free-form condition messages are unbounded and must
					// never reach the diagnostic.
					Message: "DiagnosticExcludedMessage",
				},
				{
					Type:    apiextensionsv1.Terminating,
					Status:  apiextensionsv1.ConditionTrue,
					Reason:  "InstanceDeletionInProgress",
					Message: "DiagnosticExcludedMessage",
				},
			},
		},
	}

	// A CustomResourceDefinition carrying no deletion metadata at all: no
	// deletion timestamp, no finalizers and no conditions.
	bare := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: crdName},
	}

	// Control characters in any reported field must not break the log line.
	withControlCharacters := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:       crdName,
			Finalizers: []string{"first\nfinalizer"},
		},
		Status: apiextensionsv1.CustomResourceDefinitionStatus{
			Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
				{
					Type:   apiextensionsv1.Terminating,
					Status: apiextensionsv1.ConditionTrue,
					Reason: "Bad\tReason\aAndBell",
				},
			},
		},
	}

	// Neither field has a length limit of its own.
	oversized := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:       crdName,
			Finalizers: oversizedFinalizers(64, 200),
		},
		Status: apiextensionsv1.CustomResourceDefinitionStatus{
			Conditions: oversizedConditions(64, 200),
		},
	}

	// Spec content and free-form condition messages must never be logged.
	dontWantSubstrs := []string{"DiagnosticExcludedKind", "DiagnosticExcludedMessage"}

	// The fake clientset does not honor context cancellation, so these cases
	// cover only how a retrieval error is rendered. That cancellation really
	// happens is proven by TestCRDDeletionDiagnosticHonorsContextDeadline.
	errorReactor := func(err error) k8stesting.ReactionFunc {
		return func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, err
		}
	}

	testCases := []struct {
		name        string
		objects     []runtime.Object
		reaction    k8stesting.ReactionFunc
		wantSubstrs []string
	}{
		{
			name:    "present with deletion metadata",
			objects: []runtime.Object{terminating},
			wantSubstrs: []string{
				crdName,
				"is present in the API after its deletion failed",
				"deletionTimestamp=2026-01-02 03:04:05 +0000 UTC",
				"customresourcecleanup.apiextensions.k8s.io",
				"Established=True(reason=InitialNamesAccepted)",
				"Terminating=True(reason=InstanceDeletionInProgress)",
			},
		},
		{
			name:    "no deletion metadata",
			objects: []runtime.Object{bare},
			wantSubstrs: []string{
				crdName,
				"is present in the API after its deletion failed",
				"deletionTimestamp=<nil>",
				"finalizers=[]",
				"conditions=[]",
			},
		},
		{
			name: "absent from the API",
			wantSubstrs: []string{
				crdName,
				"is not present in the API after its deletion failed",
			},
		},
		{
			name:    "control characters escaped",
			objects: []runtime.Object{withControlCharacters},
			wantSubstrs: []string{
				crdName,
				`first\nfinalizer`,
				`Bad\tReason\aAndBell`,
			},
		},
		{
			name:     "error text with control characters escaped",
			reaction: errorReactor(errors.New("first line\nsecond line")),
			wantSubstrs: []string{
				crdName,
				`first line\nsecond line`,
			},
		},
		{
			name:    "oversized state truncated",
			objects: []runtime.Object{oversized},
			wantSubstrs: []string{
				// The leading text survives truncation.
				"CustomResourceDefinition " + crdName + " is present in the API",
				"[truncated]",
			},
		},
		{
			name:     "oversized error text truncated",
			reaction: errorReactor(errors.New(strings.Repeat("e", 2*crdDeletionDiagnosticMaxBytes))),
			wantSubstrs: []string{
				"CustomResourceDefinition " + crdName + " state is unavailable",
				"[truncated]",
			},
		},
		{
			name:     "retrieval error",
			reaction: errorReactor(apierrors.NewForbidden(schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"}, crdName, errors.New("insufficient permissions"))),
			wantSubstrs: []string{
				crdName,
				"state is unavailable after its deletion failed",
				"forbidden",
			},
		},
		{
			name:     "retrieval timed out",
			reaction: errorReactor(context.DeadlineExceeded),
			wantSubstrs: []string{
				crdName,
				"state is unavailable after its deletion failed",
				"context deadline exceeded",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := apiextensionsfake.NewSimpleClientset(testCase.objects...)
			if testCase.reaction != nil {
				client.PrependReactor("get", "customresourcedefinitions", testCase.reaction)
			}

			got := crdDeletionDiagnostic(context.Background(), client, crdName)

			for _, want := range testCase.wantSubstrs {
				if !strings.Contains(got, want) {
					t.Errorf("diagnostic %q does not contain %q", got, want)
				}
			}
			for _, dontWant := range dontWantSubstrs {
				if strings.Contains(got, dontWant) {
					t.Errorf("diagnostic %q leaked CustomResourceDefinition content %q", got, dontWant)
				}
			}
			if strings.ContainsFunc(got, unicode.IsControl) {
				t.Errorf("diagnostic %q contains unescaped control characters", got)
			}
			if len(got) > crdDeletionDiagnosticMaxBytes {
				t.Errorf("diagnostic is %d bytes, want at most %d", len(got), crdDeletionDiagnosticMaxBytes)
			}
			if !utf8.ValidString(got) {
				t.Errorf("diagnostic %q is not valid UTF-8", got)
			}
		})
	}
}

func oversizedFinalizers(count, length int) []string {
	finalizers := make([]string, 0, count)
	for i := range count {
		finalizers = append(finalizers, fmt.Sprintf("%d.%s", i, strings.Repeat("f", length)))
	}
	return finalizers
}

func oversizedConditions(count, reasonLength int) []apiextensionsv1.CustomResourceDefinitionCondition {
	conditions := make([]apiextensionsv1.CustomResourceDefinitionCondition, 0, count)
	for i := range count {
		conditions = append(conditions, apiextensionsv1.CustomResourceDefinitionCondition{
			Type:   apiextensionsv1.Terminating,
			Status: apiextensionsv1.ConditionTrue,
			Reason: fmt.Sprintf("%d%s", i, strings.Repeat("R", reasonLength)),
		})
	}
	return conditions
}

// TestCRDDeletionDiagnosticHonorsContextDeadline proves that an expiring
// context actually cancels the diagnostic request, rather than only that an
// injected deadline error is rendered. It uses a local HTTP server and a real
// apiextensions client; no cluster is involved.
func TestCRDDeletionDiagnosticHonorsContextDeadline(t *testing.T) {
	t.Parallel()

	// Closed on the first request, so the test can prove the server was
	// actually reached before the client gave up.
	served := make(chan struct{})
	closeServed := sync.OnceFunc(func() { close(served) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer closeServed()
		// Answer only once the client has given up, so the diagnostic can
		// return only by honoring its context.
		<-req.Context().Done()
	}))
	defer server.Close()

	client, err := apiextensionsclientset.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	diagnostics := make(chan string, 1)
	go func() {
		diagnostics <- crdDeletionDiagnostic(ctx, client, "noxus.mygroup.example.com")
	}()

	var got string
	select {
	case got = <-diagnostics:
	case <-time.After(30 * time.Second):
		t.Fatal("the diagnostic never returned; it did not honor the context deadline")
	}

	if !strings.Contains(got, "state is unavailable after its deletion failed") {
		t.Errorf("diagnostic %q does not report the failed retrieval", got)
	}
	if !strings.Contains(got, "context deadline exceeded") {
		t.Errorf("diagnostic %q does not report the deadline", got)
	}

	select {
	case <-served:
	case <-time.After(30 * time.Second):
		t.Fatal("the request never reached the server, so nothing was cancelled in flight")
	}
}

// TestLogCRDDeletionDiagnosticUsesFreshDeadline proves that the production
// call path issues its request under its own finite deadline derived from
// crdDeletionDiagnosticTimeout, without waiting for that deadline to expire.
func TestLogCRDDeletionDiagnosticUsesFreshDeadline(t *testing.T) {
	t.Parallel()

	var (
		sawRequest  bool
		deadline    time.Time
		hasDeadline bool
	)
	client, err := apiextensionsclientset.NewForConfig(&rest.Config{
		Host: "https://192.0.2.1:6443",
		WrapTransport: func(http.RoundTripper) http.RoundTripper {
			return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				sawRequest = true
				deadline, hasDeadline = req.Context().Deadline()
				return nil, errors.New("transport not used")
			})
		},
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	before := time.Now()
	logCRDDeletionDiagnostic(client, "noxus.mygroup.example.com")

	if !sawRequest {
		t.Fatal("no request was issued")
	}
	if !hasDeadline {
		t.Fatal("the diagnostic request had no deadline")
	}
	// The deadline is measured from just before the call, so allow a small
	// slack for the time spent setting the request up.
	const slack = time.Second
	if remaining := deadline.Sub(before); remaining <= 0 || remaining > crdDeletionDiagnosticTimeout+slack {
		t.Errorf("diagnostic request deadline is %v from the call, want a fresh deadline in (0, %v]", remaining, crdDeletionDiagnosticTimeout+slack)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
