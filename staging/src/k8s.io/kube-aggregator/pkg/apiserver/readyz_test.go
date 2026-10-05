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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	availabilitymetrics "k8s.io/kube-aggregator/pkg/controllers/status/metrics"
)

func newAPIServiceWithAvailability(name string, available bool, reason string) *apiregistrationv1.APIService {
	status := apiregistrationv1.ConditionFalse
	if available {
		status = apiregistrationv1.ConditionTrue
	}
	return &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: apiregistrationv1.APIServiceStatus{
			Conditions: []apiregistrationv1.APIServiceCondition{{
				Type:   apiregistrationv1.Available,
				Status: status,
				Reason: reason,
			}},
		},
	}
}

func TestAggregatedAPIServiceReachableCheck(t *testing.T) {
	tests := []struct {
		name         string
		apiServices  []*apiregistrationv1.APIService
		forget       []string
		wantHealthy  bool
		wantInErrMsg []string
	}{
		{
			name:        "no apiservices probed yet",
			wantHealthy: true,
		},
		{
			name: "all available",
			apiServices: []*apiregistrationv1.APIService{
				newAPIServiceWithAvailability("v1.oauth.openshift.io", true, "Passed"),
				newAPIServiceWithAvailability("v1.route.openshift.io", true, "Passed"),
			},
			wantHealthy: true,
		},
		{
			name: "one unreachable via failed discovery check",
			apiServices: []*apiregistrationv1.APIService{
				newAPIServiceWithAvailability("v1.oauth.openshift.io", false, "FailedDiscoveryCheck"),
				newAPIServiceWithAvailability("v1.route.openshift.io", true, "Passed"),
			},
			wantHealthy:  false,
			wantInErrMsg: []string{"v1.oauth.openshift.io"},
		},
		{
			name: "structurally unavailable apiservices are ignored",
			apiServices: []*apiregistrationv1.APIService{
				newAPIServiceWithAvailability("v1.oauth.openshift.io", false, "MissingEndpoints"),
				newAPIServiceWithAvailability("v1.route.openshift.io", false, "ServiceNotFound"),
			},
			wantHealthy: true,
		},
		{
			name: "recovery clears the failure",
			apiServices: []*apiregistrationv1.APIService{
				newAPIServiceWithAvailability("v1.oauth.openshift.io", false, "FailedDiscoveryCheck"),
				newAPIServiceWithAvailability("v1.oauth.openshift.io", true, "Passed"),
			},
			wantHealthy: true,
		},
		{
			name: "forgotten apiservice clears the failure",
			apiServices: []*apiregistrationv1.APIService{
				newAPIServiceWithAvailability("v1.oauth.openshift.io", false, "FailedDiscoveryCheck"),
			},
			forget:      []string{"v1.oauth.openshift.io"},
			wantHealthy: true,
		},
		{
			name: "multiple unreachable are all reported sorted",
			apiServices: []*apiregistrationv1.APIService{
				newAPIServiceWithAvailability("v1.route.openshift.io", false, "FailedDiscoveryCheck"),
				newAPIServiceWithAvailability("v1.oauth.openshift.io", false, "FailedDiscoveryCheck"),
			},
			wantHealthy:  false,
			wantInErrMsg: []string{"v1.oauth.openshift.io", "v1.route.openshift.io"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := availabilitymetrics.New()
			for _, apiService := range tc.apiServices {
				m.SetUnavailableGauge(apiService)
			}
			for _, name := range tc.forget {
				m.ForgetAPIService(name)
			}

			check := NewAggregatedAPIServiceReachableCheck(m)
			if got, want := check.Name(), "aggregated-apiservice-reachable"; got != want {
				t.Errorf("Name() = %q, want %q", got, want)
			}

			err := check.Check(nil)
			if tc.wantHealthy {
				if err != nil {
					t.Fatalf("expected healthy, got error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected unhealthy, got nil error")
			}
			for _, fragment := range tc.wantInErrMsg {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("error %q does not contain %q", err.Error(), fragment)
				}
			}
		})
	}
}
