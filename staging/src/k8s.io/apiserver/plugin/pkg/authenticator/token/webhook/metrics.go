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
package webhook

import (
	"context"
)

// AuthenticatorMetrics specifies a set of methods that are used to register various metrics
type AuthenticatorMetrics struct {
	// RequestTotal increments the total number of requests for webhooks
	RequestTotal func(ctx context.Context, code, verb string)

	// RequestLatency measures request latency in seconds for webhooks. Broken down by status code and verb.
	RequestLatency func(ctx context.Context, latency float64, code, verb string)
}

type noopMetrics struct{}

func (noopMetrics) RequestTotal(context.Context, string, string)            {}
func (noopMetrics) RequestLatency(context.Context, float64, string, string) {}
