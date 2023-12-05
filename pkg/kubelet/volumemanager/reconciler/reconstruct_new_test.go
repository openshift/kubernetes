/*
Copyright 2022 The Kubernetes Authors.

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

package reconciler

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/volume"
	volumetesting "k8s.io/kubernetes/pkg/volume/testing"
	"k8s.io/kubernetes/pkg/volume/util"
)

func verifyTearDownCalls(plugin *volumetesting.FakeVolumePlugin, expected int) error {
	unmounters := plugin.GetUnmounters()
	if len(unmounters) == 0 && (expected == 0) {
		return nil
	}
	actualCallCount := 0
	for _, unmounter := range unmounters {
		actualCallCount = unmounter.GetTearDownCallCount()
		if actualCallCount == expected {
			return nil
		}
	}
	return fmt.Errorf("expected TearDown calls %d, got %d", expected, actualCallCount)
}

func TestReconstructVolumesMount(t *testing.T) {
	tests := []struct {
		name            string
		volumePath      string
		expectMount     bool
		volumeMode      v1.PersistentVolumeMode
		deviceMountPath string
	}{
		{
			name:       "reconstructed volume is mounted",
			volumePath: path.Join("pod1uid", "volumes", "fake-plugin", "volumename"),

			expectMount: true,
			volumeMode:  v1.PersistentVolumeFilesystem,
		},
		{
			name: "reconstructed volume fails to mount",
			// FailOnSetupVolumeName: MountDevice succeeds, SetUp fails
			volumePath:  path.Join("pod1uid", "volumes", "fake-plugin", volumetesting.FailOnSetupVolumeName),
			expectMount: false,
			volumeMode:  v1.PersistentVolumeFilesystem,
		},
		{
			name:            "reconstructed volume device map fails",
			volumePath:      filepath.Join("pod1uid", "volumeDevices", "fake-plugin", volumetesting.FailMountDeviceVolumeName),
			volumeMode:      v1.PersistentVolumeBlock,
			deviceMountPath: filepath.Join("plugins", "fake-plugin", "volumeDevices", "pluginDependentPath"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpKubeletDir, err := os.MkdirTemp("", "")
			if err != nil {
				t.Fatalf("can't make a temp directory for kubeletPods: %v", err)
			}
			defer os.RemoveAll(tmpKubeletDir)

			// create kubelet pod directory
			tmpKubeletPodDir := filepath.Join(tmpKubeletDir, "pods")
			os.MkdirAll(tmpKubeletPodDir, 0755)

			// create pod and volume directories so as reconciler can find them.
			vp := filepath.Join(tmpKubeletPodDir, tc.volumePath)
			mountPaths := []string{vp}
			os.MkdirAll(vp, 0755)

			// Arrange 2 - populate DSW
			outerName := filepath.Base(tc.volumePath)
			pod, pv, pvc := getPodPVCAndPV(tc.volumeMode, "pod1", outerName, "pvc1")
			volumeSpec := &volume.Spec{PersistentVolume: pv}
			kubeClient := createtestClientWithPVPVC(pv, pvc, v1.AttachedVolume{
				Name:       v1.UniqueVolumeName(fmt.Sprintf("fake-plugin/%s", outerName)),
				DevicePath: "fake/path",
			})

			rc, fakePlugin := getReconciler(tmpKubeletDir, t, mountPaths, kubeClient /*custom kubeclient*/)
			rcInstance, _ := rc.(*reconciler)
			podName := util.GetUniquePodName(pod)
			volumeName, err := rcInstance.desiredStateOfWorld.AddPodToVolume(
				podName, pod, volumeSpec, volumeSpec.Name(), "" /* volumeGidValue */, nil /* SELinuxContext */)
			if err != nil {
				t.Fatalf("Error adding volume %s to dsow: %v", volumeSpec.Name(), err)
			}

			// Act 1 - reconstruction
			rcInstance.syncStates(tmpKubeletPodDir)
			rcInstance.actualStateOfWorld.MarkVolumeAsAttached(volumeName, volumeSpec, nodeName, "")
			rcInstance.processReconstructedVolumes()

			// Assert 1 - the volume is Uncertain
			mountedPods := rcInstance.actualStateOfWorld.GetMountedVolumes()
			if len(mountedPods) != 0 {
				t.Errorf("expected 0 mounted volumes, got %+v", mountedPods)
			}
			allPods := rcInstance.actualStateOfWorld.GetAllMountedVolumes()
			if len(allPods) != 1 {
				t.Errorf("expected 1 uncertain volume in asw, got %+v", allPods)
			}

			rcInstance.populatorHasAddedPods = func() bool {
				// Mark DSW populated to allow unmounting of volumes.
				return true
			}
			// Mark devices paths as reconciled to allow unmounting of volumes.
			// rcInstance.volumesNeedDevicePath = nil

			// Act 2 - reconcile once
			rcInstance.reconcile()

			// Assert 2
			// MountDevice was attempted
			var lastErr error
			err = retryWithExponentialBackOff(testOperationBackOffDuration, func() (bool, error) {
				if tc.volumeMode == v1.PersistentVolumeFilesystem {
					if err := volumetesting.VerifyMountDeviceCallCount(1, fakePlugin); err != nil {
						lastErr = err
						return false, nil
					}
					return true, nil
				} else {
					return true, nil
				}
			})
			if err != nil {
				t.Errorf("Error waiting for volumes to get mounted: %s: %s", err, lastErr)
			}

			if tc.expectMount {
				// The volume should be fully mounted
				waitForMount(t, fakePlugin, volumeName, rcInstance.actualStateOfWorld)
				// SetUp was called and succeeded
				if err := volumetesting.VerifySetUpCallCount(1, fakePlugin); err != nil {
					t.Errorf("Expected SetUp() to be called, got %s", err)
				}
			} else {
				// The test does not expect any change in ASW, yet it needs to wait for volume operations to finish
				err = retryWithExponentialBackOff(testOperationBackOffDuration, func() (bool, error) {
					return !rcInstance.operationExecutor.IsOperationPending(volumeName, "pod1uid", nodeName), nil
				})
				if err != nil {
					t.Errorf("Error waiting for operation to get finished: %s", err)
				}
				// The volume is uncertain
				mountedPods := rcInstance.actualStateOfWorld.GetMountedVolumes()
				if len(mountedPods) != 0 {
					t.Errorf("expected 0 mounted volumes after reconcile, got %+v", mountedPods)
				}
				allPods := rcInstance.actualStateOfWorld.GetAllMountedVolumes()
				if len(allPods) != 1 {
					t.Errorf("expected 1 mounted or uncertain volumes after reconcile, got %+v", allPods)
				}
				if tc.deviceMountPath != "" {
					expectedDeviceMountPath := filepath.Join(tmpKubeletDir, tc.deviceMountPath)
					deviceMountPath := allPods[0].DeviceMountPath
					if expectedDeviceMountPath != deviceMountPath {
						t.Errorf("expected deviceMountPath to be %s, got %s", expectedDeviceMountPath, deviceMountPath)
					}
				}

			}

			// Unmount was *not* attempted in any case
			verifyTearDownCalls(fakePlugin, 0)
		})
	}
}

func getPodPVCAndPV(volumeMode v1.PersistentVolumeMode, podName, pvName, pvcName string) (*v1.Pod, *v1.PersistentVolume, *v1.PersistentVolumeClaim) {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			UID:  "pvuid",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef:   &v1.ObjectReference{Name: pvcName},
			VolumeMode: &volumeMode,
		},
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvcName,
			UID:  "pvcuid",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: pvName,
			VolumeMode: &volumeMode,
		},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
			UID:  "pod1uid",
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "volume-name",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc.Name,
						},
					},
				},
			},
		},
	}
	return pod, pv, pvc
}
