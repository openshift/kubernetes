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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
)

func diagnosticPod() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "mock", Namespace: "driver"},
		Spec: v1.PodSpec{
			Containers:          []v1.Container{{Name: "plugin"}},
			InitContainers:      []v1.Container{{Name: "init"}},
			EphemeralContainers: []v1.EphemeralContainer{{EphemeralContainerCommon: v1.EphemeralContainerCommon{Name: "debug"}}},
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			ContainerStatuses: []v1.ContainerStatus{{
				Name: "plugin", RestartCount: 2,
				LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: 137, Reason: "OOMKilled"}},
			}},
			InitContainerStatuses: []v1.ContainerStatus{{
				Name: "init", Ready: true, RestartCount: 1,
				LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}},
			}},
			EphemeralContainerStatuses: []v1.ContainerStatus{{
				Name: "debug", RestartCount: 1,
				LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: 0, Reason: "Completed"}},
			}},
		},
	}
}

func TestCSIMockDiagnosticsCapture(testingContext *testing.T) {
	testingContext.Parallel()
	directory := testingContext.TempDir()
	client := fake.NewSimpleClientset(diagnosticPod())
	requests := map[string]int{}
	var requestOrder []string
	client.PrependReactor("get", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			testingContext.Fatalf("unexpected action: %#v", action)
		}
		if _, err := os.Stat(filepath.Join(directory, "driver", "mock", "cleanup-status.json")); err != nil {
			testingContext.Errorf("status must be saved before fetching logs: %v", err)
		}
		options := action.(clienttesting.GenericAction).GetValue().(*v1.PodLogOptions)
		if options.Follow || options.LimitBytes == nil || *options.LimitBytes != csiMockLogLimit || options.TailLines == nil || *options.TailLines != 1000 {
			testingContext.Errorf("unbounded log options: %#v", options)
		}
		if action.GetNamespace() != "driver" {
			testingContext.Errorf("unexpected namespace: %s", action.GetNamespace())
		}
		requests[fmt.Sprintf("%s/%t", options.Container, options.Previous)]++
		requestOrder = append(requestOrder, fmt.Sprintf("%s/%t", options.Container, options.Previous))
		return false, nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dumpCSIMockDiagnostics(ctx, client, "driver", directory, time.Second, time.Second)
	wantOrder := []string{"plugin/true", "init/true", "debug/true", "plugin/false", "init/false", "debug/false"}
	if !reflect.DeepEqual(requestOrder, wantOrder) {
		testingContext.Errorf("log request order = %v, want %v", requestOrder, wantOrder)
	}
	for _, container := range []string{"plugin", "init", "debug"} {
		for _, previous := range []bool{true, false} {
			if requests[fmt.Sprintf("%s/%t", container, previous)] != 1 {
				testingContext.Errorf("missing request for %s (previous=%t): %v", container, previous, requests)
			}
			data, err := os.ReadFile(filepath.Join(directory, "driver", "mock", fmt.Sprintf("cleanup-%s-previous-%t.log", container, previous)))
			if err != nil || string(data) != "fake logs" {
				testingContext.Errorf("missing container log: %q, %v", data, err)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(directory, "driver", "mock", "cleanup-status.json"))
	if err != nil {
		testingContext.Fatal(err)
	}
	var status v1.PodStatus
	if err := json.Unmarshal(data, &status); err != nil {
		testingContext.Fatal(err)
	}
	if !reflect.DeepEqual(status, diagnosticPod().Status) {
		testingContext.Errorf("incomplete status: %s", data)
	}
	for _, action := range client.Actions() {
		if action.GetResource().Resource != "pods" || action.GetNamespace() != "driver" {
			testingContext.Errorf("snapshot accessed resources outside the driver pods: %#v", action)
		}
		if (action.GetVerb() != "list" || action.GetSubresource() != "") && (action.GetVerb() != "get" || action.GetSubresource() != "log") {
			testingContext.Errorf("snapshot must not mutate resources or duplicate namespace event dumps: %#v", action)
		}
	}
}

func TestCSIMockDiagnosticsListError(testingContext *testing.T) {
	testingContext.Parallel()
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("list unavailable")
	})
	directory := testingContext.TempDir()
	dumpCSIMockDiagnostics(context.Background(), client, "driver", directory, time.Second, time.Second)
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 || len(client.Actions()) != 1 {
		testingContext.Fatalf("list error must stop only snapshot collection: entries=%v, actions=%v, error=%v", entries, client.Actions(), err)
	}
}

func diagnosticClient(testingContext *testing.T, handler http.HandlerFunc) clientset.Interface {
	testingContext.Helper()
	server := httptest.NewServer(handler)
	testingContext.Cleanup(server.Close)
	client, err := clientset.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		testingContext.Fatal(err)
	}
	return client
}

func TestCSIMockDiagnosticsRequestErrorsAndDeadlines(testingContext *testing.T) {
	testingContext.Parallel()
	for _, testCase := range []struct {
		name             string
		operationTimeout time.Duration
		budget           time.Duration
		listStalls       bool
		previousStalls   bool
		bodyStalls       bool
		previousMissing  bool
		partialRead      bool
		wantCurrent      bool
	}{
		{name: "missing previous", operationTimeout: time.Second, budget: time.Second, previousMissing: true, wantCurrent: true},
		{name: "operation timeout continues", operationTimeout: 250 * time.Millisecond, budget: 3 * time.Second, previousStalls: true, wantCurrent: true},
		{name: "body read timeout continues", operationTimeout: 250 * time.Millisecond, budget: 3 * time.Second, bodyStalls: true, wantCurrent: true},
		{name: "overall budget stops requests", operationTimeout: 3 * time.Second, budget: 250 * time.Millisecond, previousStalls: true},
		{name: "list timeout", operationTimeout: 250 * time.Millisecond, budget: 3 * time.Second, listStalls: true},
		{name: "partial read preserved", operationTimeout: time.Second, budget: time.Second, partialRead: true, wantCurrent: true},
	} {
		testingContext.Run(testCase.name, func(testingContext *testing.T) {
			testingContext.Parallel()
			pod := diagnosticPod()
			pod.Spec.InitContainers = nil
			pod.Spec.EphemeralContainers = nil
			requests := make(chan string, 4)
			canceled := make(chan string, 1)
			client := diagnosticClient(testingContext, func(response http.ResponseWriter, request *http.Request) {
				isLog := strings.HasSuffix(request.URL.Path, "/log")
				previous := request.URL.Query().Get("previous") == "true"
				operation := "list"
				if isLog {
					operation = fmt.Sprintf("logs/previous=%t", previous)
					if request.URL.Path != "/api/v1/namespaces/driver/pods/mock/log" || request.URL.Query().Get("container") != "plugin" {
						testingContext.Errorf("unexpected log request: %s", request.URL)
					}
				} else if request.URL.Path != "/api/v1/namespaces/driver/pods" {
					testingContext.Errorf("unexpected list request: %s", request.URL)
				}
				requests <- operation
				if (!isLog && testCase.listStalls) || (isLog && previous && testCase.previousStalls) {
					<-request.Context().Done()
					canceled <- operation
					return
				}
				if !isLog {
					response.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(response).Encode(&v1.PodList{Items: []v1.Pod{*pod}}); err != nil {
						testingContext.Errorf("encode pod list: %v", err)
					}
					return
				}
				if previous && testCase.previousMissing {
					http.Error(response, "no previous instance", http.StatusBadRequest)
					return
				}
				if previous && testCase.bodyStalls {
					response.WriteHeader(http.StatusOK)
					response.(http.Flusher).Flush()
					<-request.Context().Done()
					canceled <- operation
					return
				}
				if testCase.partialRead {
					response.Header().Set("Content-Length", "100")
				}
				fmt.Fprint(response, "partial log")
			})
			directory := testingContext.TempDir()
			start := time.Now()
			dumpCSIMockDiagnostics(context.Background(), client, "driver", directory, testCase.operationTimeout, testCase.budget)
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				testingContext.Errorf("snapshot did not respect budget: %s", elapsed)
			}
			var requestOrder []string
			for len(requests) > 0 {
				requestOrder = append(requestOrder, <-requests)
			}
			wantOrder := []string{"list"}
			if !testCase.listStalls {
				wantOrder = append(wantOrder, "logs/previous=true")
				if testCase.wantCurrent {
					wantOrder = append(wantOrder, "logs/previous=false")
				}
			}
			if !reflect.DeepEqual(requestOrder, wantOrder) {
				testingContext.Errorf("request sequence = %v, want %v", requestOrder, wantOrder)
			}
			if testCase.listStalls || testCase.previousStalls || testCase.bodyStalls {
				wantCanceled := "logs/previous=true"
				if testCase.listStalls {
					wantCanceled = "list"
				}
				select {
				case operation := <-canceled:
					if operation != wantCanceled {
						testingContext.Errorf("canceled operation = %s, want %s", operation, wantCanceled)
					}
				case <-time.After(time.Second):
					testingContext.Error("deadline did not cancel the stalled HTTP request")
				}
			}
			data, err := os.ReadFile(filepath.Join(directory, "driver", "mock", "cleanup-plugin-previous-false.log"))
			if testCase.wantCurrent {
				if err != nil || string(data) != "partial log" {
					testingContext.Errorf("current logs must survive previous-log/read errors: %q, %v", data, err)
				}
			} else if !os.IsNotExist(err) {
				testingContext.Errorf("unexpected current logs after deadline: %q, %v", data, err)
			}
		})
	}
}

func TestCSIMockLogReadLimit(testingContext *testing.T) {
	testingContext.Parallel()
	client := diagnosticClient(testingContext, func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("previous") != "true" || request.URL.Query().Get("limitBytes") != fmt.Sprint(csiMockLogLimit) {
			testingContext.Errorf("missing previous/limit query parameters: %s", request.URL.RawQuery)
		}
		fmt.Fprint(response, strings.Repeat("x", int(2*csiMockLogLimit)))
	})
	logs, err := readCSIMockLogs(context.Background(), client, "driver", "mock", "plugin", true, time.Second)
	if err != nil || len(logs) != int(csiMockLogLimit) {
		testingContext.Fatalf("client-side log limit: bytes=%d, error=%v", len(logs), err)
	}
}

func TestCSIMockDriverCleanupOrder(testingContext *testing.T) {
	testingContext.Parallel()
	for _, failed := range []bool{false, true} {
		testingContext.Run(fmt.Sprintf("failed=%t", failed), func(testingContext *testing.T) {
			testingContext.Parallel()
			var order []string
			client := fake.NewSimpleClientset()
			client.PrependReactor("list", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("diagnostic API unavailable")
			})
			cleanupError := errors.New("original cleanup failure")
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			directory := testingContext.TempDir()
			func() {
				defer func() {
					if recovered := recover(); recovered != cleanupError {
						testingContext.Errorf("original cleanup failure replaced: %v", recovered)
					}
				}()
				runCSIMockDriverCleanup(ctx, failed, func(snapshotCtx context.Context) {
					dumpCSIMockDiagnostics(snapshotCtx, client, "driver", directory, time.Second, time.Second)
					order = append(order, "snapshot")
				}, func() {
					order = append(order, "embedded")
				}, func(cleanupCtx context.Context) {
					if cleanupCtx != ctx || cleanupCtx.Err() != context.Canceled {
						testingContext.Error("snapshot changed the original cleanup context")
					}
					order = append(order, "delete")
					panic(cleanupError)
				})
			}()
			want := []string{"embedded", "delete"}
			if failed {
				want = append([]string{"snapshot"}, want...)
			} else if len(client.Actions()) != 0 {
				testingContext.Error("successful cleanup must not collect diagnostics")
			}
			if !reflect.DeepEqual(order, want) {
				testingContext.Errorf("cleanup order = %v, want %v", order, want)
			}
		})
	}
}

func TestCSIMockArtifactErrorDoesNotStopCleanup(testingContext *testing.T) {
	var captured bytes.Buffer
	ginkgo.GinkgoWriter.TeeTo(&captured)
	defer ginkgo.GinkgoWriter.ClearTeeWriters()
	directory := filepath.Join(testingContext.TempDir(), "not-a-directory")
	if err := os.WriteFile(directory, []byte("file"), 0600); err != nil {
		testingContext.Fatal(err)
	}
	client := fake.NewSimpleClientset(diagnosticPod())
	var order []string
	runCSIMockDriverCleanup(context.Background(), true, func(ctx context.Context) {
		dumpCSIMockDiagnostics(ctx, client, "driver", directory, time.Second, time.Second)
		order = append(order, "snapshot")
	}, func() {
		order = append(order, "embedded")
	}, func(ctx context.Context) {
		order = append(order, "delete")
	})
	if !reflect.DeepEqual(order, []string{"snapshot", "embedded", "delete"}) || len(client.Actions()) != 7 {
		testingContext.Errorf("artifact errors must not stop capture or cleanup: order=%v, actions=%v", order, client.Actions())
	}
	output := captured.String()
	for _, name := range []string{"cleanup-status.json", "cleanup-plugin-previous-true.log", "cleanup-init-previous-true.log", "cleanup-debug-previous-true.log", "cleanup-plugin-previous-false.log", "cleanup-init-previous-false.log", "cleanup-debug-previous-false.log"} {
		if !strings.Contains(output, "CSI mock cleanup diagnostic driver/mock/"+name+":") {
			testingContext.Errorf("missing fallback diagnostic: %s", name)
		}
	}
	if !strings.Contains(output, `"lastState"`) || !strings.Contains(output, "OOMKilled") || !strings.Contains(output, "fake logs") {
		testingContext.Errorf("fallback output lost diagnostic content: %s", output)
	}
}
