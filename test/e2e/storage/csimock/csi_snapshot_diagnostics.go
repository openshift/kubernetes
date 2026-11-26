/*
Copyright 2025 The Kubernetes Authors.

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

package csimock

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/storage/utils"
)

// Failure diagnostics for the CSI mock snapshot tests.
//
// The snapshot tests intentionally make the mock driver return
// DeadlineExceeded for the first CreateSnapshot calls, so a run that ends with
// "VolumeSnapshot ... is not ready within ..." leaves no record of what the
// objects actually looked like when the wait gave up: by the time the failure
// is reported the test is already tearing the namespace down. The helpers below
// read, once and read-only, the VolumeSnapshot the test created, the
// VolumeSnapshotContent it is bound to, and the CSI mock pods, and log a fixed
// set of sanitized fields.
//
// Deliberate limits:
//   - Only an explicit allowlist of fields is ever rendered. Raw error
//     messages, Secrets, object annotations and whole objects are never read or
//     logged, because the snapshot fixtures reference per-test Secrets and
//     record the snapshotter secret name and namespace in VolumeSnapshotContent
//     annotations.
//   - Every read is bounded individually and the collection as a whole is
//     bounded, so a wedged API server cannot delay teardown.
//   - Collection is best effort. Failures become diagnostic fields; they never
//     fail the spec, never alter the original failure and never take over
//     cleanup.
//   - Container logs are out of scope here; generic CSI mock pod log collection
//     belongs to the driver teardown path, not to this test.

const (
	// csiSnapshotDiagnosticReadTimeout bounds a single read.
	csiSnapshotDiagnosticReadTimeout = 5 * time.Second
	// csiSnapshotDiagnosticBudget bounds the whole collection.
	csiSnapshotDiagnosticBudget = 20 * time.Second
	// csiSnapshotDiagnosticMaxContainers bounds how many container entries are
	// reported for a single CSI mock pod.
	csiSnapshotDiagnosticMaxContainers = 8
	// csiSnapshotDiagnosticMaxValueLength bounds a single rendered value.
	csiSnapshotDiagnosticMaxValueLength = 128

	// csiSnapshotValueRedacted replaces a value that is not a plain token.
	csiSnapshotValueRedacted = "redacted"
	// csiSnapshotValueTruncatedSuffix marks a value that was cut to length.
	csiSnapshotValueTruncatedSuffix = "+truncated"

	// csiMockPluginPodName and csiMockSnapshotterPodName are the pods created
	// by the CSI mock fixtures in
	// test/e2e/testing-manifests/storage-csi/mock. Both are single replica
	// StatefulSets, so the pod names are stable and no listing is needed.
	csiMockPluginPodName      = "csi-mockplugin-0"
	csiMockSnapshotterPodName = "csi-mockplugin-snapshotter-0"

	// csiSnapshotDiagnosticLogPrefix marks every line emitted here so the
	// fields are easy to find in a CI artifact.
	csiSnapshotDiagnosticLogPrefix = "CSI mock snapshot failure diagnostics:"

	// Values of the "observed" field.
	csiSnapshotObservedFound        = "found"
	csiSnapshotObservedAbsent       = "absent"
	csiSnapshotObservedUnbound      = "unbound"
	csiSnapshotObservedLookupFailed = "lookup-failed"
	csiSnapshotObservedTruncated    = "truncated"
	// csiSnapshotObservedUnknown is used when an object was never looked at,
	// which must not be reported as if it had been seen.
	csiSnapshotObservedUnknown = "unknown"

	// Values of the tri-state fields.
	csiSnapshotFieldTrue  = "true"
	csiSnapshotFieldFalse = "false"
	csiSnapshotFieldUnset = "unset"

	// Values of the "failureReason" field that do not come from
	// metav1.StatusReason.
	csiSnapshotReasonTimeout  = "Timeout"
	csiSnapshotReasonCanceled = "Canceled"
	csiSnapshotReasonUnknown  = "Unknown"

	// Values of the "uidLinkage" field.
	csiSnapshotLinkageMatch    = "match"
	csiSnapshotLinkageMismatch = "mismatch"
	csiSnapshotLinkageUnknown  = "unknown"
)

// csiSnapshotObjectDiagnostic is the sanitized view of one VolumeSnapshot or
// VolumeSnapshotContent. Adding a field here is the only way to widen what the
// diagnostics can emit.
type csiSnapshotObjectDiagnostic struct {
	Kind            string
	Namespace       string
	Name            string
	Observed        string
	FailureReason   string
	UID             string
	ResourceVersion string
	Created         string
	Deleting        string
	ReadyToUse      string
	HasError        string
	BoundContent    string
	SnapshotRefUID  string
	UIDLinkage      string
	Malformed       string
}

func (d csiSnapshotObjectDiagnostic) String() string {
	return renderDiagnosticFields([]diagnosticField{
		{"kind", d.Kind},
		{"namespace", d.Namespace},
		{"name", d.Name},
		{"observed", d.Observed},
		{"failureReason", d.FailureReason},
		{"uid", d.UID},
		{"resourceVersion", d.ResourceVersion},
		{"creationTimestamp", d.Created},
		{"deleting", d.Deleting},
		{"readyToUse", d.ReadyToUse},
		{"hasError", d.HasError},
		{"boundVolumeSnapshotContentName", d.BoundContent},
		{"volumeSnapshotRefUID", d.SnapshotRefUID},
		{"uidLinkage", d.UIDLinkage},
		{"malformed", d.Malformed},
	})
}

// csiMockContainerDiagnostic is the sanitized view of one CSI mock container.
// Container and kubelet messages are omitted on purpose; only the short
// enumerated reasons are kept.
type csiMockContainerDiagnostic struct {
	Pod                     string
	Phase                   string
	Container               string
	Observed                string
	FailureReason           string
	Ready                   string
	RestartCount            string
	State                   string
	StateReason             string
	LastTerminationReason   string
	LastTerminationExitCode string
	LastTerminationSignal   string
	OmittedContainers       string
}

func (d csiMockContainerDiagnostic) String() string {
	return renderDiagnosticFields([]diagnosticField{
		{"pod", d.Pod},
		{"phase", d.Phase},
		{"container", d.Container},
		{"observed", d.Observed},
		{"failureReason", d.FailureReason},
		{"ready", d.Ready},
		{"restartCount", d.RestartCount},
		{"state", d.State},
		{"stateReason", d.StateReason},
		{"lastTerminationReason", d.LastTerminationReason},
		{"lastTerminationExitCode", d.LastTerminationExitCode},
		{"lastTerminationSignal", d.LastTerminationSignal},
		{"omittedContainers", d.OmittedContainers},
	})
}

// diagnosticField is one rendered key/value pair. Pairing them in a struct
// keeps the key next to its value at the single call site of each diagnostic
// type, so a mismatched pair is a compile error.
type diagnosticField struct {
	key   string
	value string
}

// renderDiagnosticFields joins the fields that have a value, in the order they
// are given, so the layout of a diagnostic line is fixed. Every value passes
// through sanitizeDiagnosticValue, which is the single point where what may be
// written is decided.
func renderDiagnosticFields(fields []diagnosticField) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		value := sanitizeDiagnosticValue(field.value)
		if value == "" {
			continue
		}
		parts = append(parts, field.key+"="+value)
	}
	return strings.Join(parts, " ")
}

// sanitizeDiagnosticValue constrains a value to a short plain token.
//
// Most values rendered here are names, UIDs, resource versions, timestamps,
// numbers or constants from this file, but a few are not under its control.
// ContainerStateWaiting.Reason and ContainerStateTerminated.Reason are
// documented only as a "(brief) reason": the API enforces no format and the
// kubelet fills them from what the container runtime reported. The strings read
// out of an unstructured object are likewise only as well formed as whatever
// controller wrote them. Rather than trust either, a value that is not a plain
// token is replaced whole, so no byte of an unexpected value reaches the log
// and nothing can break the one-field-per-space layout of a diagnostic line.
func sanitizeDiagnosticValue(value string) string {
	if value == "" {
		return ""
	}
	if !isPlainDiagnosticToken(value) {
		return csiSnapshotValueRedacted
	}
	if len(value) > csiSnapshotDiagnosticMaxValueLength {
		return value[:csiSnapshotDiagnosticMaxValueLength] + csiSnapshotValueTruncatedSuffix
	}
	return value
}

// isPlainDiagnosticToken reports whether value is made only of the characters
// that object names, UIDs, RFC3339 timestamps, enumerated reasons and numbers
// consist of. Anything else, including whitespace and non-ASCII text, is not a
// value this package expects to emit.
func isPlainDiagnosticToken(value string) bool {
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9':
		case character == '-', character == '_', character == '.',
			character == ':', character == '/', character == '+':
		default:
			return false
		}
	}
	return true
}

// logCSISnapshotFailureDiagnosticsIfFailed captures only after a spec has
// already failed, so a passing run reads nothing and logs nothing.
func logCSISnapshotFailureDiagnosticsIfFailed(ctx context.Context, failed bool, dc dynamic.Interface, cs clientset.Interface, snapshotNamespace, snapshotName, driverNamespace string, readTimeout, budget time.Duration) {
	if !failed {
		return
	}
	logCSISnapshotFailureDiagnostics(ctx, dc, cs, snapshotNamespace, snapshotName, driverNamespace, readTimeout, budget)
}

// logCSISnapshotFailureDiagnostics logs the sanitized state of the snapshot
// objects and of the CSI mock pods. It is read-only and best effort: it reports
// its own failures as fields and always returns.
func logCSISnapshotFailureDiagnostics(ctx context.Context, dc dynamic.Interface, cs clientset.Interface, snapshotNamespace, snapshotName, driverNamespace string, readTimeout, budget time.Duration) {
	// Cleanup can run with an already canceled context, and this must not
	// extend teardown either, so detach from the caller and apply an explicit
	// upper bound instead.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	defer cancel()

	for _, diagnostic := range collectCSISnapshotDiagnostics(ctx, dc, snapshotNamespace, snapshotName, readTimeout) {
		framework.Logf("%s %s", csiSnapshotDiagnosticLogPrefix, diagnostic)
	}
	for _, diagnostic := range collectCSIMockPodDiagnostics(ctx, cs, driverNamespace, readTimeout) {
		framework.Logf("%s %s", csiSnapshotDiagnosticLogPrefix, diagnostic)
	}
}

// collectCSISnapshotDiagnostics reads the named VolumeSnapshot and, when it
// names one, the VolumeSnapshotContent bound to it. It always returns one entry
// per object so an absent or unreadable object is visible in the log too.
func collectCSISnapshotDiagnostics(ctx context.Context, dc dynamic.Interface, namespace, name string, readTimeout time.Duration) []csiSnapshotObjectDiagnostic {
	snapshot, boundContentReadable := describeVolumeSnapshot(ctx, dc, namespace, name, readTimeout)

	content := csiSnapshotObjectDiagnostic{Kind: "VolumeSnapshotContent"}
	switch {
	case snapshot.Observed != csiSnapshotObservedFound, !boundContentReadable:
		// Either the snapshot was never read or it does not say which content
		// it is bound to. Reporting "unbound" here would claim an observation
		// that was not made.
		content.Observed = csiSnapshotObservedUnknown
	case snapshot.BoundContent == "":
		content.Observed = csiSnapshotObservedUnbound
	default:
		content = describeVolumeSnapshotContent(ctx, dc, snapshot.BoundContent, snapshot.UID, readTimeout)
	}
	return []csiSnapshotObjectDiagnostic{snapshot, content}
}

// describeVolumeSnapshot also reports whether
// status.boundVolumeSnapshotContentName could be read, so that an unreadable
// field is not mistaken for a snapshot that is not bound yet.
func describeVolumeSnapshot(ctx context.Context, dc dynamic.Interface, namespace, name string, readTimeout time.Duration) (diagnostic csiSnapshotObjectDiagnostic, boundContentReadable bool) {
	diagnostic = csiSnapshotObjectDiagnostic{Kind: "VolumeSnapshot", Namespace: namespace, Name: name}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	snapshot, err := dc.Resource(utils.SnapshotGVR).Namespace(namespace).Get(readCtx, name, metav1.GetOptions{})
	if err != nil {
		diagnostic.Observed, diagnostic.FailureReason = describeReadError(err)
		return diagnostic, false
	}

	describeSnapshotObjectMeta(&diagnostic, snapshot)
	boundContent, _, err := unstructured.NestedString(snapshot.Object, "status", "boundVolumeSnapshotContentName")
	if err != nil {
		diagnostic.Malformed = csiSnapshotFieldTrue
		return diagnostic, false
	}
	if boundContent != sanitizeDiagnosticValue(boundContent) {
		// Not a name any controller should have written. Do not render it and
		// do not look it up.
		diagnostic.Malformed = csiSnapshotFieldTrue
		return diagnostic, false
	}
	diagnostic.BoundContent = boundContent
	return diagnostic, true
}

func describeVolumeSnapshotContent(ctx context.Context, dc dynamic.Interface, name, snapshotUID string, readTimeout time.Duration) csiSnapshotObjectDiagnostic {
	diagnostic := csiSnapshotObjectDiagnostic{Kind: "VolumeSnapshotContent", Name: name}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	content, err := dc.Resource(utils.SnapshotContentGVR).Get(readCtx, name, metav1.GetOptions{})
	if err != nil {
		diagnostic.Observed, diagnostic.FailureReason = describeReadError(err)
		diagnostic.UIDLinkage = csiSnapshotLinkageUnknown
		return diagnostic
	}

	describeSnapshotObjectMeta(&diagnostic, content)
	// spec.volumeSnapshotRef identifies the VolumeSnapshot this content was
	// created for. Only the UID is taken, which is what proves the content
	// belongs to the snapshot the test created rather than to a leftover from
	// another run.
	referencedUID, _, err := unstructured.NestedString(content.Object, "spec", "volumeSnapshotRef", "uid")
	if err != nil {
		diagnostic.Malformed = csiSnapshotFieldTrue
	}
	diagnostic.SnapshotRefUID = referencedUID
	diagnostic.UIDLinkage = describeUIDLinkage(snapshotUID, referencedUID)
	return diagnostic
}

// describeSnapshotObjectMeta copies the allowlisted identity, lifecycle and
// readiness fields that VolumeSnapshot and VolumeSnapshotContent share.
func describeSnapshotObjectMeta(diagnostic *csiSnapshotObjectDiagnostic, object *unstructured.Unstructured) {
	diagnostic.Observed = csiSnapshotObservedFound
	diagnostic.UID = string(object.GetUID())
	diagnostic.ResourceVersion = object.GetResourceVersion()
	if created := object.GetCreationTimestamp(); !created.IsZero() {
		diagnostic.Created = created.UTC().Format(time.RFC3339)
	} else {
		diagnostic.Created = csiSnapshotFieldUnset
	}
	diagnostic.Deleting = describeBool(object.GetDeletionTimestamp() != nil)

	// A field that does not match the CRD schema is reported as malformed
	// rather than as a missing value, so it cannot be mistaken for a
	// controller that never wrote it.
	ready, readyFound, readyErr := unstructured.NestedBool(object.Object, "status", "readyToUse")
	diagnostic.ReadyToUse = describeOptionalBool(ready, readyFound)
	if readyErr != nil {
		diagnostic.Malformed = csiSnapshotFieldTrue
	}

	// Presence of status.error only. Its "message" field relays driver output
	// verbatim, so the value behind it is never read or rendered.
	errorField, errorFound, errorErr := unstructured.NestedFieldNoCopy(object.Object, "status", "error")
	diagnostic.HasError = describeBool(errorFound && errorErr == nil && errorField != nil)
	if errorErr != nil {
		diagnostic.Malformed = csiSnapshotFieldTrue
	}
}

// collectCSIMockPodDiagnostics reports restart and termination state for the
// CSI mock pods. It reads the two known fixture pods by name and never lists
// the namespace, so it cannot pick up unrelated workloads.
func collectCSIMockPodDiagnostics(ctx context.Context, cs clientset.Interface, driverNamespace string, readTimeout time.Duration) []csiMockContainerDiagnostic {
	if driverNamespace == "" {
		return nil
	}
	var diagnostics []csiMockContainerDiagnostic
	for _, podName := range []string{csiMockPluginPodName, csiMockSnapshotterPodName} {
		diagnostics = append(diagnostics, describeCSIMockPod(ctx, cs, driverNamespace, podName, readTimeout)...)
	}
	return diagnostics
}

func describeCSIMockPod(ctx context.Context, cs clientset.Interface, namespace, podName string, readTimeout time.Duration) []csiMockContainerDiagnostic {
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	pod, err := cs.CoreV1().Pods(namespace).Get(readCtx, podName, metav1.GetOptions{})
	if err != nil {
		observed, reason := describeReadError(err)
		return []csiMockContainerDiagnostic{{Pod: podName, Observed: observed, FailureReason: reason}}
	}

	phase := string(pod.Status.Phase)
	statuses := make([]v1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	if len(statuses) == 0 {
		return []csiMockContainerDiagnostic{{Pod: podName, Phase: phase, Observed: csiSnapshotObservedFound}}
	}

	omitted := 0
	if len(statuses) > csiSnapshotDiagnosticMaxContainers {
		omitted = len(statuses) - csiSnapshotDiagnosticMaxContainers
		statuses = statuses[:csiSnapshotDiagnosticMaxContainers]
	}

	diagnostics := make([]csiMockContainerDiagnostic, 0, len(statuses)+1)
	for _, status := range statuses {
		diagnostics = append(diagnostics, describeCSIMockContainer(podName, phase, status))
	}
	if omitted > 0 {
		diagnostics = append(diagnostics, csiMockContainerDiagnostic{
			Pod:               podName,
			Phase:             phase,
			Observed:          csiSnapshotObservedTruncated,
			OmittedContainers: strconv.Itoa(omitted),
		})
	}
	return diagnostics
}

func describeCSIMockContainer(podName, phase string, status v1.ContainerStatus) csiMockContainerDiagnostic {
	diagnostic := csiMockContainerDiagnostic{
		Pod:          podName,
		Phase:        phase,
		Container:    status.Name,
		Observed:     csiSnapshotObservedFound,
		Ready:        describeBool(status.Ready),
		RestartCount: strconv.Itoa(int(status.RestartCount)),
	}

	// Only the short kubelet reasons are taken; the accompanying
	// human-readable messages can embed arbitrary driver output. The reasons
	// themselves are not schema constrained either, so they are rendered
	// through sanitizeDiagnosticValue like every other value.
	switch {
	case status.State.Running != nil:
		diagnostic.State = "running"
	case status.State.Waiting != nil:
		diagnostic.State = "waiting"
		diagnostic.StateReason = status.State.Waiting.Reason
	case status.State.Terminated != nil:
		diagnostic.State = "terminated"
		diagnostic.StateReason = status.State.Terminated.Reason
	default:
		diagnostic.State = csiSnapshotFieldUnset
	}

	if terminated := status.LastTerminationState.Terminated; terminated != nil {
		diagnostic.LastTerminationReason = terminated.Reason
		diagnostic.LastTerminationExitCode = strconv.Itoa(int(terminated.ExitCode))
		if terminated.Signal != 0 {
			diagnostic.LastTerminationSignal = strconv.Itoa(int(terminated.Signal))
		}
	}
	return diagnostic
}

// describeReadError maps a failed read to an "observed" value and a short,
// enumerated reason. The error itself is never rendered: API errors echo server
// messages, which keeps them outside the allowlist.
func describeReadError(err error) (observed string, reason string) {
	switch {
	case err == nil:
		return csiSnapshotObservedFound, ""
	case apierrors.IsNotFound(err):
		return csiSnapshotObservedAbsent, ""
	case errors.Is(err, context.DeadlineExceeded):
		return csiSnapshotObservedLookupFailed, csiSnapshotReasonTimeout
	case errors.Is(err, context.Canceled):
		return csiSnapshotObservedLookupFailed, csiSnapshotReasonCanceled
	}
	// Only the enumerated reason is kept: the message of an API error echoes
	// whatever the server put in it.
	if statusReason := apierrors.ReasonForError(err); statusReason != metav1.StatusReasonUnknown {
		return csiSnapshotObservedLookupFailed, string(statusReason)
	}
	return csiSnapshotObservedLookupFailed, csiSnapshotReasonUnknown
}

// describeBool renders a known boolean.
func describeBool(value bool) string {
	if value {
		return csiSnapshotFieldTrue
	}
	return csiSnapshotFieldFalse
}

// describeOptionalBool renders a boolean that the object may not carry.
func describeOptionalBool(value, found bool) string {
	if !found {
		return csiSnapshotFieldUnset
	}
	return describeBool(value)
}

// describeUIDLinkage reports whether a VolumeSnapshotContent really belongs to
// the VolumeSnapshot the test created.
func describeUIDLinkage(snapshotUID, referencedUID string) string {
	if snapshotUID == "" || referencedUID == "" {
		return csiSnapshotLinkageUnknown
	}
	if snapshotUID == referencedUID {
		return csiSnapshotLinkageMatch
	}
	return csiSnapshotLinkageMismatch
}
