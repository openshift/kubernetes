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

package spdy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/httpstream"
)

func TestRoundTripCancelsResponseHandshake(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancellation"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				close(started)
				<-release
			}))
			defer server.Close()
			defer close(release)
			ctx, cancel := context.WithCancel(context.Background())
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), time.Second)
			}
			defer cancel()
			transport, err := NewRoundTripper(nil)
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				response, err := transport.RoundTrip(req)
				if response != nil {
					response.Body.Close()
				}
				result <- err
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach server")
			}
			if !deadline {
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, ctx.Err()) || ctx.Err() == nil {
					t.Fatalf("expected context error %v, got %v", ctx.Err(), err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("response handshake ignored context cancellation")
			}
		})
	}
}

type cancelAtHandoffContext struct {
	context.Context
	cancel            context.CancelFunc
	armed             atomic.Bool
	stopped           atomic.Bool
	checkedBeforeStop atomic.Bool
}

func (ctx *cancelAtHandoffContext) Value(key interface{}) interface{} {
	return nil
}

func (ctx *cancelAtHandoffContext) AfterFunc(callback func()) func() bool {
	stop := context.AfterFunc(ctx.Context, callback)
	return func() bool {
		stopped := stop()
		ctx.stopped.Store(stopped)
		return stopped
	}
}

func (ctx *cancelAtHandoffContext) Err() error {
	err := ctx.Context.Err()
	if ctx.armed.CompareAndSwap(true, false) {
		ctx.checkedBeforeStop.Store(!ctx.stopped.Load())
		ctx.cancel()
	}
	return err
}

func TestRoundTripCancellationPreservesUpgradedConnection(t *testing.T) {
	for _, name := range []string{"after handoff", "during handoff", "racing response"} {
		racing := name == "racing response"
		iterations := 1
		if racing {
			iterations = 50
		}
		t.Run(name, func(t *testing.T) {
			for iteration := 0; iteration < iterations; iteration++ {
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					handoff := &cancelAtHandoffContext{Context: ctx, cancel: cancel}
					if name == "during handoff" {
						ctx = handoff
					}
					cancelled := make(chan struct{})
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						if name == "during handoff" {
							handoff.armed.Store(true)
						}
						if racing {
							go func() {
								cancel()
								close(cancelled)
							}()
						}
						streams := make(chan httpstream.Stream, 1)
						connection := NewResponseUpgrader().UpgradeResponse(w, req, func(stream httpstream.Stream, replySent <-chan struct{}) error {
							streams <- stream
							return nil
						})
						if connection == nil {
							return
						}
						defer connection.Close()
						select {
						case stream := <-streams:
							defer stream.Close()
							io.Copy(stream, stream)
						case <-connection.CloseChan():
						}
					}))
					defer server.Close()
					transport, err := NewRoundTripper(nil)
					if err != nil {
						t.Fatal(err)
					}
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
					if err != nil {
						t.Fatal(err)
					}
					response, err := transport.RoundTrip(req)
					if handoff.checkedBeforeStop.Load() {
						t.Fatal("response context checked before cancellation callback was stopped")
					}
					if racing {
						select {
						case <-cancelled:
						case <-time.After(5 * time.Second):
							t.Fatal("server did not cancel the handshake")
						}
					} else {
						cancel()
					}
					if err != nil {
						if !racing || !errors.Is(err, context.Canceled) {
							t.Fatalf("unexpected handshake error: %v", err)
						}
						return
					}
					connection, err := transport.NewConnection(response)
					if err != nil {
						t.Fatal(err)
					}
					defer connection.Close()
					watchdog := time.AfterFunc(5*time.Second, func() { connection.Close() })
					defer watchdog.Stop()
					stream, err := connection.CreateStream(http.Header{})
					if err != nil {
						t.Fatal(err)
					}
					defer stream.Close()
					if _, err := io.WriteString(stream, "hello"); err != nil {
						t.Fatal(err)
					}
					buffer := make([]byte, len("hello"))
					if _, err := io.ReadFull(stream, buffer); err != nil || string(buffer) != "hello" {
						t.Fatalf("upgraded connection was not preserved: %q, %v", buffer, err)
					}
				}()
			}
		})
	}
}
