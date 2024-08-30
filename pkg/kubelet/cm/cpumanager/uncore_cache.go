/*
Copyright 2024 The Kubernetes Authors.

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

package cpumanager

import (
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpumanager/topology"
)

func (n *numaFirst) sortAvailableCoresForUncoreCaches() []int {
	var result []int
	for _, cache := range n.acc.sortAvailableUncoreCaches() {
		cores := n.acc.details.CoresInUncoreCaches(cache).UnsortedList()
		n.acc.sort(cores, n.acc.details.CPUsInCores)
		result = append(result, cores...)
	}
	return result
}

func (n *numaFirst) sortAvailableUncoreCaches() []int {
	var result []int
	for _, socket := range n.sortAvailableSockets() {
		caches := n.acc.details.UncoreCachesInSocket(socket).UnsortedList()
		n.acc.sort(caches, n.acc.details.CPUsInUncoreCaches)
		result = append(result, caches...)
	}
	return result
}

func (s *socketsFirst) sortAvaliableCoresForUncoreCaches() []int {
	var result []int
	for _, cache := range s.acc.sortAvailableUncoreCaches() {
		cores := s.acc.details.CoresInUncoreCaches(cache).UnsortedList()
		s.acc.sort(cores, s.acc.details.CPUsInCores)
		result = append(result, cores...)
	}
	return result
}

func (s *socketsFirst) sortAvailableUncoreCaches() []int {
	var result []int
	for _, node := range s.sortAvailableNUMANodes() {
		caches := s.acc.details.UncoreCachesInNUMANode(node).UnsortedList()
		s.acc.sort(caches, s.acc.details.CPUsInUncoreCaches)
		result = append(result, caches...)
	}
	return result
}

func (a *cpuAccumulator) takeFullUncoreGroups() {
	for _, uncorecache := range a.freeUncoreCaches() {
		cpusInUncoreCache := a.topo.CPUDetails.CPUsInUncoreCaches(uncorecache)
		if !a.needsAtLeast(cpusInUncoreCache.Size()) {
			continue
		}
		klog.V(4).InfoS("takeFullUncoreCaches: claiming uncore-cache", "uncore-cache", uncorecache)
		a.take(cpusInUncoreCache)
	}
	return
}

func (a *cpuAccumulator) takeRemainingUncoreGroups() {
	// First take an uncore group with the exact amount of remaining cpus
	// to avoid excessive fragmentation
	for _, uncorecache := range a.sortAvailableUncoreCaches() {
		cpusInUncoreCache := a.topo.CPUDetails.CPUsInUncoreCaches(uncorecache)
		if !a.needsAtLeast(cpusInUncoreCache.Size()) {
			continue
		}
		klog.V(4).InfoS("takeFullUncoreCaches: claiming exact size uncore-cache", "uncore-cache", uncorecache)
		a.take(cpusInUncoreCache)
	}

	// Fill the rest using whatever is available
	for a.numCPUsNeeded > 0 {
		// It is sorted smaller to larger, reverse it
		caches := a.sortAvailableUncoreCaches()
		for i := len(caches) - 1; i >= 0 && !a.isSatisfied(); i-- {
			cpusInUncoreCache := a.topo.CPUDetails.CPUsInUncoreCaches(caches[i])
			klog.V(4).InfoS("takeFullUncoreCaches: claiming remainder uncore-cache", "uncore-cache", caches[i])
			a.takeN(cpusInUncoreCache, a.numCPUsNeeded)
		}
	}

	return
}

// Returns true if the supplied core is fully available in `topoDetails`.
func (a *cpuAccumulator) isUncoreCacheFree(uncoreCacheID int) bool {
	return a.details.CPUsInUncoreCaches(uncoreCacheID).Size() == a.topo.CPUsPerUncoreCache()
}

func (a *cpuAccumulator) sortAvailableUncoreCaches() []int {
	return a.numaOrSocketsFirst.sortAvailableUncoreCaches()
}

// Returns free uncore cache IDs as a slice sorted by sortAvailableUncoreCaches().
// Only support when CpuManagerUncoreCacheAlign is enabled.
func (a *cpuAccumulator) freeUncoreCaches() []int {
	free := []int{}
	for _, cache := range a.sortAvailableUncoreCaches() {
		if a.isUncoreCacheFree(cache) {
			free = append(free, cache)
		}
	}
	return free
}

func SetupTopologyByPolicyOptions(topology *topology.CPUTopology, opts StaticPolicyOptions) *topology.CPUTopology {
	if opts.AlignByUnCoreCache {
		return topology // nothing to do, happy as we are already, just consume the data
	}
	topology = topology.DeepCopy()
	topology.NumUnCoreCaches = 0 // abuse the flag to disable logic deep down in takeByTopology
	klog.InfoS("Static policy uncore cache", "count", topology.NumUnCoreCaches, "aligning", opts.AlignByUnCoreCache)
	return topology
}
