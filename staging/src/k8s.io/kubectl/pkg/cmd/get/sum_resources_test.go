/*
Copyright 2024 The Kubernetes Authors.

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

package get

import (
	"bytes"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddPodResources(t *testing.T) {
	tests := []struct {
		name           string
		pod            corev1.Pod
		expectedCPU    string
		expectedMemory string
	}{
		{
			name: "single container with resources",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
						},
					},
				},
			},
			expectedCPU:    "100m",
			expectedMemory: "134Mi",
		},
		{
			name: "multiple containers",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "multi-container"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
						{
							Name: "sidecar",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						},
					},
				},
			},
			expectedCPU:    "300m",
			expectedMemory: "335Mi",
		},
		{
			name: "container without resource requests",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "no-resources"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
						},
					},
				},
			},
			expectedCPU:    "0",
			expectedMemory: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := &resourceSummary{}
			addPodResources(tt.pod, summary)

			cpuStr := formatCPU(summary.totalCPU)
			memStr := formatMemory(summary.totalMemory)

			if cpuStr != tt.expectedCPU {
				t.Errorf("expected CPU %s, got %s", tt.expectedCPU, cpuStr)
			}
			if memStr != tt.expectedMemory {
				t.Errorf("expected memory %s, got %s", tt.expectedMemory, memStr)
			}
		})
	}
}

func TestFormatCPU(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0", "0"},
		{"100m", "100m"},
		{"1", "1"},
		{"1500m", "1500m"},
		{"2", "2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q := resource.MustParse(tt.input)
			result := formatCPU(q)
			if result != tt.expected {
				t.Errorf("formatCPU(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatMemory(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0", "0"},
		{"128Mi", "134Mi"},
		{"1Gi", "1Gi"},
		{"512Mi", "536Mi"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q := resource.MustParse(tt.input)
			result := formatMemory(q)
			if result != tt.expected {
				t.Errorf("formatMemory(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPrintResourceSummary(t *testing.T) {
	summary := &resourceSummary{
		totalCPU:    resource.MustParse("500m"),
		totalMemory: resource.MustParse("512Mi"),
		podCount:    3,
	}

	var buf bytes.Buffer
	printResourceSummary(&buf, summary)

	output := buf.String()
	if len(output) == 0 {
		t.Error("expected non-empty output")
	}
}
