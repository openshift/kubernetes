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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/kubernetes/test/e2e/storage/utils"
)

// forbiddenMarker tags every value the diagnostics must refuse to render:
// Secret data, the per-test Secret name, the snapshotter secret coordinates the
// external snapshotter writes into VolumeSnapshotContent annotations, and the
// free-form messages carried by status.error, API errors, pod status and
// container statuses.
const forbiddenMarker = "must-not-leak"

const (
	forbiddenSecretValue     = "s3cr3t-value-" + forbiddenMarker
	forbiddenSecretName      = "snapshot-secret-" + forbiddenMarker
	forbiddenErrorMessage    = "Failed to create snapshot: rpc error: code = DeadlineExceeded desc = fake error " + forbiddenMarker
	forbiddenAPIMessage      = "etcd said no, and included a token " + forbiddenMarker
	forbiddenKubeletMessage  = "back-off 5m0s restarting failed container " + forbiddenMarker
	forbiddenAnnotationValue = "annotation-" + forbiddenMarker
)

func forbiddenStrings() []string {
	return []string{
		forbiddenSecretValue,
		forbiddenSecretName,
		forbiddenErrorMessage,
		forbiddenAPIMessage,
		forbiddenKubeletMessage,
		forbiddenAnnotationValue,
		forbiddenMarker,
	}
}

func diagnosticSnapshotListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		utils.SnapshotGVR:        "VolumeSnapshotList",
		utils.SnapshotContentGVR: "VolumeSnapshotContentList",
	}
}

// diagnosticVolumeSnapshot builds a VolumeSnapshot carrying secret references,
// annotations and a status error message, i.e. everything the diagnostics must
// refuse to render.
func diagnosticVolumeSnapshot() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": utils.SnapshotAPIVersion,
		"kind":       "VolumeSnapshot",
		"metadata": map[string]interface{}{
			"name":              "snapshot",
			"namespace":         "test-ns",
			"uid":               "snapshot-uid",
			"resourceVersion":   "4242",
			"creationTimestamp": "2026-11-26T10:11:12Z",
			"labels":            map[string]interface{}{"example.com/owner": forbiddenAnnotationValue},
			"annotations": map[string]interface{}{
				"csi.storage.k8s.io/snapshotter-secret-name": forbiddenSecretName,
				"example.com/other":                          forbiddenAnnotationValue,
			},
		},
		"spec": map[string]interface{}{
			"volumeSnapshotClassName": "class",
			"source":                  map[string]interface{}{"persistentVolumeClaimName": "snapshot-test-pvc-" + forbiddenMarker},
		},
		"status": map[string]interface{}{
			"boundVolumeSnapshotContentName": "snapcontent-snapshot-uid",
			"readyToUse":                     false,
			"error": map[string]interface{}{
				"message": forbiddenErrorMessage,
				"time":    "2026-11-26T10:12:13Z",
			},
		},
	}}
}

func diagnosticVolumeSnapshotContent(referencedUID string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": utils.SnapshotAPIVersion,
		"kind":       "VolumeSnapshotContent",
		"metadata": map[string]interface{}{
			"name":              "snapcontent-snapshot-uid",
			"uid":               "content-uid",
			"resourceVersion":   "4243",
			"creationTimestamp": "2026-11-26T10:11:13Z",
			"annotations": map[string]interface{}{
				"snapshot.storage.kubernetes.io/deletion-secret-name":      forbiddenSecretName,
				"snapshot.storage.kubernetes.io/deletion-secret-namespace": forbiddenAnnotationValue,
			},
		},
		"spec": map[string]interface{}{
			"volumeSnapshotRef": map[string]interface{}{
				"name":      "snapshot",
				"namespace": "test-ns",
				"uid":       referencedUID,
			},
		},
		"status": map[string]interface{}{
			"readyToUse": false,
			"error": map[string]interface{}{
				"message": forbiddenErrorMessage,
			},
		},
	}}
}

func TestCollectCSISnapshotDiagnostics(t *testing.T) {
	t.Parallel()

	withoutStatus := diagnosticVolumeSnapshot()
	delete(withoutStatus.Object, "status")

	withoutCreationTimestamp := diagnosticVolumeSnapshot()
	unstructured.RemoveNestedField(withoutCreationTimestamp.Object, "metadata", "creationTimestamp")

	deleting := diagnosticVolumeSnapshot()
	if err := unstructured.SetNestedField(deleting.Object, "2026-11-26T10:13:14Z", "metadata", "deletionTimestamp"); err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(deleting.Object, true, "status", "readyToUse"); err != nil {
		t.Fatal(err)
	}
	unstructured.RemoveNestedField(deleting.Object, "status", "error")

	// A status that does not match the CRD schema: readyToUse is a string,
	// boundVolumeSnapshotContentName is a number and error is a scalar.
	malformed := diagnosticVolumeSnapshot()
	malformed.Object["status"] = map[string]interface{}{
		"readyToUse":                     "true",
		"boundVolumeSnapshotContentName": int64(42),
		"error":                          forbiddenErrorMessage,
	}

	statusNotAnObject := diagnosticVolumeSnapshot()
	statusNotAnObject.Object["status"] = forbiddenErrorMessage

	malformedContent := diagnosticVolumeSnapshotContent("")
	if err := unstructured.SetNestedField(malformedContent.Object, int64(7), "spec", "volumeSnapshotRef", "uid"); err != nil {
		t.Fatal(err)
	}

	// status.boundVolumeSnapshotContentName is a plain string in the CRD, so
	// nothing stops a controller from putting free-form text there.
	freeFormBoundContent := diagnosticVolumeSnapshot()
	if err := unstructured.SetNestedField(freeFormBoundContent.Object, forbiddenErrorMessage, "status", "boundVolumeSnapshotContentName"); err != nil {
		t.Fatal(err)
	}

	freeFormRefUID := diagnosticVolumeSnapshotContent("uid " + forbiddenErrorMessage)

	const (
		foundSnapshotLine = "kind=VolumeSnapshot namespace=test-ns name=snapshot observed=found uid=snapshot-uid resourceVersion=4242 creationTimestamp=2026-11-26T10:11:12Z deleting=false readyToUse=false hasError=true boundVolumeSnapshotContentName=snapcontent-snapshot-uid"
		foundContentLine  = "kind=VolumeSnapshotContent name=snapcontent-snapshot-uid observed=found uid=content-uid resourceVersion=4243 creationTimestamp=2026-11-26T10:11:13Z deleting=false readyToUse=false hasError=true volumeSnapshotRefUID=%s uidLinkage=%s"
	)

	for _, testCase := range []struct {
		name            string
		objects         []runtime.Object
		snapshotError   error
		contentError    error
		wantLines       []string
		wantContentRead bool
	}{
		{
			name:            "bound and linked content",
			objects:         []runtime.Object{diagnosticVolumeSnapshot(), diagnosticVolumeSnapshotContent("snapshot-uid")},
			wantContentRead: true,
			wantLines: []string{
				foundSnapshotLine,
				fmt.Sprintf(foundContentLine, "snapshot-uid", "match"),
			},
		},
		{
			name:            "content belongs to another snapshot",
			objects:         []runtime.Object{diagnosticVolumeSnapshot(), diagnosticVolumeSnapshotContent("stale-uid")},
			wantContentRead: true,
			wantLines: []string{
				foundSnapshotLine,
				fmt.Sprintf(foundContentLine, "stale-uid", "mismatch"),
			},
		},
		{
			name:            "content without a snapshot reference",
			objects:         []runtime.Object{diagnosticVolumeSnapshot(), diagnosticVolumeSnapshotContent("")},
			wantContentRead: true,
			wantLines: []string{
				foundSnapshotLine,
				"kind=VolumeSnapshotContent name=snapcontent-snapshot-uid observed=found uid=content-uid resourceVersion=4243 creationTimestamp=2026-11-26T10:11:13Z deleting=false readyToUse=false hasError=true uidLinkage=unknown",
			},
		},
		{
			name:            "bound content never created",
			objects:         []runtime.Object{diagnosticVolumeSnapshot()},
			wantContentRead: true,
			wantLines: []string{
				foundSnapshotLine,
				"kind=VolumeSnapshotContent name=snapcontent-snapshot-uid observed=absent uidLinkage=unknown",
			},
		},
		{
			name:            "content read rejected by the server",
			objects:         []runtime.Object{diagnosticVolumeSnapshot()},
			contentError:    apierrors.NewInternalError(fmt.Errorf("%s", forbiddenAPIMessage)),
			wantContentRead: true,
			wantLines: []string{
				foundSnapshotLine,
				"kind=VolumeSnapshotContent name=snapcontent-snapshot-uid observed=lookup-failed failureReason=InternalError uidLinkage=unknown",
			},
		},
		{
			name:    "snapshot without status",
			objects: []runtime.Object{withoutStatus},
			wantLines: []string{
				"kind=VolumeSnapshot namespace=test-ns name=snapshot observed=found uid=snapshot-uid resourceVersion=4242 creationTimestamp=2026-11-26T10:11:12Z deleting=false readyToUse=unset hasError=false",
				"kind=VolumeSnapshotContent observed=unbound",
			},
		},
		{
			name:    "snapshot without a creation timestamp",
			objects: []runtime.Object{withoutCreationTimestamp},
			wantLines: []string{
				"kind=VolumeSnapshot namespace=test-ns name=snapshot observed=found uid=snapshot-uid resourceVersion=4242 creationTimestamp=unset deleting=false readyToUse=false hasError=true boundVolumeSnapshotContentName=snapcontent-snapshot-uid",
				"kind=VolumeSnapshotContent name=snapcontent-snapshot-uid observed=absent uidLinkage=unknown",
			},
			wantContentRead: true,
		},
		{
			name:    "snapshot with a status that does not match the schema",
			objects: []runtime.Object{malformed},
			wantLines: []string{
				"kind=VolumeSnapshot namespace=test-ns name=snapshot observed=found uid=snapshot-uid resourceVersion=4242 creationTimestamp=2026-11-26T10:11:12Z deleting=false readyToUse=unset hasError=true malformed=true",
				// The bound content name could not be read, so the content is
				// reported as unknown rather than as unbound.
				"kind=VolumeSnapshotContent observed=unknown",
			},
		},
		{
			name:    "snapshot whose status is not an object",
			objects: []runtime.Object{statusNotAnObject},
			wantLines: []string{
				"kind=VolumeSnapshot namespace=test-ns name=snapshot observed=found uid=snapshot-uid resourceVersion=4242 creationTimestamp=2026-11-26T10:11:12Z deleting=false readyToUse=unset hasError=false malformed=true",
				"kind=VolumeSnapshotContent observed=unknown",
			},
		},
		{
			name:            "content with a snapshot reference that does not match the schema",
			objects:         []runtime.Object{diagnosticVolumeSnapshot(), malformedContent},
			wantContentRead: true,
			wantLines: []string{
				foundSnapshotLine,
				"kind=VolumeSnapshotContent name=snapcontent-snapshot-uid observed=found uid=content-uid resourceVersion=4243 creationTimestamp=2026-11-26T10:11:13Z deleting=false readyToUse=false hasError=true uidLinkage=unknown malformed=true",
			},
		},
		{
			name:            "snapshot being deleted and ready",
			objects:         []runtime.Object{deleting, diagnosticVolumeSnapshotContent("snapshot-uid")},
			wantContentRead: true,
			wantLines: []string{
				"kind=VolumeSnapshot namespace=test-ns name=snapshot observed=found uid=snapshot-uid resourceVersion=4242 creationTimestamp=2026-11-26T10:11:12Z deleting=true readyToUse=true hasError=false boundVolumeSnapshotContentName=snapcontent-snapshot-uid",
				fmt.Sprintf(foundContentLine, "snapshot-uid", "match"),
			},
		},
		{
			name:    "bound content name that is not a plain token is neither rendered nor looked up",
			objects: []runtime.Object{freeFormBoundContent},
			wantLines: []string{
				"kind=VolumeSnapshot namespace=test-ns name=snapshot observed=found uid=snapshot-uid resourceVersion=4242 creationTimestamp=2026-11-26T10:11:12Z deleting=false readyToUse=false hasError=true malformed=true",
				"kind=VolumeSnapshotContent observed=unknown",
			},
		},
		{
			name:            "snapshot reference uid that is not a plain token is redacted",
			objects:         []runtime.Object{diagnosticVolumeSnapshot(), freeFormRefUID},
			wantContentRead: true,
			wantLines: []string{
				foundSnapshotLine,
				"kind=VolumeSnapshotContent name=snapcontent-snapshot-uid observed=found uid=content-uid resourceVersion=4243 creationTimestamp=2026-11-26T10:11:13Z deleting=false readyToUse=false hasError=true volumeSnapshotRefUID=redacted uidLinkage=mismatch",
			},
		},
		{
			name: "snapshot already gone",
			wantLines: []string{
				"kind=VolumeSnapshot namespace=test-ns name=snapshot observed=absent",
				"kind=VolumeSnapshotContent observed=unknown",
			},
		},
		{
			name:          "snapshot read rejected by the server",
			snapshotError: apierrors.NewInternalError(fmt.Errorf("%s", forbiddenAPIMessage)),
			wantLines: []string{
				"kind=VolumeSnapshot namespace=test-ns name=snapshot observed=lookup-failed failureReason=InternalError",
				"kind=VolumeSnapshotContent observed=unknown",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), diagnosticSnapshotListKinds(), testCase.objects...)
			if testCase.snapshotError != nil {
				client.PrependReactor("get", "volumesnapshots", func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, testCase.snapshotError
				})
			}
			if testCase.contentError != nil {
				client.PrependReactor("get", "volumesnapshotcontents", func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, testCase.contentError
				})
			}

			diagnostics := collectCSISnapshotDiagnostics(context.Background(), client, "test-ns", "snapshot", time.Second)
			assertDiagnosticLines(t, diagnosticStrings(diagnostics), testCase.wantLines)
			assertReadOnlySnapshotActions(t, client.Actions())

			contentRead := false
			for _, action := range client.Actions() {
				if action.GetResource().Resource == "volumesnapshotcontents" {
					contentRead = true
				}
			}
			if contentRead != testCase.wantContentRead {
				t.Errorf("VolumeSnapshotContent read = %t, want %t", contentRead, testCase.wantContentRead)
			}
		})
	}
}

func TestCollectCSIMockPodDiagnostics(t *testing.T) {
	t.Parallel()

	plugin := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        csiMockPluginPodName,
			Namespace:   "driver-ns",
			Labels:      map[string]string{"app": "csi-mockplugin-" + forbiddenMarker},
			Annotations: map[string]string{"example.com/other": forbiddenAnnotationValue},
		},
		Spec: v1.PodSpec{
			NodeName: "node-" + forbiddenMarker,
			Containers: []v1.Container{{
				Name:  "mock",
				Image: "registry.example.com/mock:" + forbiddenMarker,
				Env:   []v1.EnvVar{{Name: "CSI_ENDPOINT", Value: forbiddenSecretValue}},
			}},
		},
		Status: v1.PodStatus{
			Phase:   v1.PodRunning,
			Reason:  forbiddenKubeletMessage,
			Message: forbiddenKubeletMessage,
			PodIP:   "10.0.0.1",
			InitContainerStatuses: []v1.ContainerStatus{{
				Name: "init", Ready: true, RestartCount: 0,
				ImageID: "sha256:" + forbiddenMarker,
				State:   v1.ContainerState{Terminated: &v1.ContainerStateTerminated{Reason: "Completed", Message: forbiddenKubeletMessage}},
			}},
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name: "mock", Ready: false, RestartCount: 3,
					ContainerID:          "cri-o://" + forbiddenMarker,
					State:                v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: forbiddenKubeletMessage}},
					LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: 137, Signal: 9, Reason: "OOMKilled", Message: forbiddenKubeletMessage}},
				},
				{
					Name: "csi-provisioner", Ready: true, RestartCount: 0,
					State: v1.ContainerState{Running: &v1.ContainerStateRunning{}},
				},
				{
					// A container the kubelet has not reported on yet.
					Name: "driver-registrar", Ready: false, RestartCount: 0,
				},
				{
					// ContainerState*.Reason is only documented as a "(brief)
					// reason": the kubelet fills it from what the runtime said,
					// so it is not guaranteed to be a short token.
					Name: "busybox", Ready: false, RestartCount: 1,
					State:                v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: "runtime said: " + forbiddenKubeletMessage}},
					LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: -1, Reason: forbiddenKubeletMessage}},
				},
			},
		},
	}
	snapshotter := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: csiMockSnapshotterPodName, Namespace: "driver-ns"},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			ContainerStatuses: []v1.ContainerStatus{{
				Name: "csi-snapshotter", Ready: true, RestartCount: 1,
				State:                v1.ContainerState{Running: &v1.ContainerStateRunning{}},
				LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: 1, Reason: "Error", Message: forbiddenKubeletMessage}},
			}},
		},
	}
	noContainerStatus := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: csiMockSnapshotterPodName, Namespace: "driver-ns"},
		Status:     v1.PodStatus{Phase: v1.PodPending},
	}

	crowded := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: csiMockPluginPodName, Namespace: "driver-ns"},
		Status:     v1.PodStatus{Phase: v1.PodRunning},
	}
	var crowdedLines []string
	for i := 0; i < csiSnapshotDiagnosticMaxContainers+2; i++ {
		crowded.Status.ContainerStatuses = append(crowded.Status.ContainerStatuses, v1.ContainerStatus{
			Name:  fmt.Sprintf("container-%d", i),
			Ready: true,
			State: v1.ContainerState{Running: &v1.ContainerStateRunning{}},
		})
		if i < csiSnapshotDiagnosticMaxContainers {
			crowdedLines = append(crowdedLines, fmt.Sprintf("pod=csi-mockplugin-0 phase=Running container=container-%d observed=found ready=true restartCount=0 state=running", i))
		}
	}
	crowdedLines = append(crowdedLines,
		"pod=csi-mockplugin-0 phase=Running observed=truncated omittedContainers=2",
		"pod=csi-mockplugin-snapshotter-0 observed=absent",
	)

	for _, testCase := range []struct {
		name            string
		driverNamespace string
		objects         []runtime.Object
		getError        error
		wantLines       []string
	}{
		{
			name:            "restart and termination state of both fixture pods",
			driverNamespace: "driver-ns",
			objects:         []runtime.Object{plugin, snapshotter},
			wantLines: []string{
				"pod=csi-mockplugin-0 phase=Running container=init observed=found ready=true restartCount=0 state=terminated stateReason=Completed",
				"pod=csi-mockplugin-0 phase=Running container=mock observed=found ready=false restartCount=3 state=waiting stateReason=CrashLoopBackOff lastTerminationReason=OOMKilled lastTerminationExitCode=137 lastTerminationSignal=9",
				"pod=csi-mockplugin-0 phase=Running container=csi-provisioner observed=found ready=true restartCount=0 state=running",
				"pod=csi-mockplugin-0 phase=Running container=driver-registrar observed=found ready=false restartCount=0 state=unset",
				"pod=csi-mockplugin-0 phase=Running container=busybox observed=found ready=false restartCount=1 state=waiting stateReason=redacted lastTerminationReason=redacted lastTerminationExitCode=-1",
				"pod=csi-mockplugin-snapshotter-0 phase=Running container=csi-snapshotter observed=found ready=true restartCount=1 state=running lastTerminationReason=Error lastTerminationExitCode=1",
			},
		},
		{
			name:            "missing pods are reported, not skipped",
			driverNamespace: "driver-ns",
			wantLines: []string{
				"pod=csi-mockplugin-0 observed=absent",
				"pod=csi-mockplugin-snapshotter-0 observed=absent",
			},
		},
		{
			name:            "pod without container statuses",
			driverNamespace: "driver-ns",
			objects:         []runtime.Object{noContainerStatus},
			wantLines: []string{
				"pod=csi-mockplugin-0 observed=absent",
				"pod=csi-mockplugin-snapshotter-0 phase=Pending observed=found",
			},
		},
		{
			name:            "container list is bounded",
			driverNamespace: "driver-ns",
			objects:         []runtime.Object{crowded},
			wantLines:       crowdedLines,
		},
		{
			name:            "pod read rejected by the server",
			driverNamespace: "driver-ns",
			getError:        apierrors.NewForbidden(v1.Resource("pods"), csiMockPluginPodName, fmt.Errorf("%s", forbiddenAPIMessage)),
			wantLines: []string{
				"pod=csi-mockplugin-0 observed=lookup-failed failureReason=Forbidden",
				"pod=csi-mockplugin-snapshotter-0 observed=lookup-failed failureReason=Forbidden",
			},
		},
		{
			name:            "no driver namespace to read",
			driverNamespace: "",
			objects:         []runtime.Object{plugin, snapshotter},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			client := fake.NewClientset(testCase.objects...)
			if testCase.getError != nil {
				client.PrependReactor("get", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, testCase.getError
				})
			}

			diagnostics := collectCSIMockPodDiagnostics(context.Background(), client, testCase.driverNamespace, time.Second)
			assertDiagnosticLines(t, diagnosticStrings(diagnostics), testCase.wantLines)

			if testCase.driverNamespace == "" && len(client.Actions()) != 0 {
				t.Errorf("without a driver namespace nothing must be read, got %#v", client.Actions())
			}
			for _, action := range client.Actions() {
				if action.GetVerb() != "get" || action.GetResource().Resource != "pods" || action.GetSubresource() != "" {
					t.Errorf("pod diagnostics must only get pods, got %#v", action)
				}
			}
		})
	}
}

// TestLogCSISnapshotFailureDiagnosticsRedaction runs the whole capture against
// objects stuffed with secrets, annotations and free-form messages and checks
// the test output for leaks.
func TestLogCSISnapshotFailureDiagnosticsRedaction(t *testing.T) {
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), diagnosticSnapshotListKinds(),
		diagnosticVolumeSnapshot(), diagnosticVolumeSnapshotContent("snapshot-uid"))
	coreClient := fake.NewClientset(
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        csiMockPluginPodName,
				Namespace:   "driver-ns",
				Labels:      map[string]string{"app": "csi-mockplugin-" + forbiddenMarker},
				Annotations: map[string]string{"example.com/other": forbiddenAnnotationValue},
			},
			Spec: v1.PodSpec{
				NodeName:   "node-" + forbiddenMarker,
				Containers: []v1.Container{{Name: "mock", Env: []v1.EnvVar{{Name: "CSI_ENDPOINT", Value: forbiddenSecretValue}}}},
			},
			Status: v1.PodStatus{
				Phase:   v1.PodRunning,
				Reason:  forbiddenKubeletMessage,
				Message: forbiddenKubeletMessage,
				ContainerStatuses: []v1.ContainerStatus{{
					Name: "mock", RestartCount: 2,
					ContainerID:          "cri-o://" + forbiddenMarker,
					State:                v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: forbiddenKubeletMessage}},
					LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: 2, Reason: "Error", Message: forbiddenKubeletMessage}},
				}},
			},
		},
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: forbiddenSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"secret-data": []byte(forbiddenSecretValue)},
		},
	)

	output := captureGinkgoOutput(t, func() {
		logCSISnapshotFailureDiagnostics(context.Background(), dynamicClient, coreClient, "test-ns", "snapshot", "driver-ns", time.Second, 5*time.Second)
	})

	assertNoLeak(t, output)
	for _, want := range []string{
		"CSI mock snapshot failure diagnostics: kind=VolumeSnapshot namespace=test-ns name=snapshot observed=found uid=snapshot-uid resourceVersion=4242 creationTimestamp=2026-11-26T10:11:12Z deleting=false readyToUse=false hasError=true boundVolumeSnapshotContentName=snapcontent-snapshot-uid",
		"CSI mock snapshot failure diagnostics: kind=VolumeSnapshotContent name=snapcontent-snapshot-uid observed=found uid=content-uid resourceVersion=4243 creationTimestamp=2026-11-26T10:11:13Z deleting=false readyToUse=false hasError=true volumeSnapshotRefUID=snapshot-uid uidLinkage=match",
		"CSI mock snapshot failure diagnostics: pod=csi-mockplugin-0 phase=Running container=mock observed=found ready=false restartCount=2 state=waiting stateReason=CrashLoopBackOff lastTerminationReason=Error lastTerminationExitCode=2",
		"CSI mock snapshot failure diagnostics: pod=csi-mockplugin-snapshotter-0 observed=absent",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("missing diagnostic line %q:\n%s", want, output)
		}
	}

	assertReadOnlySnapshotActions(t, dynamicClient.Actions())
	for _, action := range coreClient.Actions() {
		if action.GetResource().Resource != "pods" || action.GetVerb() != "get" || action.GetSubresource() != "" {
			t.Errorf("diagnostics must only get pods, got %#v", action)
		}
	}
}

// TestLogCSISnapshotFailureDiagnosticsBounded points the capture at a server
// that never answers. Each case makes a different limit the binding one, so
// both the per-read timeout and the overall budget are load bearing.
func TestLogCSISnapshotFailureDiagnosticsBounded(t *testing.T) {
	for _, testCase := range []struct {
		name string
		// readTimeout and budget are deliberately far apart so only one of
		// them can end the capture, and maxElapsed stays well above the limit
		// under test so scheduler jitter cannot fail the assertion.
		readTimeout  time.Duration
		budget       time.Duration
		maxElapsed   time.Duration
		wantCanceled bool
	}{
		{name: "each read is bounded", readTimeout: 150 * time.Millisecond, budget: time.Minute, maxElapsed: 20 * time.Second, wantCanceled: true},
		{name: "the whole collection is bounded", readTimeout: time.Minute, budget: 400 * time.Millisecond, maxElapsed: 20 * time.Second},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			canceled := make(chan struct{}, 8)
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				<-request.Context().Done()
				select {
				case canceled <- struct{}{}:
				default:
				}
			}))
			t.Cleanup(server.Close)

			config := &rest.Config{Host: server.URL}
			dynamicClient, err := dynamic.NewForConfig(config)
			if err != nil {
				t.Fatal(err)
			}
			coreClient, err := clientset.NewForConfig(config)
			if err != nil {
				t.Fatal(err)
			}

			// Cleanup nodes can run after the spec context was canceled; the
			// capture must still happen and must still be bounded.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			start := time.Now()
			output := captureGinkgoOutput(t, func() {
				logCSISnapshotFailureDiagnostics(ctx, dynamicClient, coreClient, "test-ns", "snapshot", "driver-ns", testCase.readTimeout, testCase.budget)
			})
			if elapsed := time.Since(start); elapsed >= testCase.maxElapsed {
				t.Errorf("capture took %s, which no limit can explain", elapsed)
			}

			assertNoLeak(t, output)
			for _, want := range []string{
				"kind=VolumeSnapshot namespace=test-ns name=snapshot observed=lookup-failed failureReason=Timeout",
				"kind=VolumeSnapshotContent observed=unknown",
				"pod=csi-mockplugin-0 observed=lookup-failed failureReason=Timeout",
				"pod=csi-mockplugin-snapshotter-0 observed=lookup-failed failureReason=Timeout",
			} {
				if !strings.Contains(output, want) {
					t.Errorf("missing diagnostic line %q:\n%s", want, output)
				}
			}

			// The per-read deadline must reach the server: a stalled request
			// has to be canceled rather than left hanging. Only asserted where
			// several reads get their own deadline; when the overall budget is
			// the binding limit, how many requests are issued at all depends on
			// how much of it is left.
			if testCase.wantCanceled {
				select {
				case <-canceled:
				case <-time.After(30 * time.Second):
					t.Fatal("the stalled request was never canceled")
				}
			}
		})
	}
}

func TestDescribeReadError(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name         string
		err          error
		wantObserved string
		wantReason   string
	}{
		{name: "no error", wantObserved: csiSnapshotObservedFound},
		{
			name: "not found", err: apierrors.NewNotFound(v1.Resource("pods"), "mock"),
			wantObserved: csiSnapshotObservedAbsent,
		},
		{
			name: "deadline exceeded", err: fmt.Errorf("get: %w", context.DeadlineExceeded),
			wantObserved: csiSnapshotObservedLookupFailed, wantReason: "Timeout",
		},
		{
			name: "canceled", err: fmt.Errorf("get: %w", context.Canceled),
			wantObserved: csiSnapshotObservedLookupFailed, wantReason: "Canceled",
		},
		{
			name:         "server error keeps its reason but not its message",
			err:          apierrors.NewInternalError(fmt.Errorf("%s", forbiddenAPIMessage)),
			wantObserved: csiSnapshotObservedLookupFailed, wantReason: "InternalError",
		},
		{
			name:         "plain error has no reason to report",
			err:          fmt.Errorf("%s", forbiddenAPIMessage),
			wantObserved: csiSnapshotObservedLookupFailed, wantReason: "Unknown",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			observed, reason := describeReadError(testCase.err)
			if observed != testCase.wantObserved || reason != testCase.wantReason {
				t.Errorf("describeReadError() = (%q, %q), want (%q, %q)", observed, reason, testCase.wantObserved, testCase.wantReason)
			}
			assertNoLeak(t, observed+" "+reason)
		})
	}
}

func TestDescribeUIDLinkage(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		snapshotUID, referencedUID, want string
	}{
		{"uid", "uid", csiSnapshotLinkageMatch},
		{"uid", "other", csiSnapshotLinkageMismatch},
		{"", "uid", csiSnapshotLinkageUnknown},
		{"uid", "", csiSnapshotLinkageUnknown},
		{"", "", csiSnapshotLinkageUnknown},
	} {
		if got := describeUIDLinkage(testCase.snapshotUID, testCase.referencedUID); got != testCase.want {
			t.Errorf("describeUIDLinkage(%q, %q) = %q, want %q", testCase.snapshotUID, testCase.referencedUID, got, testCase.want)
		}
	}
}

// TestSanitizeDiagnosticValue pins the single point that decides what may be
// written, including that a rejected value leaves no byte of itself behind.
func TestSanitizeDiagnosticValue(t *testing.T) {
	t.Parallel()
	longToken := strings.Repeat("a", csiSnapshotDiagnosticMaxValueLength)

	for _, testCase := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: ""},
		{name: "object name", value: "snapcontent-8f1a2b3c-0000", want: "snapcontent-8f1a2b3c-0000"},
		{name: "rfc3339 timestamp", value: "2026-11-26T10:11:12Z", want: "2026-11-26T10:11:12Z"},
		{name: "kubelet reason", value: "CrashLoopBackOff", want: "CrashLoopBackOff"},
		{name: "negative exit code", value: "-1", want: "-1"},
		{name: "container id", value: "cri-o://abc123", want: "cri-o://abc123"},
		{name: "at the length limit", value: longToken, want: longToken},
		{name: "over the length limit", value: longToken + "bbb", want: longToken + csiSnapshotValueTruncatedSuffix},
		{name: "space", value: "back-off restarting failed container", want: csiSnapshotValueRedacted},
		{name: "newline", value: "Error\nsecret-value", want: csiSnapshotValueRedacted},
		{name: "tab", value: "Error\tsecret-value", want: csiSnapshotValueRedacted},
		{name: "equals sign", value: "reason=injected", want: csiSnapshotValueRedacted},
		{name: "quote", value: `Error"injected"`, want: csiSnapshotValueRedacted},
		{name: "control character", value: "Error\x00secret", want: csiSnapshotValueRedacted},
		{name: "non ascii", value: "Erreur-déjà", want: csiSnapshotValueRedacted},
		{name: "invalid utf8", value: "Error\xff\xfe", want: csiSnapshotValueRedacted},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeDiagnosticValue(testCase.value); got != testCase.want {
				t.Errorf("sanitizeDiagnosticValue(%q) = %q, want %q", testCase.value, got, testCase.want)
			}
		})
	}

	// A rejected value must not survive in any form, not even a fragment.
	rejected := "back-off restarting " + forbiddenSecretValue
	got := sanitizeDiagnosticValue(rejected)
	if got != csiSnapshotValueRedacted {
		t.Fatalf("sanitizeDiagnosticValue(%q) = %q, want %q", rejected, got, csiSnapshotValueRedacted)
	}
	assertNoLeak(t, got)
	for _, word := range strings.Fields(rejected) {
		if strings.Contains(got, word) {
			t.Errorf("rejected value kept %q: %s", word, got)
		}
	}
}

// TestRenderDiagnosticFieldsConstrainsEveryValue proves the sanitizer is
// applied by the renderer, so no diagnostic type can bypass it.
func TestRenderDiagnosticFieldsConstrainsEveryValue(t *testing.T) {
	t.Parallel()
	got := renderDiagnosticFields([]diagnosticField{
		{"name", "snapshot"},
		{"stateReason", "back-off restarting " + forbiddenKubeletMessage},
		{"uid", "snapshot-uid"},
	})
	if want := "name=snapshot stateReason=redacted uid=snapshot-uid"; got != want {
		t.Errorf("renderDiagnosticFields() = %q, want %q", got, want)
	}
	assertNoLeak(t, got)

	// Both rendered types route through the renderer, so a hostile value in
	// any field is constrained the same way.
	object := csiSnapshotObjectDiagnostic{Kind: "VolumeSnapshot", Name: forbiddenErrorMessage, Observed: csiSnapshotObservedFound}
	container := csiMockContainerDiagnostic{Pod: csiMockPluginPodName, StateReason: forbiddenKubeletMessage}
	for _, line := range []string{object.String(), container.String()} {
		assertNoLeak(t, line)
		if !strings.Contains(line, csiSnapshotValueRedacted) {
			t.Errorf("hostile value was not redacted: %s", line)
		}
	}
}

// TestLogCSISnapshotFailureDiagnosticsIfFailed pins the failure-only gate that
// the DeferCleanup node in csi_snapshot.go relies on.
func TestLogCSISnapshotFailureDiagnosticsIfFailed(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed=%t", failed), func(t *testing.T) {
			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), diagnosticSnapshotListKinds(),
				diagnosticVolumeSnapshot(), diagnosticVolumeSnapshotContent("snapshot-uid"))
			coreClient := fake.NewClientset()

			output := captureGinkgoOutput(t, func() {
				logCSISnapshotFailureDiagnosticsIfFailed(context.Background(), failed, dynamicClient, coreClient,
					"test-ns", "snapshot", "driver-ns", time.Second, 5*time.Second)
			})

			read := len(dynamicClient.Actions()) + len(coreClient.Actions())
			logged := strings.Contains(output, csiSnapshotDiagnosticLogPrefix)
			if !failed {
				if read != 0 || logged {
					t.Errorf("a passing spec must read and log nothing: actions=%d, output=%s", read, output)
				}
				return
			}
			if read == 0 || !logged {
				t.Errorf("a failed spec must capture: actions=%d, output=%s", read, output)
			}
		})
	}
}

func TestRenderDiagnosticFieldsDropsEmptyValues(t *testing.T) {
	t.Parallel()
	got := renderDiagnosticFields([]diagnosticField{
		{"kind", "VolumeSnapshot"},
		{"namespace", ""},
		{"name", "snapshot"},
	})
	if want := "kind=VolumeSnapshot name=snapshot"; got != want {
		t.Errorf("renderDiagnosticFields() = %q, want %q", got, want)
	}
}

func diagnosticStrings[T fmt.Stringer](diagnostics []T) []string {
	lines := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		lines = append(lines, diagnostic.String())
	}
	return lines
}

func assertDiagnosticLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(want) == 0 && len(got) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("diagnostics =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	for _, line := range got {
		assertNoLeak(t, line)
	}
}

// assertNoLeak fails when rendered diagnostics contain anything the fixtures
// marked as off limits.
func assertNoLeak(t *testing.T, rendered string) {
	t.Helper()
	for _, forbidden := range forbiddenStrings() {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("diagnostics leaked %q:\n%s", forbidden, rendered)
		}
	}
}

func assertReadOnlySnapshotActions(t *testing.T, actions []clienttesting.Action) {
	t.Helper()
	for _, action := range actions {
		if action.GetVerb() != "get" {
			t.Errorf("snapshot diagnostics must stay read-only, got %#v", action)
		}
		if resource := action.GetResource().Resource; resource != "volumesnapshots" && resource != "volumesnapshotcontents" {
			t.Errorf("snapshot diagnostics must not touch %q", resource)
		}
	}
}

// ginkgoWriterLock serializes the capture below: GinkgoWriter is process
// global, so two captures at once would see each other's output.
var ginkgoWriterLock sync.Mutex

// captureGinkgoOutput collects what framework.Logf writes while body runs.
func captureGinkgoOutput(t *testing.T, body func()) string {
	t.Helper()
	ginkgoWriterLock.Lock()
	defer ginkgoWriterLock.Unlock()
	var captured bytes.Buffer
	ginkgo.GinkgoWriter.TeeTo(&captured)
	defer ginkgo.GinkgoWriter.ClearTeeWriters()
	body()
	return captured.String()
}
