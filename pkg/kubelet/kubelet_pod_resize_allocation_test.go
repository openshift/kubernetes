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

package kubelet

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	containertest "k8s.io/kubernetes/pkg/kubelet/container/testing"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
	"k8s.io/kubernetes/test/utils/ktesting"
)

// TestSyncPodUsesAllocatedResources tests that SyncPod calls UpdatePodFromAllocation
// to ensure container status shows allocated resources, not desired resources.
//
// This reproduces the e2e test failure scenario where:
// - Pod spec has desired resources (1991m)
// - Allocation manager has allocated less (186m)
// - Without the fix: container status incorrectly shows 1991m
// - With the fix: container status correctly shows 186m
func TestSyncPodUsesAllocatedResources(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlacePodVerticalScaling, true)

	tCtx := ktesting.Init(t)
	testKubelet := newTestKubelet(t, false /* controllerAttachDetachEnabled */)
	defer testKubelet.Cleanup()
	kubelet := testKubelet.kubelet

	allocatedCPU := resource.MustParse("186m")
	desiredCPU := resource.MustParse("1991m")

	// Create a pod with desired resources in spec
	pod := podWithUIDNameNsSpec("test-pod-uid", "testpod2", "test-ns", v1.PodSpec{
		Containers: []v1.Container{
			{
				Name:  "c",
				Image: "test-image",
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU: desiredCPU, // Desired: 1991m
					},
					Limits: v1.ResourceList{
						v1.ResourceCPU: desiredCPU,
					},
				},
			},
		},
	})
	pod.Status.Phase = v1.PodRunning

	// Set up allocation manager with allocated resources (186m)
	allocatedPod := pod.DeepCopy()
	allocatedPod.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = allocatedCPU
	allocatedPod.Spec.Containers[0].Resources.Limits[v1.ResourceCPU] = allocatedCPU
	err := kubelet.allocationManager.SetAllocatedResources(allocatedPod)
	require.NoError(t, err, "Failed to set allocated resources")

	// Set pod in pod manager (this is what HandlePodSyncs gets)
	kubelet.podManager.SetPods([]*v1.Pod{pod})

	// Create a running container status from the runtime
	testKubelet.fakeRuntime.PodList = []*containertest.FakePod{
		{
			Pod: &kubecontainer.Pod{
				ID:        pod.UID,
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Containers: []*kubecontainer.Container{
					{
						ID:    kubecontainer.ContainerID{Type: "test", ID: "container-id"},
						Name:  "c",
						Image: "test-image",
						State: kubecontainer.ContainerStateRunning,
					},
				},
			},
		},
	}

	// Call SyncPod - this is where the fix applies
	// SyncPod should call UpdatePodFromAllocation to get allocated resources
	_, _, err = kubelet.SyncPod(tCtx, kubetypes.SyncPodUpdate, pod, nil, &kubecontainer.PodStatus{
		ID:        pod.UID,
		Name:      pod.Name,
		Namespace: pod.Namespace,
		ContainerStatuses: []*kubecontainer.Status{
			{
				ID:    kubecontainer.ContainerID{Type: "test", ID: "container-id"},
				Name:  "c",
				State: kubecontainer.ContainerStateRunning,
				Resources: &kubecontainer.ContainerResources{
					CPULimit:    &allocatedCPU, // Runtime reports allocated resources
					CPURequest:  &allocatedCPU,
					MemoryLimit: nil,
				},
			},
		},
	})

	// Allow errors from SyncPod (we're mainly testing the status generation)
	if err != nil {
		t.Logf("SyncPod returned error (expected in test environment): %v", err)
	}

	// Get the pod status that was set by SyncPod
	status, ok := kubelet.statusManager.GetPodStatus(pod.UID)
	require.True(t, ok, "Pod status should be set")

	// Verify that container status has allocated resources, not desired
	require.Len(t, status.ContainerStatuses, 1, "Should have one container status")
	containerStatus := status.ContainerStatuses[0]

	// Check allocatedResources field
	if allocatedRes, ok := containerStatus.AllocatedResources[v1.ResourceCPU]; ok {
		t.Logf("Container status AllocatedResources CPU: %v", allocatedRes.String())

		// With the fix in SyncPod (calling UpdatePodFromAllocation):
		// allocatedResources should be 186m (allocated), not 1991m (desired)
		if !allocatedRes.Equal(allocatedCPU) {
			t.Errorf("BUG: Container status shows AllocatedResources CPU = %v, expected %v (allocated). "+
				"This means SyncPod is not calling UpdatePodFromAllocation before generating pod status. "+
				"The pod spec has desired resources (%v), but the allocation manager allocated only %v.",
				allocatedRes.String(), allocatedCPU.String(), desiredCPU.String(), allocatedCPU.String())
		}

		if allocatedRes.Equal(desiredCPU) {
			t.Errorf("CRITICAL BUG: Container status shows desired resources (%v) instead of allocated resources (%v). "+
				"This is the exact bug from the failing e2e test! "+
				"SyncPod must call UpdatePodFromAllocation before using the pod for status generation.",
				desiredCPU.String(), allocatedCPU.String())
		}
	} else {
		t.Error("AllocatedResources[CPU] not set in container status")
	}

	// Also check the resources field
	if containerStatus.Resources != nil {
		if cpuLimit, ok := containerStatus.Resources.Limits[v1.ResourceCPU]; ok {
			t.Logf("Container status Resources.Limits CPU: %v", cpuLimit.String())

			if !cpuLimit.Equal(allocatedCPU) {
				t.Errorf("Container status Resources.Limits CPU = %v, expected %v",
					cpuLimit.String(), allocatedCPU.String())
			}
		}
	}
}
