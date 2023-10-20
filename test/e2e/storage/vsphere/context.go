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

package vsphere

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

// TestContext holds common information for vSphere tests
type TestContext struct {
	NodeMapper       *NodeMapper
	VSphereInstances map[string]*VSphere
}

func NewTestContext(f *framework.Framework) *TestContext {
	// Read vSphere conf and get VSphere instances
	vsphereInstances, err := GetVSphereInstances()
	if err != nil {
		framework.Failf("Failed to bootstrap vSphere with error: %v", err)
	}

	// Get all nodes
	nodeList, err := f.ClientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		framework.Failf("Failed to get nodes: %v", err)
	}
	testContext := &TestContext{NodeMapper: NewNodeMapper(), VSphereInstances: vsphereInstances}

	// Get Node to VSphere mapping
	err = testContext.NodeMapper.GenerateNodeMap(vsphereInstances, *nodeList)
	if err != nil {
		framework.Failf("Failed to bootstrap vSphere with error: %v", err)
	}

	// Generate Zone to Datastore mapping
	err = testContext.NodeMapper.GenerateZoneToDatastoreMap()
	if err != nil {
		framework.Failf("Failed to generate zone to datastore mapping with error: %v", err)
	}

	return testContext
}
