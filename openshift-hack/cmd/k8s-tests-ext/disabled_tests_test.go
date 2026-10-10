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

package main

import (
	"testing"

	et "github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
)

func TestFilterOutDisabledSpecsVSphereTopology(t *testing.T) {
	tests := []struct {
		name     string
		testName string
		disabled bool
	}{
		{
			name:     "delayed binding rejects conflicting topology",
			testName: "[sig-storage] In-tree Volumes [Driver: vsphere] [Provider:vsphere] [Testpattern: Dynamic PV (delayed binding)] topology should fail to schedule a pod which has topologies that conflict with AllowedTopologies",
			disabled: true,
		},
		{
			name:     "delayed binding provisions with allowed topology",
			testName: "[sig-storage] In-tree Volumes [Driver: vsphere] [Provider:vsphere] [Testpattern: Dynamic PV (delayed binding)] topology should provision a volume and schedule a pod with AllowedTopologies",
			disabled: true,
		},
		{
			name:     "immediate binding rejects conflicting topology",
			testName: "[sig-storage] In-tree Volumes [Driver: vsphere] [Provider:vsphere] [Testpattern: Dynamic PV (immediate binding)] topology should fail to schedule a pod which has topologies that conflict with AllowedTopologies",
			disabled: true,
		},
		{
			name:     "immediate binding provisions with allowed topology",
			testName: "[sig-storage] In-tree Volumes [Driver: vsphere] [Provider:vsphere] [Testpattern: Dynamic PV (immediate binding)] topology should provision a volume and schedule a pod with AllowedTopologies",
			disabled: true,
		},
		{
			name:     "different driver topology test remains enabled",
			testName: "[sig-storage] In-tree Volumes [Driver: aws] [Provider:aws] [Testpattern: Dynamic PV (delayed binding)] topology should provision a volume and schedule a pod with AllowedTopologies",
		},
		{
			name:     "different test pattern remains enabled",
			testName: "[sig-storage] In-tree Volumes [Driver: vsphere] [Provider:vsphere] [Testpattern: Dynamic PV (default fs)] topology should provision a volume and schedule a pod with AllowedTopologies",
		},
		{
			name:     "different topology test remains enabled",
			testName: "[sig-storage] In-tree Volumes [Driver: vsphere] [Provider:vsphere] [Testpattern: Dynamic PV (delayed binding)] topology should provision a volume without AllowedTopologies",
		},
		{
			name:     "outdated pre-provider name remains enabled",
			testName: "[sig-storage] In-tree Volumes [Driver: vsphere] [Testpattern: Dynamic PV (delayed binding)] topology should provision a volume and schedule a pod with AllowedTopologies",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			specs := et.ExtensionTestSpecs{{Name: test.testName}}
			gotDisabled := len(filterOutDisabledSpecs(specs)) == 0
			if gotDisabled != test.disabled {
				t.Fatalf("filterOutDisabledSpecs() disabled = %t, want %t", gotDisabled, test.disabled)
			}
		})
	}
}
