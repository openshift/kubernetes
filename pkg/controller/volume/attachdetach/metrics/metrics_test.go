/*
Copyright 2018 The Kubernetes Authors.

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

package metrics

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	csitrans "k8s.io/csi-translation-lib"
	"k8s.io/kubernetes/pkg/controller/volume/attachdetach/cache"
	controllervolumetesting "k8s.io/kubernetes/pkg/controller/volume/attachdetach/testing"
	"k8s.io/kubernetes/pkg/volume/csimigration"
	volumetesting "k8s.io/kubernetes/pkg/volume/testing"
	"k8s.io/kubernetes/pkg/volume/util/types"
)

func TestTotalVolumesMetricCollection(t *testing.T) {
	fakeVolumePluginMgr, _ := volumetesting.GetTestVolumePluginMgr(t)
	dsw := cache.NewDesiredStateOfWorld(fakeVolumePluginMgr)
	asw := cache.NewActualStateOfWorld(fakeVolumePluginMgr)
	podName := "pod-uid"
	volumeName := v1.UniqueVolumeName("volume-name")
	volumeSpec := controllervolumetesting.GetTestVolumeSpec(string(volumeName), volumeName)
	nodeName := k8stypes.NodeName("node-name")

	dsw.AddNode(nodeName, false)
	_, err := dsw.AddPod(types.UniquePodName(podName), controllervolumetesting.NewPod(podName, podName), volumeSpec, nodeName)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	asw.AddVolumeNode(volumeName, volumeSpec, nodeName, "", true)

	csiTranslator := csitrans.New()
	metricCollector := newAttachDetachStateCollector(
		nil,
		nil,
		nil,
		asw,
		dsw,
		fakeVolumePluginMgr,
		csimigration.NewPluginManager(csiTranslator),
		csiTranslator)

	totalVolumesMap := metricCollector.getTotalVolumesCount()
	if len(totalVolumesMap) != 2 {
		t.Errorf("Expected 2 states, got %d", len(totalVolumesMap))
	}

	dswCount, ok := totalVolumesMap["desired_state_of_world"]
	if !ok {
		t.Errorf("Expected desired_state_of_world, got nothing")
	}

	fakePluginCount := dswCount["fake-plugin"]
	if fakePluginCount != 1 {
		t.Errorf("Expected 1 fake-plugin volume in DesiredStateOfWorld, got %d", fakePluginCount)
	}

	aswCount, ok := totalVolumesMap["actual_state_of_world"]
	if !ok {
		t.Errorf("Expected actual_state_of_world, got nothing")
	}

	fakePluginCount = aswCount["fake-plugin"]
	if fakePluginCount != 1 {
		t.Errorf("Expected 1 fake-plugin volume in ActualStateOfWorld, got %d", fakePluginCount)
	}
}
