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

package apiserver

import (
	"fmt"
	"net/http"

	"k8s.io/apiserver/pkg/server/healthz"
	availabilitymetrics "k8s.io/kube-aggregator/pkg/controllers/status/metrics"
)

// NewAggregatedAPIServiceReachableCheck returns a readyz check that reports
// failure while any remote (Spec.Service != nil) APIService is considered
// unavailable from this apiserver instance's point of view.
//
// The verdicts come from the remote AvailableConditionController, which
// probes each APIService's discovery endpoint from this instance over the
// same proxy transport (and therefore the same pooled connections) used by
// the aggregation proxy handler. This makes the check a truthful,
// per-instance signal of aggregated API reachability that an external load
// balancer can consume, either via the root /readyz or directly via the
// /readyz/aggregated-apiservice-reachable subpath:
//   - during a local dataplane outage (e.g. the node's pod network has not
//     converged after a reboot) the probes fail and the check reports
//     unready, keeping this instance out of the load balancer rotation;
//   - if pooled backend connections silently break after readiness, the
//     probes share the broken pool and the check flips back to unready
//     until the pool recovers.
//
// APIServices that have not been probed yet are treated as reachable, so a
// freshly started apiserver is not blocked on the first controller sync.
// Only APIServices that are unavailable because the discovery probe itself
// failed ("FailedDiscoveryCheck") make the check report unready; structural
// unavailability (missing Service or Endpoints, e.g. while the aggregated
// apiserver is still being deployed at install time) is deliberately ignored
// so that all instances do not drop out of the load balancer at once while a
// backend is absent cluster-wide.
func NewAggregatedAPIServiceReachableCheck(metrics *availabilitymetrics.Metrics) healthz.HealthChecker {
	return &aggregatedAPIServiceReachableCheck{metrics: metrics}
}

// failedDiscoveryCheckReason is the Available condition reason set by the
// remote AvailableConditionController when the discovery probe to an
// APIService's endpoints fails from this instance.
const failedDiscoveryCheckReason = "FailedDiscoveryCheck"

type aggregatedAPIServiceReachableCheck struct {
	metrics *availabilitymetrics.Metrics
}

func (c *aggregatedAPIServiceReachableCheck) Name() string {
	return "aggregated-apiservice-reachable"
}

func (c *aggregatedAPIServiceReachableCheck) Check(_ *http.Request) error {
	if unreachable := c.metrics.UnavailableAPIServices(failedDiscoveryCheckReason); len(unreachable) > 0 {
		return fmt.Errorf("aggregated APIServices unreachable from this instance: %v", unreachable)
	}
	return nil
}
