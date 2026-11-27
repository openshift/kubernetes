/*
Copyright 2026 The Kubernetes Authors.

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

package network

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	uexec "k8s.io/utils/exec"
	"k8s.io/utils/ptr"
)

const (
	testAffinityNamespace = "services-1234"
	testAffinityService   = "affinity-clusterip-timeout"
)

// newFakeClock returns a clock function that advances by step on every call,
// starting at a fixed instant so durations are deterministic.
func newFakeClock(t *testing.T, step time.Duration) func() time.Time {
	t.Helper()
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	var n int
	return func() time.Time {
		t.Helper()
		now := base.Add(time.Duration(n) * step)
		n++
		return now
	}
}

func TestClassifyAffinityProbeError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want affinityProbeStatus
	}{
		{name: "success", err: nil, want: affinityProbeStatusCompleted},
		{name: "canceled", err: context.Canceled, want: affinityProbeStatusErrCanceled},
		{name: "wrapped canceled", err: fmt.Errorf("exec failed: %w", context.Canceled), want: affinityProbeStatusErrCanceled},
		{name: "deadline", err: context.DeadlineExceeded, want: affinityProbeStatusErrDeadline},
		{name: "wrapped deadline", err: fmt.Errorf("exec failed: %w", context.DeadlineExceeded), want: affinityProbeStatusErrDeadline},
		{
			// This is what e2eoutput.RunHostCmd actually returns when the curl
			// inside the exec pod exits non-zero.
			name: "kubectl exec exited non-zero",
			err:  uexec.CodeExitError{Err: errors.New("error running kubectl exec: curl: (7) Failed to connect"), Code: 7},
			want: affinityProbeStatusErrExitCode,
		},
		{
			name: "wrapped exit error",
			err:  fmt.Errorf("probe: %w", uexec.CodeExitError{Err: errors.New("boom"), Code: 28}),
			want: affinityProbeStatusErrExitCode,
		},
		{name: "could not run the command at all", err: errors.New("error starting kubectl: fork/exec: no such file"), want: affinityProbeStatusErrExec},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyAffinityProbeError(tc.err); got != tc.want {
				t.Errorf("classifyAffinityProbeError() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAffinityProbeRecorderClassification(t *testing.T) {
	type step struct {
		ordinal int
		backend string
		err     error

		wantStatus   affinityProbeStatus
		wantRelation affinityBackendRelation
		wantDistinct int
	}
	testCases := []struct {
		name          string
		steps         []step
		wantSucceeded int
	}{
		{
			name: "sticky to the same backend",
			steps: []step{
				{ordinal: 0, backend: "pod-a", wantStatus: affinityProbeStatusCompleted, wantRelation: affinityBackendRelationFirst, wantDistinct: 1},
				{ordinal: 1, backend: "pod-a", wantStatus: affinityProbeStatusCompleted, wantRelation: affinityBackendRelationSame, wantDistinct: 1},
			},
			wantSucceeded: 2,
		},
		{
			name: "affinity expires and a different backend answers",
			steps: []step{
				{ordinal: 0, backend: "pod-a", wantStatus: affinityProbeStatusCompleted, wantRelation: affinityBackendRelationFirst, wantDistinct: 1},
				{ordinal: 1, backend: "pod-b", wantStatus: affinityProbeStatusCompleted, wantRelation: affinityBackendRelationDifferent, wantDistinct: 2},
				{ordinal: 2, backend: "pod-a", wantStatus: affinityProbeStatusCompleted, wantRelation: affinityBackendRelationDifferent, wantDistinct: 2},
			},
			wantSucceeded: 3,
		},
		{
			name: "errors carry no backend relation and do not change the distinct count",
			steps: []step{
				{ordinal: 0, err: uexec.CodeExitError{Err: errors.New("boom"), Code: 7}, wantStatus: affinityProbeStatusErrExitCode, wantRelation: affinityBackendRelationNA, wantDistinct: 0},
				{ordinal: 1, backend: "pod-a", wantStatus: affinityProbeStatusCompleted, wantRelation: affinityBackendRelationFirst, wantDistinct: 1},
				{ordinal: 2, backend: "ignored", err: context.Canceled, wantStatus: affinityProbeStatusErrCanceled, wantRelation: affinityBackendRelationNA, wantDistinct: 1},
				{ordinal: 3, backend: "pod-a", wantStatus: affinityProbeStatusCompleted, wantRelation: affinityBackendRelationSame, wantDistinct: 1},
			},
			wantSucceeded: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := newAffinityProbeRecorderWithClock(newFakeClock(t, time.Second))
			for i, s := range tc.steps {
				start := r.recordStart(s.ordinal)
				if start.Phase != affinityProbePhaseStart || start.Status != affinityProbeStatusRunning {
					t.Errorf("step %d: start record = %q, want a running start record", i, start)
				}

				got := r.recordFinish(s.ordinal, s.backend, s.err)
				if got.Ordinal != s.ordinal {
					t.Errorf("step %d: ordinal = %d, want %d", i, got.Ordinal, s.ordinal)
				}
				if got.Phase != affinityProbePhaseOutcome {
					t.Errorf("step %d: phase = %q, want %q", i, got.Phase, affinityProbePhaseOutcome)
				}
				if got.Status != s.wantStatus {
					t.Errorf("step %d: status = %q, want %q", i, got.Status, s.wantStatus)
				}
				if got.Relation != s.wantRelation {
					t.Errorf("step %d: relation = %q, want %q", i, got.Relation, s.wantRelation)
				}
				if got.DistinctBackends != s.wantDistinct {
					t.Errorf("step %d: distinctBackends = %d, want %d", i, got.DistinctBackends, s.wantDistinct)
				}
				if got.Duration <= 0 {
					t.Errorf("step %d: duration = %v, want a positive probe duration", i, got.Duration)
				}
			}

			snapshot := r.snapshot()
			if snapshot.InFlight {
				t.Errorf("snapshot reports a probe in flight after every probe finished: %q", snapshot)
			}
			if snapshot.Started != len(tc.steps) || snapshot.Finished != len(tc.steps) {
				t.Errorf("started=%d finished=%d, want %d/%d", snapshot.Started, snapshot.Finished, len(tc.steps), len(tc.steps))
			}
			if snapshot.Succeeded != tc.wantSucceeded {
				t.Errorf("succeeded = %d, want %d", snapshot.Succeeded, tc.wantSucceeded)
			}
		})
	}
}

// TestAffinityProbeRecorderInFlight covers the case the diagnostics exist for:
// a probe that starts and never returns because the spec was interrupted.
func TestAffinityProbeRecorderInFlight(t *testing.T) {
	r := newAffinityProbeRecorderWithClock(newFakeClock(t, time.Minute))

	r.recordStart(0)
	r.recordFinish(0, "pod-a", nil)
	r.recordStart(1)

	snapshot := r.snapshot()
	if !snapshot.InFlight {
		t.Fatalf("snapshot.InFlight = false, want true while probe 1 is still executing")
	}
	if snapshot.InFlightOrdinal != 1 {
		t.Errorf("snapshot.InFlightOrdinal = %d, want 1", snapshot.InFlightOrdinal)
	}
	if snapshot.InFlightFor <= 0 {
		t.Errorf("snapshot.InFlightFor = %v, want a positive duration", snapshot.InFlightFor)
	}
	// Started > Finished is what identifies the abandoned exec.
	if snapshot.Started != 2 || snapshot.Finished != 1 || snapshot.Succeeded != 1 {
		t.Errorf("started=%d finished=%d succeeded=%d, want 2/1/1", snapshot.Started, snapshot.Finished, snapshot.Succeeded)
	}

	for _, want := range []string{"started=2", "finished=1", "inFlight=true", "inFlightProbe=1"} {
		if !strings.Contains(snapshot.String(), want) {
			t.Errorf("summary %q does not contain %q", snapshot, want)
		}
	}
}

// TestAffinityProbeRecorderOutOfOrderFinish asserts that a stale or duplicated
// finish cannot erase a genuinely in-flight probe.
func TestAffinityProbeRecorderOutOfOrderFinish(t *testing.T) {
	r := newAffinityProbeRecorderWithClock(newFakeClock(t, time.Second))

	r.recordStart(0)
	r.recordFinish(0, "pod-a", nil)
	r.recordStart(1)
	// A duplicate finish for the already-finished probe 0.
	if got := r.recordFinish(0, "pod-a", nil); got.Duration != 0 {
		t.Errorf("duplicate finish reported duration %v, want 0", got.Duration)
	}

	snapshot := r.snapshot()
	if !snapshot.InFlight || snapshot.InFlightOrdinal != 1 {
		t.Errorf("snapshot = %q, want probe 1 still in flight", snapshot)
	}
}

// TestAffinityProbeRecorderDistinctBackendsSaturate pins the only unbounded
// state the recorder keeps.
func TestAffinityProbeRecorderDistinctBackendsSaturate(t *testing.T) {
	r := newAffinityProbeRecorderWithClock(newFakeClock(t, time.Millisecond))
	for i := 0; i < maxAffinityDistinctBackends+10; i++ {
		r.recordStart(i)
		r.recordFinish(i, fmt.Sprintf("pod-%d", i), nil)
	}

	if got := r.snapshot().DistinctBackends; got != maxAffinityDistinctBackends {
		t.Errorf("DistinctBackends = %d, want it capped at %d", got, maxAffinityDistinctBackends)
	}
}

// TestAffinityProbeRecorderConcurrentWithSnapshot models what Ginkgo does on an
// interrupt: it abandons the goroutine running the spec body (which stays
// blocked in the exec and may still report later) and runs the cleanup node,
// which reads the recorder. Run with -race, this asserts the two do not race.
func TestAffinityProbeRecorderConcurrentWithSnapshot(t *testing.T) {
	r := newAffinityProbeRecorder()

	var wg sync.WaitGroup
	wg.Add(2)

	// The abandoned "spec" goroutine keeps probing.
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			r.recordStart(i)
			r.recordFinish(i, fmt.Sprintf("pod-%d", i%3), nil)
		}
	}()

	// The "cleanup node" goroutine reports concurrently.
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if line := r.snapshot().String(); line == "" {
				t.Error("unexpected empty telemetry summary")
			}
		}
	}()

	wg.Wait()
}

// TestAffinityProbeTelemetryDataBoundaries asserts that nothing derived from
// backend hostnames or raw error text reaches a rendered line.
func TestAffinityProbeTelemetryDataBoundaries(t *testing.T) {
	const (
		backendA = "netserver-0-abcdef"
		backendB = "netserver-1-ghijkl"
		rawError = "error running kubectl exec: 10.128.2.17:9376 refused; stderr: + curl -q -s http://172.30.44.9:80/"
	)

	r := newAffinityProbeRecorderWithClock(newFakeClock(t, time.Second))
	lines := []string{
		r.recordStart(0).String(),
		r.recordFinish(0, backendA, nil).String(),
		r.recordStart(1).String(),
		r.recordFinish(1, backendB, uexec.CodeExitError{Err: errors.New(rawError), Code: 7}).String(),
		r.recordStart(2).String(),
		r.snapshot().String(),
	}
	rendered := strings.Join(lines, "\n")

	for _, forbidden := range []string{backendA, backendB, rawError, "netserver", "10.128.2.17", "172.30.44.9", "stderr", "curl"} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("rendered telemetry leaks %q:\n%s", forbidden, rendered)
		}
	}

	// ...and the records still say something useful.
	for _, want := range []string{"probe=0", "status=completed", "backend=first", "status=error-exit-code", "inFlight=true", "inFlightProbe=2"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered telemetry is missing %q:\n%s", want, rendered)
		}
	}
}

func TestSanitizeAPIErrorReason(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "deadline", err: context.DeadlineExceeded, want: "DeadlineExceeded"},
		{name: "wrapped deadline", err: fmt.Errorf("get service: %w", context.DeadlineExceeded), want: "DeadlineExceeded"},
		{name: "canceled", err: context.Canceled, want: "Canceled"},
		{name: "not found", err: apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, testAffinityService), want: string(metav1.StatusReasonNotFound)},
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Resource: "endpointslices"}, testAffinityService, errors.New("nope")), want: string(metav1.StatusReasonForbidden)},
		{name: "opaque", err: errors.New("dial tcp 10.0.0.1:6443: connection refused"), want: affinityDiagReasonUnknown},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeAPIErrorReason(tc.err)
			if got != tc.want {
				t.Fatalf("sanitizeAPIErrorReason() = %q, want %q", got, tc.want)
			}
			// The token must not carry anything from the raw error text.
			for _, fragment := range []string{"10.0.0.1:6443", testAffinityService, "nope", "connection refused"} {
				if strings.Contains(got, fragment) {
					t.Errorf("sanitizeAPIErrorReason() = %q, which leaks %q", got, fragment)
				}
			}
		})
	}
}

func TestSummarizeAffinityService(t *testing.T) {
	testCases := []struct {
		name string
		svc  *v1.Service
		err  error
		want string
	}{
		{
			name: "clusterIP service with a session affinity timeout",
			svc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: testAffinityService, Namespace: testAffinityNamespace, UID: "0f8f6d1e-0000-4000-8000-000000000001"},
				Spec: v1.ServiceSpec{
					Type:            v1.ServiceTypeClusterIP,
					ClusterIP:       "172.30.44.9",
					SessionAffinity: v1.ServiceAffinityClientIP,
					SessionAffinityConfig: &v1.SessionAffinityConfig{
						ClientIP: &v1.ClientIPConfig{TimeoutSeconds: ptr.To[int32](125)},
					},
				},
			},
			want: "service: type=ClusterIP sessionAffinity=ClientIP sessionAffinityTimeoutSeconds=125",
		},
		{
			name: "empty service renders a fixed shape",
			svc:  &v1.Service{},
			want: "service: type=unset sessionAffinity=unset sessionAffinityTimeoutSeconds=unset",
		},
		{
			name: "read error is reduced to a bounded reason",
			err:  apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, testAffinityService),
			want: "service: err=NotFound",
		},
		{
			name: "nil service without an error",
			want: "service: err=" + affinityDiagReasonUnknown,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeAffinityService(tc.svc, tc.err).String()
			if got != tc.want {
				t.Fatalf("summary = %q, want %q", got, tc.want)
			}
			for _, forbidden := range []string{"172.30.44.9", "0f8f6d1e", testAffinityNamespace, testAffinityService} {
				if strings.Contains(got, forbidden) {
					t.Errorf("summary %q leaks %q", got, forbidden)
				}
			}
		})
	}
}

func TestSummarizeAffinityEndpointSlices(t *testing.T) {
	testCases := []struct {
		name string
		list *discoveryv1.EndpointSliceList
		err  error
		want string
	}{
		{
			name: "mixed readiness across slices",
			list: &discoveryv1.EndpointSliceList{
				Items: []discoveryv1.EndpointSlice{
					{
						Endpoints: []discoveryv1.Endpoint{
							{Addresses: []string{"10.128.2.17"}, Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true), Serving: ptr.To(true), Terminating: ptr.To(false)}},
							{Addresses: []string{"10.128.2.18"}, Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(false), Serving: ptr.To(true), Terminating: ptr.To(true)}},
						},
					},
					{
						Endpoints: []discoveryv1.Endpoint{
							{Addresses: []string{"10.128.2.19"}, Conditions: discoveryv1.EndpointConditions{}},
						},
					},
				},
			},
			want: "endpointslices: slices=2 endpoints=3 ready=1 notReady=1 readyUnknown=1 serving=2 terminating=1",
		},
		{
			name: "no slices",
			list: &discoveryv1.EndpointSliceList{},
			want: "endpointslices: slices=0 endpoints=0 ready=0 notReady=0 readyUnknown=0 serving=0 terminating=0",
		},
		{
			name: "read error is reduced to a bounded reason",
			err:  context.DeadlineExceeded,
			want: "endpointslices: err=DeadlineExceeded",
		},
		{
			name: "nil list without an error",
			want: "endpointslices: err=" + affinityDiagReasonUnknown,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeAffinityEndpointSlices(tc.list, tc.err).String()
			if got != tc.want {
				t.Fatalf("summary = %q, want %q", got, tc.want)
			}
			for _, forbidden := range []string{"10.128.2.17", "10.128.2.18", "10.128.2.19"} {
				if strings.Contains(got, forbidden) {
					t.Errorf("summary %q leaks address %q", got, forbidden)
				}
			}
		})
	}
}

func TestAffinityEndpointSliceSelector(t *testing.T) {
	if got, want := affinityEndpointSliceSelector(testAffinityService), "kubernetes.io/service-name="+testAffinityService; got != want {
		t.Errorf("affinityEndpointSliceSelector() = %q, want %q", got, want)
	}
}

func newAffinityTestEndpointSlice(name, namespace, serviceName string, ready bool) *discoveryv1.EndpointSlice {
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{discoveryv1.LabelServiceName: serviceName},
		},
		Endpoints: []discoveryv1.Endpoint{{Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(ready)}}},
	}
}

// TestCollectAffinityDiagnosticsIsScoped asserts that the collection reads only
// the Service under test and only the EndpointSlices that belong to it, in the
// test's own namespace.
func TestCollectAffinityDiagnosticsIsScoped(t *testing.T) {
	cs := fake.NewClientset(
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: testAffinityService, Namespace: testAffinityNamespace},
			Spec: v1.ServiceSpec{
				Type:            v1.ServiceTypeClusterIP,
				SessionAffinity: v1.ServiceAffinityClientIP,
				SessionAffinityConfig: &v1.SessionAffinityConfig{
					ClientIP: &v1.ClientIPConfig{TimeoutSeconds: ptr.To[int32](125)},
				},
			},
		},
		newAffinityTestEndpointSlice("affinity-abcde", testAffinityNamespace, testAffinityService, true),
		newAffinityTestEndpointSlice("affinity-fghij", testAffinityNamespace, testAffinityService, false),
		// Decoys: another Service in the same namespace, and the same Service
		// name in a different namespace. Neither may be counted.
		newAffinityTestEndpointSlice("other-svc-klmno", testAffinityNamespace, "some-other-service", true),
		newAffinityTestEndpointSlice("affinity-pqrst", "services-9999", testAffinityService, true),
	)

	svc, slices := collectAffinityDiagnostics(context.Background(), cs, testAffinityNamespace, testAffinityService)

	if want := "service: type=ClusterIP sessionAffinity=ClientIP sessionAffinityTimeoutSeconds=125"; svc.String() != want {
		t.Errorf("service summary = %q, want %q", svc, want)
	}
	if want := "endpointslices: slices=2 endpoints=2 ready=1 notReady=1 readyUnknown=0 serving=0 terminating=0"; slices.String() != want {
		t.Errorf("endpointslice summary = %q, want %q", slices, want)
	}
}

// TestCollectAffinityDiagnosticsSanitizesAPIErrors asserts that a failing read
// still produces a bounded, sanitized line rather than a raw error.
func TestCollectAffinityDiagnosticsSanitizesAPIErrors(t *testing.T) {
	cs := fake.NewClientset()
	cs.PrependReactor("get", "services", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "services"}, testAffinityService, errors.New("user cannot get services in namespace services-1234"))
	})
	cs.PrependReactor("list", "endpointslices", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewTimeoutError("request did not complete within 10s", 10)
	})

	svc, slices := collectAffinityDiagnostics(context.Background(), cs, testAffinityNamespace, testAffinityService)

	if want := "service: err=Forbidden"; svc.String() != want {
		t.Errorf("service summary = %q, want %q", svc, want)
	}
	if want := "endpointslices: err=Timeout"; slices.String() != want {
		t.Errorf("endpointslice summary = %q, want %q", slices, want)
	}
	for _, forbidden := range []string{"user cannot get", testAffinityNamespace, "within 10s"} {
		if strings.Contains(svc.String()+slices.String(), forbidden) {
			t.Errorf("summaries leak %q", forbidden)
		}
	}
}

// TestReportAffinityTimeoutDiagnosticsCollectionGating pins the "a clean pass
// must not make artifact API calls" requirement, and the opt-out honored by
// every other on-failure collector in this tree.
func TestReportAffinityTimeoutDiagnosticsCollectionGating(t *testing.T) {
	testCases := []struct {
		name             string
		failed           bool
		collectArtifacts bool
		wantCalls        int
	}{
		{name: "passing spec collects nothing", failed: false, collectArtifacts: true, wantCalls: 0},
		{name: "passing spec with collection disabled", failed: false, collectArtifacts: false, wantCalls: 0},
		{name: "failed or interrupted spec collects", failed: true, collectArtifacts: true, wantCalls: 1},
		{name: "failed spec with collection disabled", failed: true, collectArtifacts: false, wantCalls: 0},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := newAffinityProbeRecorderWithClock(newFakeClock(t, time.Second))
			r.recordStart(0)

			calls := 0
			reportAffinityTimeoutDiagnostics(tc.failed, tc.collectArtifacts, r, func(context.Context) { calls++ })

			if calls != tc.wantCalls {
				t.Errorf("collect called %d times, want %d", calls, tc.wantCalls)
			}
		})
	}
}

// TestReportAffinityTimeoutDiagnosticsUsesFreshBoundedContext asserts that the
// collection runs under its own deadline and is not already cancelled, which is
// what makes it usable from a cleanup node after the spec context was cancelled
// by an interrupt.
func TestReportAffinityTimeoutDiagnosticsUsesFreshBoundedContext(t *testing.T) {
	r := newAffinityProbeRecorderWithClock(newFakeClock(t, time.Second))

	var deadline time.Time
	var hasDeadline bool
	var ctxErr error
	reportAffinityTimeoutDiagnostics(true, true, r, func(ctx context.Context) {
		deadline, hasDeadline = ctx.Deadline()
		ctxErr = ctx.Err()
	})

	if !hasDeadline {
		t.Fatal("collection context has no deadline, want a bounded one")
	}
	if ctxErr != nil {
		t.Errorf("collection context is already done: %v", ctxErr)
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > affinityDiagCollectionTimeout {
		t.Errorf("collection deadline is %v away, want (0, %v]", remaining, affinityDiagCollectionTimeout)
	}
}

// TestReportAffinityTimeoutDiagnosticsCancelsItsContext asserts that the
// collection context is always released, so a collector that outlives the
// report (as an abandoned API call would) is cancelled rather than leaked, and
// that the bound fits inside Ginkgo's default cleanup grace period.
func TestReportAffinityTimeoutDiagnosticsCancelsItsContext(t *testing.T) {
	const ginkgoDefaultGracePeriod = 30 * time.Second
	if affinityDiagCollectionTimeout >= ginkgoDefaultGracePeriod {
		t.Errorf("affinityDiagCollectionTimeout is %v, which does not fit inside Ginkgo's %v cleanup grace period",
			affinityDiagCollectionTimeout, ginkgoDefaultGracePeriod)
	}

	r := newAffinityProbeRecorderWithClock(newFakeClock(t, time.Second))

	var captured context.Context
	reportAffinityTimeoutDiagnostics(true, true, r, func(ctx context.Context) { captured = ctx })

	if captured == nil {
		t.Fatal("collector was not called")
	}
	if captured.Err() == nil {
		t.Error("collection context is still live after the report returned, want it cancelled")
	}
}
