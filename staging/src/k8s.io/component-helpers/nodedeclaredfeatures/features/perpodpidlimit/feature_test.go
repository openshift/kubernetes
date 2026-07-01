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

package perpodpidlimit

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/component-helpers/nodedeclaredfeatures"
)

type mockFeatureGate map[string]bool

func (m mockFeatureGate) Enabled(key string) bool {
	return m[key]
}

func TestPerPodPIDLimitFeature_Discover(t *testing.T) {
	tests := []struct {
		name        string
		featureGate bool
		expected    bool
	}{
		{
			name:        "GateEnabled",
			featureGate: true,
			expected:    true,
		},
		{
			name:        "GateDisabled",
			featureGate: false,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &nodedeclaredfeatures.NodeConfiguration{
				FeatureGates: mockFeatureGate{PerPodPIDLimitFeatureGate: tt.featureGate},
			}
			if got := Feature.Discover(cfg); got != tt.expected {
				t.Fatalf("Discover() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPerPodPIDLimitFeature_InferForScheduling(t *testing.T) {
	tests := []struct {
		name     string
		podInfo  *nodedeclaredfeatures.PodInfo
		expected bool
	}{
		{
			name: "PodWithPIDLimit",
			podInfo: &nodedeclaredfeatures.PodInfo{
				Spec: &v1.PodSpec{
					Resources: &v1.ResourceRequirements{
						Limits: v1.ResourceList{v1.ResourcePID: resource.MustParse("2048")},
					},
				},
			},
			expected: true,
		},
		{
			name: "PodWithoutPIDLimit",
			podInfo: &nodedeclaredfeatures.PodInfo{
				Spec: &v1.PodSpec{
					Resources: &v1.ResourceRequirements{
						Limits: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
					},
				},
			},
			expected: false,
		},
		{
			name: "PodWithNilResources",
			podInfo: &nodedeclaredfeatures.PodInfo{
				Spec: &v1.PodSpec{},
			},
			expected: false,
		},
		{
			name: "PodWithEmptyLimits",
			podInfo: &nodedeclaredfeatures.PodInfo{
				Spec: &v1.PodSpec{
					Resources: &v1.ResourceRequirements{
						Limits: v1.ResourceList{},
					},
				},
			},
			expected: false,
		},
		{
			name: "PodWithPIDAndOtherLimits",
			podInfo: &nodedeclaredfeatures.PodInfo{
				Spec: &v1.PodSpec{
					Resources: &v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceCPU: resource.MustParse("2"),
							v1.ResourcePID: resource.MustParse("4096"),
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Feature.InferForScheduling(tt.podInfo); got != tt.expected {
				t.Fatalf("InferForScheduling() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPerPodPIDLimitFeature_InferForUpdate(t *testing.T) {
	oldPodInfo := &nodedeclaredfeatures.PodInfo{Spec: &v1.PodSpec{}}
	newPodInfo := &nodedeclaredfeatures.PodInfo{
		Spec: &v1.PodSpec{
			Resources: &v1.ResourceRequirements{
				Limits: v1.ResourceList{v1.ResourcePID: resource.MustParse("2048")},
			},
		},
	}
	if Feature.InferForUpdate(oldPodInfo, newPodInfo) {
		t.Fatal("InferForUpdate should always return false (PID limits are immutable)")
	}
}
