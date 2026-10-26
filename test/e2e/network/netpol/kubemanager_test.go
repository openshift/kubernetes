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

package netpol

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/test/e2e/framework"
)

func TestProbeConnectivityExecResults(t *testing.T) {
	for _, test := range []struct {
		name               string
		status             string
		firstStatus        string
		sharedBudget       bool
		expectConnectivity bool
		wantConnected      bool
		wantError          bool
		wantRetry          bool
		wantExhausted      bool
	}{
		{name: "allowed", status: `{"status":"Success"}`, expectConnectivity: true, wantConnected: true},
		{name: "denied", status: `{"status":"Failure","reason":"NonZeroExitCode","details":{"causes":[{"reason":"ExitCode","message":"1"}]}}`},
		{name: "denial retries when allowed", status: `{"status":"Failure","reason":"NonZeroExitCode","details":{"causes":[{"reason":"ExitCode","message":"1"}]}}`, expectConnectivity: true, wantRetry: true, wantExhausted: true},
		{name: "allowed retries when denied", status: `{"status":"Success"}`, wantConnected: true, wantRetry: true, wantExhausted: true},
		{name: "unexpected exit", status: `{"status":"Failure","reason":"NonZeroExitCode","details":{"causes":[{"reason":"ExitCode","message":"126"}]}}`, wantError: true},
		{name: "transport failure is not denial", status: "forbidden", wantError: true},
		{name: "handshake deadline is not denial", status: "hang", wantError: true},
		{name: "handshake deadline is not allowed", status: "hang", expectConnectivity: true, wantError: true},
		{name: "retry shares deadline", firstStatus: `{"status":"Failure","reason":"NonZeroExitCode","details":{"causes":[{"reason":"ExitCode","message":"1"}]}}`, status: "hang", expectConnectivity: true, wantError: true, wantRetry: true, sharedBudget: true},
		{name: "retry converges", firstStatus: `{"status":"Failure","reason":"NonZeroExitCode","details":{"causes":[{"reason":"ExitCode","message":"1"}]}}`, status: `{"status":"Success"}`, expectConnectivity: true, wantConnected: true, wantRetry: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var attempts atomic.Int32
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method == http.MethodGet || test.status == "forbidden" {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				attempt := attempts.Add(1)
				status := test.status
				if attempt == 1 && test.firstStatus != "" {
					status = test.firstStatus
				}
				if status == "hang" {
					<-release
					return
				}
				if _, err := httpstream.Handshake(req, w, []string{"v4.channel.k8s.io"}); err != nil {
					t.Error(err)
					return
				}
				streams := make(chan httpstream.Stream, 3)
				connection := spdy.NewResponseUpgrader().UpgradeResponse(w, req, func(stream httpstream.Stream, replySent <-chan struct{}) error {
					streams <- stream
					return nil
				})
				if connection == nil {
					t.Error("upgrade failed")
					return
				}
				defer connection.Close()
				var statusStream httpstream.Stream
				for count := 0; count < 3; count++ {
					select {
					case stream := <-streams:
						if stream.Headers().Get(v1.StreamType) == v1.StreamTypeError {
							statusStream = stream
						} else {
							defer stream.Close()
						}
					case <-connection.CloseChan():
						return
					}
				}
				if statusStream == nil {
					t.Error("missing status stream")
					return
				}
				if _, err := io.WriteString(statusStream, status); err != nil {
					t.Error(err)
				}
				statusStream.Close()
			}))
			defer server.Close()
			defer close(release)
			config := &rest.Config{Host: server.URL}
			client, err := clientset.NewForConfig(config)
			if err != nil {
				t.Fatal(err)
			}
			probeFramework := &framework.Framework{ClientSet: client}
			probeFramework.SetClientConfig(config)
			manager := newKubeManager(probeFramework, "cluster.local")
			pollInterval, pollTimeout := 1, 2
			if test.sharedBudget {
				pollInterval, pollTimeout = 2, 4
			}
			started := time.Now()
			type probeResult struct {
				connected bool
				err       error
			}
			results := make(chan probeResult, 1)
			go func() {
				connected, _, err := manager.probeConnectivity(&probeConnectivityArgs{
					nsFrom: "namespace", podFrom: "pod", containerFrom: "container",
					addrTo: "127.0.0.1", toPort: 80, protocol: v1.ProtocolTCP,
					expectConnectivity: test.expectConnectivity, timeoutSeconds: 1,
					pollIntervalSeconds: pollInterval, pollTimeoutSeconds: pollTimeout,
				})
				results <- probeResult{connected: connected, err: err}
			}()
			var result probeResult
			select {
			case result = <-results:
			case <-time.After(5 * time.Second):
				t.Fatal("probe did not terminate within polling budget")
			}
			connected, err := result.connected, result.err
			var timeoutError net.Error
			exhaustedDuringExec := test.wantExhausted && errors.As(err, &timeoutError) && timeoutError.Timeout() && !connected
			if !exhaustedDuringExec && ((err != nil) != test.wantError || connected != test.wantConnected) {
				t.Fatalf("connected=%t, err=%v; want connected=%t, error=%t", connected, err, test.wantConnected, test.wantError)
			}
			if time.Since(started) > 5*time.Second {
				t.Fatal("probe exceeded polling budget")
			}
			if test.wantRetry && attempts.Load() < 2 {
				t.Fatalf("expected retries, got %d attempts", attempts.Load())
			}
		})
	}
}

type failedProber struct{}

func (failedProber) probeConnectivity(*probeConnectivityArgs) (bool, string, error) {
	return false, "exec", fmt.Errorf("exec transport failed")
}

func TestProbeTransportFailureCannotPassDeniedPolicy(t *testing.T) {
	pods := []TestPod{{Namespace: "namespace", Name: "pod", ServiceIP: "127.0.0.1"}}
	testCase := &TestCase{Reachability: NewReachability([]PodString{pods[0].PodString()}, false)}
	originalFail := framework.Fail
	defer func() { framework.Fail = originalFail }()
	var failures []string
	framework.Fail = func(message string, callerSkip ...int) {
		failures = append(failures, message)
	}
	ProbePodToPodConnectivity(failedProber{}, pods, "cluster.local", testCase)
	if len(failures) == 0 {
		t.Fatal("exec transport failure did not fail the denied-policy assertion")
	}
}
