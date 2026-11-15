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
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	quotaDiagCRDName    = "e2e-test-things-crd"
	quotaDiagPlural     = "things"
	quotaDiagGroup      = "example.com"
	quotaDiagGV         = quotaDiagGroup + "/v1"
	quotaDiagNamespace  = "e2e-resourcequota-1234"
	quotaDiagQuotaName  = "quota-for-things"
	quotaDiagCountQuota = "count/things.example.com"
	// quotaDiagSentinel is planted in every object field that must never be logged.
	quotaDiagSentinel = "do-not-log-me"
)

var quotaDiagResourceName = v1.ResourceName(quotaDiagCountQuota)

// quotaDiagRequest captures how a diagnostic request was issued, so tests can assert
// the bounds that were applied to it.
type quotaDiagRequest struct {
	name        string
	hasDeadline bool
	deadline    time.Time
	ctxErr      error
}

func quotaDiagRecord(ctx context.Context, name string) quotaDiagRequest {
	deadline, hasDeadline := ctx.Deadline()
	return quotaDiagRequest{name: name, hasDeadline: hasDeadline, deadline: deadline, ctxErr: ctx.Err()}
}

type quotaDiagFakeCRDs struct {
	definition *apiextensionsv1.CustomResourceDefinition
	err        error
	beforeGet  func()
	requests   []quotaDiagRequest
}

func (f *quotaDiagFakeCRDs) Get(ctx context.Context, name string, _ metav1.GetOptions) (*apiextensionsv1.CustomResourceDefinition, error) {
	f.requests = append(f.requests, quotaDiagRecord(ctx, name))
	if f.beforeGet != nil {
		f.beforeGet()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.definition, nil
}

type quotaDiagFakeQuotas struct {
	quota    *v1.ResourceQuota
	err      error
	requests []quotaDiagRequest
}

func (f *quotaDiagFakeQuotas) Get(ctx context.Context, name string, _ metav1.GetOptions) (*v1.ResourceQuota, error) {
	f.requests = append(f.requests, quotaDiagRecord(ctx, name))
	if f.err != nil {
		return nil, f.err
	}
	return f.quota, nil
}

type quotaDiagFakeAPIResources struct {
	resources *metav1.APIResourceList
	err       error
	requests  []quotaDiagRequest
}

func (f *quotaDiagFakeAPIResources) GetAPIResources(ctx context.Context, groupVersion string) (*metav1.APIResourceList, error) {
	f.requests = append(f.requests, quotaDiagRecord(ctx, groupVersion))
	if f.err != nil {
		return nil, f.err
	}
	return f.resources, nil
}

// quotaDiagLog collects the formatted diagnostic lines.
type quotaDiagLog struct {
	lines []string
}

func (l *quotaDiagLog) logf(format string, args ...interface{}) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *quotaDiagLog) joined() string {
	return strings.Join(l.lines, "\n")
}

type quotaDiagFixture struct {
	log          *quotaDiagLog
	crds         *quotaDiagFakeCRDs
	quotas       *quotaDiagFakeQuotas
	apiResources *quotaDiagFakeAPIResources
}

// newQuotaDiagFixture wires healthy fakes; individual tests override what they exercise.
func newQuotaDiagFixture() *quotaDiagFixture {
	return &quotaDiagFixture{
		log:          &quotaDiagLog{},
		crds:         &quotaDiagFakeCRDs{definition: quotaDiagTestCRD()},
		quotas:       &quotaDiagFakeQuotas{quota: quotaDiagTestQuota()},
		apiResources: &quotaDiagFakeAPIResources{resources: quotaDiagTestAPIResources(quotaDiagThingResource())},
	}
}

func (f *quotaDiagFixture) diagnostics() customResourceQuotaDiagnostics {
	return customResourceQuotaDiagnostics{
		logf:           f.log.logf,
		crds:           f.crds,
		quotas:         f.quotas,
		apiResources:   f.apiResources,
		budget:         customResourceQuotaDiagnosticsBudget,
		requestTimeout: customResourceQuotaDiagnosticsRequestTimeout,
	}
}

func (f *quotaDiagFixture) requestCounts() (int, int, int) {
	return len(f.crds.requests), len(f.quotas.requests), len(f.apiResources.requests)
}

func quotaDiagTarget() customResourceQuotaDiagnosticsTarget {
	return newCustomResourceQuotaDiagnosticsTarget(quotaDiagTestCRD(), quotaDiagNamespace, quotaDiagQuotaName, quotaDiagResourceName)
}

// quotaDiagTestCRD plants quotaDiagSentinel in every field that must stay out of the
// log, so the "no arbitrary object content" assertions are load bearing.
func quotaDiagTestCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:              quotaDiagCRDName,
			UID:               "crd-uid",
			ResourceVersion:   "4242",
			Generation:        1,
			CreationTimestamp: metav1.Date(2026, 11, 15, 1, 15, 22, 0, time.UTC),
			Annotations:       map[string]string{"example.com/token": quotaDiagSentinel},
			Labels:            map[string]string{"example.com/owner": quotaDiagSentinel},
			Finalizers:        []string{"example.com/" + quotaDiagSentinel},
			ManagedFields:     []metav1.ManagedFieldsEntry{{Manager: quotaDiagSentinel}},
			OwnerReferences:   []metav1.OwnerReference{{Name: quotaDiagSentinel, UID: types.UID(quotaDiagSentinel)}},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: quotaDiagGroup,
			Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: quotaDiagPlural, Kind: "Thing"},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name: "v1", Served: true, Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object", Description: quotaDiagSentinel},
					},
				},
			},
		},
		Status: apiextensionsv1.CustomResourceDefinitionStatus{
			AcceptedNames: apiextensionsv1.CustomResourceDefinitionNames{Plural: quotaDiagPlural},
			Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
				{
					Type:               apiextensionsv1.NamesAccepted,
					Status:             apiextensionsv1.ConditionTrue,
					Reason:             "NoConflicts",
					Message:            "no conflicts found",
					LastTransitionTime: metav1.Date(2026, 11, 15, 1, 15, 22, 0, time.UTC),
				},
				{
					Type:               apiextensionsv1.Established,
					Status:             apiextensionsv1.ConditionTrue,
					Reason:             "InitialNamesAccepted",
					Message:            "the initial names have been accepted",
					LastTransitionTime: metav1.Date(2026, 11, 15, 1, 15, 23, 0, time.UTC),
				},
			},
		},
	}
}

func quotaDiagTestQuota() *v1.ResourceQuota {
	return &v1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:              quotaDiagQuotaName,
			Namespace:         quotaDiagNamespace,
			UID:               "quota-uid",
			ResourceVersion:   "5150",
			Generation:        7,
			CreationTimestamp: metav1.Date(2026, 11, 15, 1, 15, 24, 0, time.UTC),
			Annotations:       map[string]string{"example.com/token": quotaDiagSentinel},
			Labels:            map[string]string{"example.com/owner": quotaDiagSentinel},
			ManagedFields:     []metav1.ManagedFieldsEntry{{Manager: quotaDiagSentinel}},
		},
		Spec: v1.ResourceQuotaSpec{
			Hard: v1.ResourceList{quotaDiagResourceName: resource.MustParse("7")},
			ScopeSelector: &v1.ScopeSelector{MatchExpressions: []v1.ScopedResourceSelectorRequirement{
				{ScopeName: v1.ResourceQuotaScopePriorityClass, Operator: v1.ScopeSelectorOpIn, Values: []string{quotaDiagSentinel}},
			}},
		},
		Status: v1.ResourceQuotaStatus{
			Hard: v1.ResourceList{v1.ResourcePods: resource.MustParse("10")},
			Used: v1.ResourceList{v1.ResourcePods: resource.MustParse("0")},
		},
	}
}

func quotaDiagThingResource() metav1.APIResource {
	return metav1.APIResource{Name: quotaDiagPlural, Kind: "Thing", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "watch"}}
}

func quotaDiagTestAPIResources(resources ...metav1.APIResource) *metav1.APIResourceList {
	return &metav1.APIResourceList{GroupVersion: quotaDiagGV, APIResources: resources}
}

func quotaDiagAssertContains(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, substring := range want {
		if !strings.Contains(got, substring) {
			t.Errorf("expected diagnostics to contain %q, got:\n%s", substring, got)
		}
	}
}

func quotaDiagAssertNotContains(t *testing.T, got string, unwanted ...string) {
	t.Helper()
	for _, substring := range unwanted {
		if strings.Contains(got, substring) {
			t.Errorf("expected diagnostics not to contain %q, got:\n%s", substring, got)
		}
	}
}

func quotaDiagAssertRequestCounts(t *testing.T, fixture *quotaDiagFixture, wantCRDs, wantQuotas, wantDiscovery int) {
	t.Helper()
	crds, quotas, discovery := fixture.requestCounts()
	if crds != wantCRDs || quotas != wantQuotas || discovery != wantDiscovery {
		t.Errorf("requests: got crd=%d quota=%d discovery=%d, want crd=%d quota=%d discovery=%d",
			crds, quotas, discovery, wantCRDs, wantQuotas, wantDiscovery)
	}
}

// TestRecordFailureIsFailureOnly asserts that a successful wait collects nothing at all.
func TestRecordFailureIsFailureOnly(t *testing.T) {
	fixture := newQuotaDiagFixture()

	if err := fixture.diagnostics().recordFailure(context.Background(), quotaDiagTarget(), nil); err != nil {
		t.Fatalf("expected nil error for a successful wait, got %v", err)
	}
	if len(fixture.log.lines) != 0 {
		t.Errorf("expected no diagnostics on success, got:\n%s", fixture.log.joined())
	}
	quotaDiagAssertRequestCounts(t, fixture, 0, 0, 0)
}

// TestRecordFailurePreservesCauseAndSurvivesCanceledContext asserts that the original
// failure is returned untouched and that collection still runs when the caller's
// context is already done, for example because the spec itself ran out of time.
func TestRecordFailurePreservesCauseAndSurvivesCanceledContext(t *testing.T) {
	fixture := newQuotaDiagFixture()
	cause := fmt.Errorf("wrapped: %w", context.DeadlineExceeded)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	got := fixture.diagnostics().recordFailure(ctx, quotaDiagTarget(), cause)
	end := time.Now()

	if got != cause {
		t.Fatalf("expected the original cause to be returned unchanged, got %v", got)
	}
	quotaDiagAssertRequestCounts(t, fixture, 1, 1, 1)

	requests := append(append(append([]quotaDiagRequest{}, fixture.crds.requests...), fixture.quotas.requests...), fixture.apiResources.requests...)
	for _, request := range requests {
		if request.ctxErr != nil {
			t.Errorf("request %q saw a done context %v, diagnostics must outlive the caller's cancellation", request.name, request.ctxErr)
		}
		if !request.hasDeadline {
			t.Fatalf("request %q had no deadline, diagnostics must stay bounded", request.name)
		}
		// The deadline is set between start and end, so this closed window fails both a
		// shorter and a longer per request bound, and fails if a request were handed the
		// whole budget instead.
		if lower := start.Add(customResourceQuotaDiagnosticsRequestTimeout); request.deadline.Before(lower) {
			t.Errorf("request %q deadline %s is before the per request bound %s", request.name, request.deadline, lower)
		}
		if upper := end.Add(customResourceQuotaDiagnosticsRequestTimeout); request.deadline.After(upper) {
			t.Errorf("request %q deadline %s is past the per request bound %s", request.name, request.deadline, upper)
		}
	}
	quotaDiagAssertContains(t, fixture.log.joined(),
		fmt.Sprintf("ResourceQuota %s/%s never reported usage", quotaDiagNamespace, quotaDiagQuotaName),
		cause.Error())
}

// TestRecordFailureClampsRequestsToTheBudget asserts that the overall budget, not only
// the per request timeout, bounds the collection.
func TestRecordFailureClampsRequestsToTheBudget(t *testing.T) {
	fixture := newQuotaDiagFixture()
	diagnostics := fixture.diagnostics()
	// A budget shorter than one request forces the requests to inherit it.
	diagnostics.budget = time.Second
	diagnostics.requestTimeout = time.Hour

	start := time.Now()
	if got := diagnostics.recordFailure(context.Background(), quotaDiagTarget(), errTestQuotaUsage); got != errTestQuotaUsage {
		t.Fatalf("expected the original cause to be returned unchanged, got %v", got)
	}
	quotaDiagAssertRequestCounts(t, fixture, 1, 1, 1)

	for _, request := range fixture.crds.requests {
		if upper := start.Add(2 * time.Second); request.deadline.After(upper) {
			t.Errorf("request %q deadline %s escapes the %s budget", request.name, request.deadline, diagnostics.budget)
		}
	}
}

// TestRecordFailureStopsOnceTheBudgetIsSpent asserts that an exhausted budget issues no
// requests at all, even though the per request timeout is generous.
func TestRecordFailureStopsOnceTheBudgetIsSpent(t *testing.T) {
	fixture := newQuotaDiagFixture()
	diagnostics := fixture.diagnostics()
	diagnostics.budget = time.Nanosecond

	if got := diagnostics.recordFailure(context.Background(), quotaDiagTarget(), errTestQuotaUsage); got != errTestQuotaUsage {
		t.Fatalf("expected the original cause to be returned unchanged, got %v", got)
	}
	quotaDiagAssertRequestCounts(t, fixture, 0, 0, 0)
	quotaDiagAssertContains(t, fixture.log.joined(), "diagnostics budget is spent: context deadline exceeded")
}

var errTestQuotaUsage = errors.New("timed out waiting for quota usage")

// TestCollectLogsScopedState asserts the snapshot carries exactly the facts triage needs
// and none of the object content that must never be logged.
func TestCollectLogsScopedState(t *testing.T) {
	fixture := newQuotaDiagFixture()
	ctx, cancel := context.WithTimeout(context.Background(), customResourceQuotaDiagnosticsBudget)
	defer cancel()

	fixture.diagnostics().collect(ctx, quotaDiagTarget())

	got := fixture.log.joined()
	quotaDiagAssertContains(t, got,
		// CRD identity and the two conditions that gate servability.
		`uid=crd-uid resourceVersion=4242 generation=1 creationTimestamp=2026-11-15T01:15:22Z`,
		`group=example.com plural=things kind=Thing scope=Namespaced acceptedPlural=things`,
		`versions=[v1(served=true,storage=true)]`,
		`NamesAccepted=True(reason="NoConflicts"`,
		`Established=True(reason="InitialNamesAccepted"`,
		`lastTransitionTime=2026-11-15T01:15:23Z`,
		// Quota spec, status and the metadata that shows how far the update loop got.
		`uid=quota-uid resourceVersion=5150 generation=7 creationTimestamp=2026-11-15T01:15:24Z`,
		`spec.hard={count/things.example.com=7} status.hard={pods=10} status.used={pods=0}`,
		`"count/things.example.com" present in spec.hard=true status.hard=false status.used=false`,
		// Targeted discovery visibility.
		`discovery for "example.com/v1" lists "things": kind=Thing namespaced=true verbs=[get,list,watch]`,
	)
	quotaDiagAssertNotContains(t, got, quotaDiagSentinel, "example.com/owner", "example.com/token")
	quotaDiagAssertRequestCounts(t, fixture, 1, 1, 1)

	for _, line := range fixture.log.lines {
		if !strings.HasPrefix(line, customResourceQuotaDiagnosticsPrefix) {
			t.Errorf("every diagnostic line must start with %q so the snapshot can be grepped, got %q",
				customResourceQuotaDiagnosticsPrefix, line)
		}
	}
	if groupVersion := fixture.apiResources.requests[0].name; groupVersion != quotaDiagGV {
		t.Errorf("expected discovery to be asked only for %q, got %q", quotaDiagGV, groupVersion)
	}
}

// TestCollectReportsMissingResources asserts that an object which is already gone is
// reported rather than silently skipped, and that the later steps still run.
func TestCollectReportsMissingResources(t *testing.T) {
	fixture := newQuotaDiagFixture()
	fixture.crds.err = apierrors.NewNotFound(
		schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"}, quotaDiagCRDName)
	fixture.quotas.err = apierrors.NewNotFound(
		schema.GroupResource{Resource: "resourcequotas"}, quotaDiagQuotaName)
	fixture.apiResources.resources = quotaDiagTestAPIResources()

	ctx, cancel := context.WithTimeout(context.Background(), customResourceQuotaDiagnosticsBudget)
	defer cancel()
	fixture.diagnostics().collect(ctx, quotaDiagTarget())

	quotaDiagAssertContains(t, fixture.log.joined(),
		`failed to read CustomResourceDefinition "e2e-test-things-crd": customresourcedefinitions.apiextensions.k8s.io "e2e-test-things-crd" not found`,
		`failed to read ResourceQuota e2e-resourcequota-1234/quota-for-things: resourcequotas "quota-for-things" not found`,
		`discovery for "example.com/v1" does not list "things" (0 resources served)`,
	)
	quotaDiagAssertRequestCounts(t, fixture, 1, 1, 1)
}

// TestCollectReportsRequestErrors asserts that transport level errors are logged, never
// discarded, and that a failing step does not stop the ones after it.
func TestCollectReportsRequestErrors(t *testing.T) {
	fixture := newQuotaDiagFixture()
	fixture.crds.err = errors.New("crd boom")
	fixture.quotas.err = errors.New("quota boom")
	fixture.apiResources.err = errors.New("discovery boom")

	ctx, cancel := context.WithTimeout(context.Background(), customResourceQuotaDiagnosticsBudget)
	defer cancel()
	fixture.diagnostics().collect(ctx, quotaDiagTarget())

	quotaDiagAssertContains(t, fixture.log.joined(),
		"crd boom", "quota boom", `discovery for "example.com/v1" failed: discovery boom`)
	quotaDiagAssertRequestCounts(t, fixture, 1, 1, 1)
}

// TestCollectStopsWhenBudgetIsSpentMidway asserts the remaining steps are abandoned as
// soon as the budget runs out, while what was already collected is still reported.
func TestCollectStopsWhenBudgetIsSpentMidway(t *testing.T) {
	const budget = 50 * time.Millisecond
	fixture := newQuotaDiagFixture()
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	// Spend the budget while the first step is in flight.
	fixture.crds.beforeGet = func() { time.Sleep(4 * budget) }

	fixture.diagnostics().collect(ctx, quotaDiagTarget())

	quotaDiagAssertRequestCounts(t, fixture, 1, 0, 0)
	got := fixture.log.joined()
	quotaDiagAssertContains(t, got,
		// What was already collected survives.
		`CustomResourceDefinition "e2e-test-things-crd" identity: uid=crd-uid`,
		`skipping ResourceQuota e2e-resourcequota-1234/quota-for-things, diagnostics budget is spent`,
		`skipping discovery for "example.com/v1", diagnostics budget is spent`,
	)
}

// TestCollectStopsWhenBudgetIsSpent asserts that collection issues nothing at all once
// the budget is gone, and says why for every step.
func TestCollectStopsWhenBudgetIsSpent(t *testing.T) {
	fixture := newQuotaDiagFixture()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	fixture.diagnostics().collect(ctx, quotaDiagTarget())

	quotaDiagAssertRequestCounts(t, fixture, 0, 0, 0)
	quotaDiagAssertContains(t, fixture.log.joined(),
		`skipping CustomResourceDefinition "e2e-test-things-crd", diagnostics budget is spent: context deadline exceeded`,
		`skipping ResourceQuota e2e-resourcequota-1234/quota-for-things, diagnostics budget is spent`,
		`skipping discovery for "example.com/v1", diagnostics budget is spent`,
	)
}

// TestCollectDiscoveryVisibility covers the shapes the discovery document can have for
// the custom resource.
func TestCollectDiscoveryVisibility(t *testing.T) {
	other := metav1.APIResource{Name: "others", Kind: "Other", Namespaced: true, Verbs: metav1.Verbs{"get"}}
	// A resource whose kind matches but whose name does not must not be mistaken for it.
	kindOnly := metav1.APIResource{Name: "thingies", Kind: "Thing", Namespaced: true, Verbs: metav1.Verbs{"get"}}

	testCases := []struct {
		name         string
		groupVersion string
		resources    []metav1.APIResource
		wantContains string
		wantRequests int
	}{
		{
			name:         "listed after other resources",
			groupVersion: quotaDiagGV,
			resources:    []metav1.APIResource{other, quotaDiagThingResource()},
			wantContains: `discovery for "example.com/v1" lists "things": kind=Thing namespaced=true verbs=[get,list,watch]`,
			wantRequests: 1,
		},
		{
			name:         "not listed",
			groupVersion: quotaDiagGV,
			resources:    []metav1.APIResource{other},
			wantContains: `discovery for "example.com/v1" does not list "things" (1 resources served)`,
			wantRequests: 1,
		},
		{
			name:         "same kind but different resource name",
			groupVersion: quotaDiagGV,
			resources:    []metav1.APIResource{kindOnly},
			wantContains: `discovery for "example.com/v1" does not list "things" (1 resources served)`,
			wantRequests: 1,
		},
		{
			name:         "empty document",
			groupVersion: quotaDiagGV,
			wantContains: `discovery for "example.com/v1" does not list "things" (0 resources served)`,
			wantRequests: 1,
		},
		{
			name:         "no served version",
			groupVersion: "",
			resources:    []metav1.APIResource{quotaDiagThingResource()},
			wantContains: "skipping discovery, the CustomResourceDefinition has no served version",
			wantRequests: 0,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newQuotaDiagFixture()
			fixture.apiResources.resources = quotaDiagTestAPIResources(testCase.resources...)

			ctx, cancel := context.WithTimeout(context.Background(), customResourceQuotaDiagnosticsBudget)
			defer cancel()
			fixture.diagnostics().logGroupVersionDiscovery(ctx, testCase.groupVersion, quotaDiagPlural)

			quotaDiagAssertContains(t, fixture.log.joined(), testCase.wantContains)
			if got := len(fixture.apiResources.requests); got != testCase.wantRequests {
				t.Errorf("got %d discovery requests, want %d", got, testCase.wantRequests)
			}
		})
	}
}

func TestNewCustomResourceQuotaDiagnosticsTarget(t *testing.T) {
	got := newCustomResourceQuotaDiagnosticsTarget(quotaDiagTestCRD(), quotaDiagNamespace, quotaDiagQuotaName, quotaDiagResourceName)
	want := customResourceQuotaDiagnosticsTarget{
		crdName:        quotaDiagCRDName,
		groupVersion:   quotaDiagGV,
		resourcePlural: quotaDiagPlural,
		quotaNamespace: quotaDiagNamespace,
		quotaName:      quotaDiagQuotaName,
		resourceName:   quotaDiagResourceName,
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFirstServedCustomResourceVersion(t *testing.T) {
	testCases := []struct {
		name     string
		versions []apiextensionsv1.CustomResourceDefinitionVersion
		want     string
	}{
		{name: "no versions", want: ""},
		{
			name:     "no served version",
			versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1alpha1"}},
			want:     "",
		},
		{
			name: "first served version wins",
			versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1alpha1"},
				{Name: "v1beta1", Served: true},
				{Name: "v1", Served: true, Storage: true},
			},
			want: "example.com/v1beta1",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			definition := &apiextensionsv1.CustomResourceDefinition{
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{Group: quotaDiagGroup, Versions: testCase.versions},
			}
			if got := firstServedCustomResourceVersion(definition); got != testCase.want {
				t.Errorf("got %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestFormatResourceListIsDeterministic(t *testing.T) {
	list := v1.ResourceList{
		v1.ResourcePods:       resource.MustParse("3"),
		quotaDiagResourceName: resource.MustParse("1"),
		v1.ResourceCPU:        resource.MustParse("100m"),
	}
	const want = "{count/things.example.com=1,cpu=100m,pods=3}"
	for i := 0; i < 8; i++ {
		if got := formatResourceList(list); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	if got := formatResourceList(nil); got != "{}" {
		t.Errorf("got %q for an empty list, want {}", got)
	}
}

func TestFormatCustomResourceDefinitionVersions(t *testing.T) {
	if got := formatCustomResourceDefinitionVersions(nil); got != "[]" {
		t.Errorf("got %q for no versions, want []", got)
	}
	got := formatCustomResourceDefinitionVersions([]apiextensionsv1.CustomResourceDefinitionVersion{
		{Name: "v1beta1", Served: true},
		{Name: "v1", Served: true, Storage: true},
		{Name: "v1alpha1"},
	})
	const want = "[v1beta1(served=true,storage=false),v1(served=true,storage=true),v1alpha1(served=false,storage=false)]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatCustomResourceDefinitionConditionsReportsAbsentConditions(t *testing.T) {
	got := formatCustomResourceDefinitionConditions([]apiextensionsv1.CustomResourceDefinitionCondition{
		{
			Type:    apiextensionsv1.NamesAccepted,
			Status:  apiextensionsv1.ConditionFalse,
			Reason:  "ListKindConflict",
			Message: "\"ThingList\" is already in use",
		},
		{Type: apiextensionsv1.NonStructuralSchema, Status: apiextensionsv1.ConditionTrue},
	})
	quotaDiagAssertContains(t, got, `NamesAccepted=False(reason="ListKindConflict"`, "Established=<absent>", "lastTransitionTime=<none>")
	// Only the two scoped conditions are reported.
	quotaDiagAssertNotContains(t, got, string(apiextensionsv1.NonStructuralSchema))
}

func TestFormatObjectIdentityNormalisesTheTimestamp(t *testing.T) {
	quota := quotaDiagTestQuota()
	quota.CreationTimestamp = metav1.Date(2026, 11, 15, 2, 15, 24, 0, time.FixedZone("plus1", int(time.Hour/time.Second)))
	const want = "uid=quota-uid resourceVersion=5150 generation=7 creationTimestamp=2026-11-15T01:15:24Z"
	if got := formatObjectIdentity(quota); got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	empty := &v1.ResourceQuota{}
	if got := formatObjectIdentity(empty); got != "uid= resourceVersion= generation=0 creationTimestamp=<none>" {
		t.Errorf("unexpected rendering of an empty object: %q", got)
	}
}
