/*
Copyright 2021 The Kubernetes Authors.

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

package filters

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"errors"

	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/request"
	utilsclock "k8s.io/utils/clock"
)

var (
	watchVerbs = sets.NewString("watch")
)

func WithResponseWriteLatencyTracker(handler http.Handler) http.Handler {
	return withResponseWriteLatencyTracker(handler, clock.RealClock{})
}

func withResponseWriteLatencyTracker(handler http.Handler, clock clock.PassiveClock) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		requestInfo, ok := request.RequestInfoFrom(ctx)
		if !ok {
			handleError(w, req, http.StatusInternalServerError, fmt.Errorf("no RequestInfo found in context, handler chain must be wrong"))
			return
		}
		if watchVerbs.Has(requestInfo.Verb) {
			handler.ServeHTTP(w, req)
			return
		}

		req = req.WithContext(request.WithResponseWriteLatencyTracker(ctx))
		w = &writeTracker{
			clock:          clock,
			ctx:            req.Context(),
			ResponseWriter: w,
		}
		handler.ServeHTTP(w, req)
	})
}

var _ http.ResponseWriter = &writeTracker{}

type writeTracker struct {
	clock utilsclock.PassiveClock
	http.ResponseWriter
	ctx context.Context
}

func (wt *writeTracker) Unwrap() http.ResponseWriter {
	return wt.ResponseWriter
}

func (wt *writeTracker) Write(p []byte) (int, error) {
	startedAt := wt.clock.Now()
	defer func() {
		if tracker := request.ResponseWriteLatencyTrackerFrom(wt.ctx); tracker != nil {
			tracker.TrackResponseWrite(len(p), wt.clock.Since(startedAt))
		}
	}()

	return wt.ResponseWriter.Write(p)
}

func (wt *writeTracker) CloseNotify() <-chan bool {
	if notifier, ok := wt.ResponseWriter.(http.CloseNotifier); ok {
		return notifier.CloseNotify()
	}

	return nil
}

func (wt *writeTracker) Flush() {
	if flusher, ok := wt.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (wt *writeTracker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := wt.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}

	return nil, nil, errors.New("not a hijacker")
}
