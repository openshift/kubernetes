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
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// resourceSummary holds the aggregated resource requests for a set of pods.
type resourceSummary struct {
	totalCPU    resource.Quantity
	totalMemory resource.Quantity
	podCount    int
}

// calculateResourceSummary fetches pods and computes total CPU and memory requests
func calculateResourceSummary(f cmdutil.Factory, namespace string, labelSelector string) (*resourceSummary, error) {
	clientset, err := f.KubernetesClientSet()
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	listOpts := metav1.ListOptions{}
	if labelSelector != "" {
		listOpts.LabelSelector = labelSelector
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	summary := &resourceSummary{}
	for i := 0; i < len(pods.Items); i++ {
		addPodResources(pods.Items[i], summary)
	}

	return summary, nil
}

// addPodResources adds the resource requests from a single pod to the summary.
func addPodResources(pod corev1.Pod, summary *resourceSummary) {
	summary.podCount++

	for _, container := range pod.Spec.Containers {
		cpu := container.Resources.Requests[corev1.ResourceCPU]
		summary.totalCPU.Add(cpu)

		mem := container.Resources.Requests[corev1.ResourceMemory]
		summary.totalMemory.Add(mem)
	}
}

// formatCPU formats a CPU quantity into a human-readable string
func formatCPU(q resource.Quantity) string {
	milliValue := q.MilliValue()
	if milliValue == 0 {
		return "0"
	}
	if milliValue%1000 == 0 {
		return fmt.Sprintf("%d", milliValue/1000)
	}
	return fmt.Sprintf("%dm", milliValue)
}

// formatMemory formats a memory quantity into a human-readable string.
func formatMemory(q resource.Quantity) string {
	bytes := q.Value()
	if bytes == 0 {
		return "0"
	}

	// Convert to the most appropriate unit
	if bytes >= 1000*1000*1000 {
		return fmt.Sprintf("%dGi", bytes/(1000*1000*1000))
	}
	if bytes >= 1000*1000 {
		return fmt.Sprintf("%dMi", bytes/(1000*1000))
	}
	if bytes >= 1000 {
		return fmt.Sprintf("%dKi", bytes/1000)
	}
	return fmt.Sprintf("%d", bytes)
}

// formatQuantityStr is a helper that returns a formatted resource quantity string.
func formatQuantityStr(q resource.Quantity, resourceType string) string {
	if resourceType == "cpu" {
		return formatCPU(q)
	}
	return formatMemory(q)
}

// printResourceSummary prints the resource summary footer.
func printResourceSummary(out io.Writer, summary *resourceSummary) {
	fmt.Fprintf(out, "\n%-12s %-8s %s\n", "RESOURCE", "REQUEST", "")
	fmt.Fprintf(out, "%-12s %-8s\n", "CPU", formatCPU(summary.totalCPU))
	fmt.Fprintf(out, "%-12s %-8s\n", "Memory", formatMemory(summary.totalMemory))
	fmt.Fprintf(out, "Total pods: %d\n", summary.podCount)
}
