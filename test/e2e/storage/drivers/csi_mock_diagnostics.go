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

package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

const csiMockLogLimit int64 = 1024 * 1024

func runCSIMockDriverCleanup(ctx context.Context, failed bool, snapshot func(context.Context), embeddedCleanup func(), driverCleanup func(context.Context)) {
	if failed {
		snapshot(ctx)
	}
	embeddedCleanup()
	driverCleanup(ctx)
}

func dumpCSIMockDiagnostics(ctx context.Context, client clientset.Interface, namespace, reportDir string, operationTimeout, budget time.Duration) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	defer cancel()

	listCtx, listCancel := context.WithTimeout(ctx, operationTimeout)
	pods, err := client.CoreV1().Pods(namespace).List(listCtx, metav1.ListOptions{})
	listCancel()
	if err != nil {
		framework.Logf("CSI mock cleanup: listing pods in %s: %v", namespace, err)
		return
	}

	for _, pod := range pods.Items {
		status, err := json.MarshalIndent(pod.Status, "", "  ")
		if err != nil {
			framework.Logf("CSI mock cleanup: encoding status for %s/%s: %v", namespace, pod.Name, err)
			continue
		}
		writeCSIMockDiagnostic(reportDir, namespace, pod.Name, "cleanup-status.json", status)
	}

	for _, pod := range pods.Items {
		containers := make([]string, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers)+len(pod.Spec.EphemeralContainers))
		for _, container := range pod.Spec.Containers {
			containers = append(containers, container.Name)
		}
		for _, container := range pod.Spec.InitContainers {
			containers = append(containers, container.Name)
		}
		for _, container := range pod.Spec.EphemeralContainers {
			containers = append(containers, container.Name)
		}
		for _, previous := range []bool{true, false} {
			for _, container := range containers {
				if ctx.Err() != nil {
					framework.Logf("CSI mock cleanup: diagnostic budget exhausted for %s: %v", namespace, ctx.Err())
					return
				}
				logs, err := readCSIMockLogs(ctx, client, namespace, pod.Name, container, previous, operationTimeout)
				if err != nil {
					framework.Logf("CSI mock cleanup: logs for %s/%s/%s (previous=%t): %v", namespace, pod.Name, container, previous, err)
				}
				if len(logs) > 0 {
					writeCSIMockDiagnostic(reportDir, namespace, pod.Name, fmt.Sprintf("cleanup-%s-previous-%t.log", container, previous), logs)
				}
			}
		}
	}
}

func readCSIMockLogs(ctx context.Context, client clientset.Interface, namespace, pod, container string, previous bool, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tailLines, limitBytes := int64(1000), csiMockLogLimit
	stream, err := client.CoreV1().Pods(namespace).GetLogs(pod, &v1.PodLogOptions{
		Container:  container,
		Previous:   previous,
		TailLines:  &tailLines,
		LimitBytes: &limitBytes,
	}).Stream(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return io.ReadAll(io.LimitReader(stream, csiMockLogLimit))
}

func writeCSIMockDiagnostic(reportDir, namespace, pod, name string, data []byte) {
	if reportDir != "" {
		directory := filepath.Join(reportDir, namespace, pod)
		if err := os.MkdirAll(directory, 0755); err != nil {
			framework.Logf("CSI mock cleanup: creating %s: %v", directory, err)
		} else if err := os.WriteFile(filepath.Join(directory, name), data, 0644); err != nil {
			framework.Logf("CSI mock cleanup: writing %s: %v", name, err)
		} else {
			return
		}
	}
	framework.Logf("CSI mock cleanup diagnostic %s/%s/%s:\n%s", namespace, pod, name, data)
}
