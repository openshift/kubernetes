/*
Copyright 2018 The Kubernetes Authors.

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

package metrics

import (
	"sort"
	"sync"

	"k8s.io/component-base/metrics"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	apiregistrationv1apihelper "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1/helper"
)

/*
 * By default, all the following metrics are defined as falling under
 * ALPHA stability level https://github.com/kubernetes/enhancements/blob/master/keps/sig-instrumentation/1209-metrics-stability/kubernetes-control-plane-metrics-stability.md#stability-classes)
 *
 * Promoting the stability level of the metric is a responsibility of the component owner, since it
 * involves explicitly acknowledging support for the metric across multiple releases, in accordance with
 * the metric stability policy.
 */
var (
	unavailableGaugeDesc = metrics.NewDesc(
		"aggregator_unavailable_apiservice",
		"Gauge of APIServices which are marked as unavailable broken down by APIService name.",
		[]string{"name"},
		nil,
		metrics.ALPHA,
		"",
	)
)

type Metrics struct {
	unavailableCounter *metrics.CounterVec

	*availabilityCollector

	// reasonsMtx guards unavailableReasons, the per-instance record of why each
	// currently-unavailable APIService was marked unavailable by this instance.
	reasonsMtx         sync.RWMutex
	unavailableReasons map[string]string
}

func New() *Metrics {
	return &Metrics{
		unavailableCounter: metrics.NewCounterVec(
			&metrics.CounterOpts{
				Name:           "aggregator_unavailable_apiservice_total",
				Help:           "Counter of APIServices which are marked as unavailable broken down by APIService name and reason.",
				StabilityLevel: metrics.ALPHA,
			},
			[]string{"name", "reason"},
		),
		availabilityCollector: newAvailabilityCollector(),
		unavailableReasons:    make(map[string]string),
	}
}

// Register registers apiservice availability metrics.
func (m *Metrics) Register(
	registrationFunc func(metrics.Registerable) error,
	customRegistrationFunc func(metrics.StableCollector) error,
) error {
	err := registrationFunc(m.unavailableCounter)
	if err != nil {
		return err
	}

	err = customRegistrationFunc(m.availabilityCollector)
	if err != nil {
		return err
	}

	return nil
}

// UnavailableCounter returns a counter to track apiservices marked as unavailable.
func (m *Metrics) UnavailableCounter(apiServiceName, reason string) metrics.CounterMetric {
	return m.unavailableCounter.WithLabelValues(apiServiceName, reason)
}

type availabilityCollector struct {
	metrics.BaseStableCollector

	mtx            sync.RWMutex
	availabilities map[string]bool
}

// SetUnavailableGauge set the metrics so that it reflect the current state base on availability of the given service
func (m *Metrics) SetUnavailableGauge(newAPIService *apiregistrationv1.APIService) {
	if apiregistrationv1apihelper.IsAPIServiceConditionTrue(newAPIService, apiregistrationv1.Available) {
		m.setUnavailableReason(newAPIService.Name, "")
		m.SetAPIServiceAvailable(newAPIService.Name)
		return
	}

	reason := "UnknownReason"
	if condition := apiregistrationv1apihelper.GetAPIServiceConditionByType(newAPIService, apiregistrationv1.Available); condition != nil {
		reason = condition.Reason
	}
	m.setUnavailableReason(newAPIService.Name, reason)
	m.SetAPIServiceUnavailable(newAPIService.Name)
}

// SetUnavailableCounter increases the metrics only if the given service is unavailable and its APIServiceCondition has changed
func (m *Metrics) SetUnavailableCounter(originalAPIService, newAPIService *apiregistrationv1.APIService) {
	wasAvailable := apiregistrationv1apihelper.IsAPIServiceConditionTrue(originalAPIService, apiregistrationv1.Available)
	isAvailable := apiregistrationv1apihelper.IsAPIServiceConditionTrue(newAPIService, apiregistrationv1.Available)
	statusChanged := isAvailable != wasAvailable

	if statusChanged && !isAvailable {
		reason := "UnknownReason"
		if newCondition := apiregistrationv1apihelper.GetAPIServiceConditionByType(newAPIService, apiregistrationv1.Available); newCondition != nil {
			reason = newCondition.Reason
		}
		m.UnavailableCounter(newAPIService.Name, reason).Inc()
	}
}

// Check if apiServiceStatusCollector implements necessary interface.
var _ metrics.StableCollector = &availabilityCollector{}

func newAvailabilityCollector() *availabilityCollector {
	return &availabilityCollector{
		availabilities: make(map[string]bool),
	}
}

// DescribeWithStability implements the metrics.StableCollector interface.
func (c *availabilityCollector) DescribeWithStability(ch chan<- *metrics.Desc) {
	ch <- unavailableGaugeDesc
}

// CollectWithStability implements the metrics.StableCollector interface.
func (c *availabilityCollector) CollectWithStability(ch chan<- metrics.Metric) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	for apiServiceName, isAvailable := range c.availabilities {
		gaugeValue := 1.0
		if isAvailable {
			gaugeValue = 0.0
		}
		ch <- metrics.NewLazyConstMetric(
			unavailableGaugeDesc,
			metrics.GaugeValue,
			gaugeValue,
			apiServiceName,
		)
	}
}

// SetAPIServiceAvailable sets the given apiservice availability gauge to available.
func (c *availabilityCollector) SetAPIServiceAvailable(apiServiceKey string) {
	c.setAPIServiceAvailability(apiServiceKey, true)
}

// SetAPIServiceUnavailable sets the given apiservice availability gauge to unavailable.
func (c *availabilityCollector) SetAPIServiceUnavailable(apiServiceKey string) {
	c.setAPIServiceAvailability(apiServiceKey, false)
}

func (c *availabilityCollector) setAPIServiceAvailability(apiServiceKey string, availability bool) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.availabilities[apiServiceKey] = availability
}

// ForgetAPIService removes the availability gauge of the given apiservice.
func (c *availabilityCollector) ForgetAPIService(apiServiceKey string) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	delete(c.availabilities, apiServiceKey)
}

// UnavailableAPIServices returns the names of the APIServices currently
// considered unavailable from this apiserver instance's point of view,
// together with the reason recorded for the unavailability. Only APIServices
// whose unavailability reason is contained in the given set are returned; an
// empty set returns all unavailable APIServices.
func (m *Metrics) UnavailableAPIServices(reasons ...string) []string {
	unavailable := m.availabilityCollector.unavailableAPIServices()
	if len(reasons) == 0 {
		return unavailable
	}

	m.reasonsMtx.RLock()
	defer m.reasonsMtx.RUnlock()

	var filtered []string
	for _, name := range unavailable {
		for _, reason := range reasons {
			if m.unavailableReasons[name] == reason {
				filtered = append(filtered, name)
				break
			}
		}
	}
	return filtered
}

// ForgetAPIService removes all records of the given apiservice. It shadows
// the embedded availabilityCollector method to also clear the recorded
// unavailability reason.
func (m *Metrics) ForgetAPIService(apiServiceKey string) {
	m.setUnavailableReason(apiServiceKey, "")
	m.availabilityCollector.ForgetAPIService(apiServiceKey)
}

func (m *Metrics) setUnavailableReason(apiServiceKey, reason string) {
	m.reasonsMtx.Lock()
	defer m.reasonsMtx.Unlock()

	if reason == "" {
		delete(m.unavailableReasons, apiServiceKey)
		return
	}
	m.unavailableReasons[apiServiceKey] = reason
}

// unavailableAPIServices returns the names of the APIServices currently
// considered unavailable from this apiserver instance's point of view.
func (c *availabilityCollector) unavailableAPIServices() []string {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	var unavailable []string
	for apiServiceName, isAvailable := range c.availabilities {
		if !isAvailable {
			unavailable = append(unavailable, apiServiceName)
		}
	}
	sort.Strings(unavailable)
	return unavailable
}
