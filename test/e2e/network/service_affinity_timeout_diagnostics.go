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

// Diagnostics for the session affinity *expiry* probe used by
// execAffinityTestForSessionAffinityTimeout.
//
// The expiry probe blocks in `kubectl exec` (e2eoutput.RunHostCmd). The kubectl
// builder does log a "Running '...'" line before each exec and a "stdout:" line
// after it, but every iteration of the loop runs the *same* command, so those
// lines are byte-identical and carry no iteration ordinal and no phase marker.
// When a spec is interrupted while a probe is in flight the artifacts therefore
// show a trailing "Running" line with no companion completion line, and nothing
// states which iteration it belonged to, how long it had been pending, or what
// the earlier iterations resolved to; reconstructing that means counting
// identical lines and differencing timestamps by hand. There is also no
// aggregate (how many probes ran, how many succeeded, how many distinct
// backends answered) and no record of the Service or its EndpointSlices at the
// moment the spec died.
//
// Everything in this file is observational. It never influences the probe
// command, the assertion, the retry count, the sleep between retries, or any
// cleanup performed by the spec.
//
// Data boundaries: the lines emitted here carry only an iteration ordinal, a
// lifecycle phase, durations, a fixed-cardinality status token, a backend
// *relation* token and a distinct-backend count. They never render backend
// hostnames, command stdout/stderr, raw error strings, addresses or object
// identifiers -- whatever the kubectl builder logs separately is its own
// business and is not duplicated here.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clientset "k8s.io/client-go/kubernetes"
	uexec "k8s.io/utils/exec"

	"k8s.io/kubernetes/test/e2e/framework"

	"github.com/onsi/ginkgo/v2"
)

const (
	// affinityDiagLogPrefix tags every line emitted by this file so the records
	// are trivially greppable in a CI build log.
	affinityDiagLogPrefix = "affinity-timeout-diagnostics"

	// affinityDiagCollectionTimeout is the *total* deadline for the on-failure
	// object collection. Ginkgo allows a cleanup node only a grace period (30s
	// by default) once the suite has been interrupted, so the collection must
	// finish comfortably inside it.
	affinityDiagCollectionTimeout = 10 * time.Second

	// maxAffinityDistinctBackends caps the only unbounded state the recorder
	// keeps. The probe loop runs a small, fixed number of iterations against a
	// three-replica Deployment, so this is a guard rail rather than an expected
	// limit; the reported count saturates here.
	maxAffinityDistinctBackends = 64

	// noAffinityProbeInFlight is the sentinel ordinal used when no probe is
	// currently executing.
	noAffinityProbeInFlight = -1

	// affinityDiagValueUnset is reported for a field the Service does not set.
	affinityDiagValueUnset = "unset"

	// affinityDiagReasonUnknown is reported for an error that carries no
	// recognizable, safe-to-log reason.
	affinityDiagReasonUnknown = "Unknown"
)

// affinityDiagLogf emits one prefixed diagnostics line. It is named with the
// "f" suffix so that vet's printf analysis treats it as a printf wrapper.
func affinityDiagLogf(format string, args ...interface{}) {
	framework.Logf("%s: %s", affinityDiagLogPrefix, fmt.Sprintf(format, args...))
}

// affinityProbePhase is the lifecycle point at which a telemetry record was taken.
type affinityProbePhase string

const (
	affinityProbePhaseStart   affinityProbePhase = "start"
	affinityProbePhaseOutcome affinityProbePhase = "outcome"
)

// affinityProbeStatus is a sanitized, fixed-cardinality classification of a
// probe. It never embeds anything derived from command output or error text.
type affinityProbeStatus string

const (
	// affinityProbeStatusRunning marks the start record of a probe. A start
	// record with no matching outcome record is an in-flight (and, after an
	// interrupt, an abandoned) exec.
	affinityProbeStatusRunning affinityProbeStatus = "running"
	// affinityProbeStatusCompleted marks a probe that returned a backend.
	affinityProbeStatusCompleted affinityProbeStatus = "completed"
	// affinityProbeStatusErrExitCode marks a probe whose `kubectl exec` ran and
	// exited non-zero, which is what a refused or timed-out curl looks like.
	affinityProbeStatusErrExitCode affinityProbeStatus = "error-exit-code"
	// affinityProbeStatusErrCanceled and affinityProbeStatusErrDeadline are
	// defensive: the current probe helper does not thread a context into
	// `kubectl exec`, so they are only reachable if it ever starts to.
	affinityProbeStatusErrCanceled affinityProbeStatus = "error-canceled"
	affinityProbeStatusErrDeadline affinityProbeStatus = "error-deadline-exceeded"
	// affinityProbeStatusErrExec marks a probe that could not be run at all.
	affinityProbeStatusErrExec affinityProbeStatus = "error-exec"
)

// affinityBackendRelation describes how a probe's backend compares with the
// backend observed by the previous successful probe. It deliberately carries a
// relation rather than an identity.
type affinityBackendRelation string

const (
	affinityBackendRelationNA        affinityBackendRelation = "n/a"
	affinityBackendRelationFirst     affinityBackendRelation = "first"
	affinityBackendRelationSame      affinityBackendRelation = "same"
	affinityBackendRelationDifferent affinityBackendRelation = "different"
)

// classifyAffinityProbeError maps a probe error onto a fixed set of status
// tokens. The error itself is never retained: e2eoutput.RunHostCmd wraps the
// command's stdout and stderr into its error message.
func classifyAffinityProbeError(err error) affinityProbeStatus {
	var exitErr uexec.CodeExitError
	switch {
	case err == nil:
		return affinityProbeStatusCompleted
	case errors.Is(err, context.Canceled):
		return affinityProbeStatusErrCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return affinityProbeStatusErrDeadline
	case errors.As(err, &exitErr):
		return affinityProbeStatusErrExitCode
	default:
		return affinityProbeStatusErrExec
	}
}

// affinityProbeRecord is one telemetry point for one probe iteration. Records
// are logged as they happen and are not retained.
type affinityProbeRecord struct {
	Ordinal int
	Phase   affinityProbePhase
	// Elapsed is measured from recorder construction, immediately before the
	// probe loop.
	Elapsed time.Duration
	// Duration is the probe's own wall time; it is zero on start records.
	Duration         time.Duration
	Status           affinityProbeStatus
	Relation         affinityBackendRelation
	DistinctBackends int
}

func (r affinityProbeRecord) String() string {
	return fmt.Sprintf("probe=%d phase=%s elapsed=%s duration=%s status=%s backend=%s distinctBackends=%d",
		r.Ordinal, r.Phase,
		r.Elapsed.Truncate(time.Millisecond), r.Duration.Truncate(time.Millisecond),
		r.Status, r.Relation, r.DistinctBackends)
}

// affinityProbeSnapshot is a fixed-shape, immutable summary of the recorder.
type affinityProbeSnapshot struct {
	// Started counts probes that began; Finished counts probes that returned,
	// whether or not they succeeded; Succeeded counts probes that returned a
	// backend. Started > Finished means a probe was abandoned mid-exec.
	Started   int
	Finished  int
	Succeeded int

	DistinctBackends int

	InFlight        bool
	InFlightOrdinal int
	InFlightFor     time.Duration
}

func (s affinityProbeSnapshot) String() string {
	return fmt.Sprintf("summary started=%d finished=%d succeeded=%d distinctBackends=%d inFlight=%t inFlightProbe=%d inFlightFor=%s",
		s.Started, s.Finished, s.Succeeded, s.DistinctBackends,
		s.InFlight, s.InFlightOrdinal, s.InFlightFor.Truncate(time.Millisecond))
}

// affinityProbeRecorder records bounded, sanitized telemetry around the expiry
// probe loop.
//
// Concurrency: the recorder is written by the goroutine running the spec body
// and read by a Ginkgo cleanup node. After an interrupt Ginkgo abandons the spec
// goroutine (it keeps running, blocked in the exec) and proceeds to the cleanup
// nodes, so the two really can run at the same time. Every field below is
// therefore guarded by mu.
type affinityProbeRecorder struct {
	// now is injectable for tests; it is time.Now in production.
	now func() time.Time

	mu    sync.Mutex
	start time.Time

	started   int
	finished  int
	succeeded int

	inFlightOrdinal int
	inFlightSince   time.Time

	// lastBackend/haveBackend/backends are only used to derive a relation and a
	// count. They hold raw backend hostnames and are never rendered into any log
	// line or snapshot.
	lastBackend string
	haveBackend bool
	backends    map[string]struct{}
}

func newAffinityProbeRecorder() *affinityProbeRecorder {
	return newAffinityProbeRecorderWithClock(time.Now)
}

func newAffinityProbeRecorderWithClock(now func() time.Time) *affinityProbeRecorder {
	return &affinityProbeRecorder{
		now:             now,
		start:           now(),
		inFlightOrdinal: noAffinityProbeInFlight,
		backends:        map[string]struct{}{},
	}
}

// probeStarted records that probe ordinal is about to block on the exec.
func (r *affinityProbeRecorder) probeStarted(ordinal int) {
	affinityDiagLogf("%s", r.recordStart(ordinal))
}

// recordStart mutates the recorder and returns the record to log. The lock is
// released (via defer, so a panic cannot strand it) before the caller logs.
func (r *affinityProbeRecorder) recordStart(ordinal int) affinityProbeRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	r.started++
	r.inFlightOrdinal = ordinal
	r.inFlightSince = now
	return affinityProbeRecord{
		Ordinal:          ordinal,
		Phase:            affinityProbePhaseStart,
		Elapsed:          now.Sub(r.start),
		Status:           affinityProbeStatusRunning,
		Relation:         affinityBackendRelationNA,
		DistinctBackends: len(r.backends),
	}
}

// probeFinished records the outcome of probe ordinal. backend is the hostname
// the probe returned; it is used only to derive a relation and a count.
func (r *affinityProbeRecorder) probeFinished(ordinal int, backend string, err error) {
	affinityDiagLogf("%s", r.recordFinish(ordinal, backend, err))
}

// recordFinish mutates the recorder and returns the record to log.
func (r *affinityProbeRecorder) recordFinish(ordinal int, backend string, err error) affinityProbeRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	status := classifyAffinityProbeError(err)
	relation := affinityBackendRelationNA
	if err == nil {
		switch {
		case !r.haveBackend:
			relation = affinityBackendRelationFirst
		case backend == r.lastBackend:
			relation = affinityBackendRelationSame
		default:
			relation = affinityBackendRelationDifferent
		}
		r.lastBackend = backend
		r.haveBackend = true
		if len(r.backends) < maxAffinityDistinctBackends {
			r.backends[backend] = struct{}{}
		}
		r.succeeded++
	}

	// Only the probe the recorder believes is in flight can be timed, and only
	// it clears the in-flight marker. The probe loop is sequential, so this is
	// always the matching ordinal; the guard keeps a stale or duplicated call
	// from erasing a genuinely running probe.
	var duration time.Duration
	if r.inFlightOrdinal == ordinal {
		duration = now.Sub(r.inFlightSince)
		r.inFlightOrdinal = noAffinityProbeInFlight
		r.inFlightSince = time.Time{}
	}
	r.finished++

	return affinityProbeRecord{
		Ordinal:          ordinal,
		Phase:            affinityProbePhaseOutcome,
		Elapsed:          now.Sub(r.start),
		Duration:         duration,
		Status:           status,
		Relation:         relation,
		DistinctBackends: len(r.backends),
	}
}

// snapshot returns a fixed-shape copy of the recorder state. It is safe to call
// while a probe is still running.
func (r *affinityProbeRecorder) snapshot() affinityProbeSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	s := affinityProbeSnapshot{
		Started:          r.started,
		Finished:         r.finished,
		Succeeded:        r.succeeded,
		DistinctBackends: len(r.backends),
		InFlight:         r.inFlightOrdinal != noAffinityProbeInFlight,
		InFlightOrdinal:  r.inFlightOrdinal,
	}
	if s.InFlight {
		s.InFlightFor = r.now().Sub(r.inFlightSince)
	}
	return s
}

// affinityServiceSummary is the fixed-shape description of the Service under
// test. It carries no addresses and no object identifiers.
type affinityServiceSummary struct {
	Type                   string
	SessionAffinity        string
	SessionAffinityTimeout string
	ErrReason              string
}

func (s affinityServiceSummary) String() string {
	if s.ErrReason != "" {
		return fmt.Sprintf("service: err=%s", s.ErrReason)
	}
	return fmt.Sprintf("service: type=%s sessionAffinity=%s sessionAffinityTimeoutSeconds=%s",
		s.Type, s.SessionAffinity, s.SessionAffinityTimeout)
}

// affinityEndpointSliceSummary is an aggregate readiness count across the
// EndpointSlices owned by the Service under test. Individual endpoints,
// addresses and node names are never recorded.
type affinityEndpointSliceSummary struct {
	Slices       int
	Endpoints    int
	Ready        int
	NotReady     int
	UnknownReady int
	Serving      int
	Terminating  int
	ErrReason    string
}

func (s affinityEndpointSliceSummary) String() string {
	if s.ErrReason != "" {
		return fmt.Sprintf("endpointslices: err=%s", s.ErrReason)
	}
	return fmt.Sprintf("endpointslices: slices=%d endpoints=%d ready=%d notReady=%d readyUnknown=%d serving=%d terminating=%d",
		s.Slices, s.Endpoints, s.Ready, s.NotReady, s.UnknownReady, s.Serving, s.Terminating)
}

// sanitizeAPIErrorReason maps an API error onto a bounded reason token. Raw
// error strings are never emitted because they can embed request URLs and
// object contents.
func sanitizeAPIErrorReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "DeadlineExceeded"
	case errors.Is(err, context.Canceled):
		return "Canceled"
	}
	if reason := apierrors.ReasonForError(err); reason != metav1.StatusReasonUnknown {
		return string(reason)
	}
	return affinityDiagReasonUnknown
}

// summarizeAffinityService reduces a Service to the three fields that matter for
// a session affinity expiry failure.
func summarizeAffinityService(svc *v1.Service, err error) affinityServiceSummary {
	if err != nil {
		return affinityServiceSummary{ErrReason: sanitizeAPIErrorReason(err)}
	}
	if svc == nil {
		return affinityServiceSummary{ErrReason: affinityDiagReasonUnknown}
	}
	summary := affinityServiceSummary{
		Type:                   string(svc.Spec.Type),
		SessionAffinity:        string(svc.Spec.SessionAffinity),
		SessionAffinityTimeout: affinityDiagValueUnset,
	}
	if summary.Type == "" {
		summary.Type = affinityDiagValueUnset
	}
	if summary.SessionAffinity == "" {
		summary.SessionAffinity = affinityDiagValueUnset
	}
	if cfg := svc.Spec.SessionAffinityConfig; cfg != nil && cfg.ClientIP != nil && cfg.ClientIP.TimeoutSeconds != nil {
		summary.SessionAffinityTimeout = strconv.Itoa(int(*cfg.ClientIP.TimeoutSeconds))
	}
	return summary
}

// summarizeAffinityEndpointSlices aggregates endpoint readiness across slices.
func summarizeAffinityEndpointSlices(list *discoveryv1.EndpointSliceList, err error) affinityEndpointSliceSummary {
	if err != nil {
		return affinityEndpointSliceSummary{ErrReason: sanitizeAPIErrorReason(err)}
	}
	if list == nil {
		return affinityEndpointSliceSummary{ErrReason: affinityDiagReasonUnknown}
	}
	summary := affinityEndpointSliceSummary{Slices: len(list.Items)}
	for i := range list.Items {
		for j := range list.Items[i].Endpoints {
			conditions := list.Items[i].Endpoints[j].Conditions
			summary.Endpoints++
			switch {
			case conditions.Ready == nil:
				summary.UnknownReady++
			case *conditions.Ready:
				summary.Ready++
			default:
				summary.NotReady++
			}
			if conditions.Serving != nil && *conditions.Serving {
				summary.Serving++
			}
			if conditions.Terminating != nil && *conditions.Terminating {
				summary.Terminating++
			}
		}
	}
	return summary
}

// affinityEndpointSliceSelector scopes the EndpointSlice read to the Service
// under test rather than listing every slice in the namespace.
func affinityEndpointSliceSelector(serviceName string) string {
	return fmt.Sprintf("%s=%s", discoveryv1.LabelServiceName, serviceName)
}

// collectAffinityDiagnostics performs the two scoped reads under the caller's
// context and reduces them to fixed-shape summaries.
func collectAffinityDiagnostics(ctx context.Context, cs clientset.Interface, namespace, serviceName string) (affinityServiceSummary, affinityEndpointSliceSummary) {
	svc, svcErr := cs.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	list, listErr := cs.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: affinityEndpointSliceSelector(serviceName),
	})
	return summarizeAffinityService(svc, svcErr), summarizeAffinityEndpointSlices(list, listErr)
}

// logAffinityDiagnostics is the production collector passed to
// reportAffinityTimeoutDiagnostics.
func logAffinityDiagnostics(ctx context.Context, cs clientset.Interface, namespace, serviceName string) {
	svc, slices := collectAffinityDiagnostics(ctx, cs, namespace, serviceName)
	affinityDiagLogf("%s", svc)
	affinityDiagLogf("%s", slices)
}

// reportAffinityTimeoutDiagnostics is the body of the spec's cleanup hook.
//
// It is a no-op when the spec passed, so a clean run performs no extra API
// calls and emits nothing. When the spec failed, timed out, was aborted or was
// interrupted it logs the probe summary, and -- unless artifact collection is
// disabled for the run -- then calls collect under a *fresh* bounded context:
// the spec's own context has already been cancelled by that point, and Ginkgo
// allows a cleanup node only a short grace period.
func reportAffinityTimeoutDiagnostics(failed, collectArtifacts bool, probes *affinityProbeRecorder, collect func(ctx context.Context)) {
	if !failed {
		return
	}
	affinityDiagLogf("%s", probes.snapshot())
	if !collectArtifacts {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), affinityDiagCollectionTimeout)
	defer cancel()
	collect(ctx)
}

// startAffinityTimeoutDiagnostics creates the probe recorder and registers the
// diagnostics cleanup hook for the current spec.
//
// Call it from the spec body *after* the fixture cleanups it must outlive. The
// framework registers its own teardown (namespace deletion) as the very first
// DeferCleanup and Ginkgo runs DeferCleanup nodes first-in-last-out, so a hook
// registered last runs first -- before the exec pod is deleted, before
// StopServeHostnameService, and before the namespace goes away.
//
// The hook deliberately takes no context parameter: Ginkgo cancels the spec's
// context on interrupt, which is exactly when the diagnostics are needed, so the
// collection builds its own bounded context instead.
func startAffinityTimeoutDiagnostics(cs clientset.Interface, namespace, serviceName string) *affinityProbeRecorder {
	probes := newAffinityProbeRecorder()
	ginkgo.DeferCleanup(func() {
		reportAffinityTimeoutDiagnostics(
			ginkgo.CurrentSpecReport().Failed(),
			framework.TestContext.DumpLogsOnFailure,
			probes,
			func(ctx context.Context) { logAffinityDiagnostics(ctx, cs, namespace, serviceName) },
		)
	})
	return probes
}
