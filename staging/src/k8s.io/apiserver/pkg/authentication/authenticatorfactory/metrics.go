/*
Copyright 2020 The Kubernetes Authors.

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

package authenticatorfactory

import (
	"context"
	"sync"

	compbasemetrics "k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

type registerables []compbasemetrics.Registerable

var registerMetrics sync.Once

// register all metrics.
func register() {
	registerMetrics.Do(func() {
		for _, metric := range metrics {
			legacyregistry.MustRegister(metric)
		}
	})
}

func init() {
	register()
}

var (
	requestTotal = compbasemetrics.NewCounterVec(
		&compbasemetrics.CounterOpts{
			Name:           "apiserver_delegated_authorization_request_total",
			Help:           "Number of HTTP requests partitioned by status code and verb.",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{"code", "verb"},
	)

	requestLatency = compbasemetrics.NewHistogramVec(
		&compbasemetrics.HistogramOpts{
			Name:           "apiserver_delegated_authorization_request_duration_seconds",
			Help:           "Request latency in seconds. Broken down by status code and verb.",
			Buckets:        []float64{0.25, 0.5, 0.7, 1, 1.5, 3, 5, 7},
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{"code", "verb"},
	)

	metrics = registerables{
		requestTotal,
		requestLatency,
	}
)

// RequestTotal increments the total number of requests for the delegated authorization.
func RequestTotal(ctx context.Context, code, verb string) {
	requestTotal.WithLabelValues(code, verb).Add(1)
}

// RequestLatency measures request latency in seconds for the delegated authorization. Broken down by status code and verb.
func RequestLatency(ctx context.Context, latency float64, code, verb string) {
	requestLatency.WithLabelValues(code, verb).Observe(latency)
}
