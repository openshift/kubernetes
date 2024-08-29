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

package topology

import (
	"k8s.io/utils/cpuset"

	cadvisorapi "github.com/google/cadvisor/info/v1"
)

const (
	cacheLevel3    = 3
	invalidCacheID = -1
)

// get specific level cache
func getUncoreCacheID(caches []cadvisorapi.Cache, level int) int {
	for _, cn := range caches {
		if cn.Level == level {
			return cn.Id
		}
	}
	return invalidCacheID
}

// CPUsPerUncoreCache returns the average number of logical CPUs are associated with
// each uncore cache id. Even CPUs share the same llc id may not the same.
func (topo *CPUTopology) CPUsPerUncoreCache() int {
	if topo.NumUnCoreCaches == 0 {
		return 0
	}
	return topo.NumCPUs / topo.NumUnCoreCaches
}

// UncoreCachesInSocket returns all of the logical uncore cache IDs associated with the
// given Socket IDs in this CPUDetails.
func (d CPUDetails) UncoreCachesInSocket(ids ...int) cpuset.CPUSet {
	var coreIDs []int
	for _, id := range ids {
		for _, info := range d {
			if info.SocketID == id {
				coreIDs = append(coreIDs, info.UnCoreCacheID)
			}
		}
	}
	return cpuset.New(coreIDs...)
}

func (d CPUDetails) UncoreCachesInNUMANode(ids ...int) cpuset.CPUSet {
	var coreIDs []int
	for _, id := range ids {
		for _, info := range d {
			if info.NUMANodeID == id {
				coreIDs = append(coreIDs, info.UnCoreCacheID)
			}
		}
	}
	return cpuset.New(coreIDs...)
}

// UncoreCaches returns all of the uncore cache IDs associated with the CPUs in this
// CPUDetails.
func (d CPUDetails) UncoreCaches() cpuset.CPUSet {
	var coreIDs []int
	for _, info := range d {
		if info.UnCoreCacheID != invalidCacheID {
			coreIDs = append(coreIDs, info.UnCoreCacheID)
		}
	}
	return cpuset.New(coreIDs...)
}

// CPUsInUncoreCaches returns all of the logical cpu IDs associated with the given
// uncore cache ID in this CPUDetails.
func (d CPUDetails) CPUsInUncoreCaches(ids ...int) cpuset.CPUSet {
	var coreIDs []int
	for _, id := range ids {
		for cpuid, info := range d {
			if info.UnCoreCacheID == id {
				coreIDs = append(coreIDs, cpuid)
			}
		}
	}
	return cpuset.New(coreIDs...)
}

// CoresInUncoreCaches returns all of the logical core IDs associated with the given
// uncore cache ID in this CPUDetails.
func (d CPUDetails) CoresInUncoreCaches(ids ...int) cpuset.CPUSet {
	var coreIDs []int
	for _, id := range ids {
		for _, info := range d {
			if info.UnCoreCacheID == id {
				coreIDs = append(coreIDs, info.CoreID)
			}
		}
	}
	return cpuset.New(coreIDs...)
}

func (topo *CPUTopology) FixUnCoreCacheIDs() *CPUTopology {
	// NumUnCoreCaches is abused as master flag for backward compatibility
	if topo.NumUnCoreCaches > 0 {
		return topo
	}
	topo = topo.DeepCopy()
	for id, info := range topo.CPUDetails {
		info.UnCoreCacheID = 0
		topo.CPUDetails[id] = info
	}
	return topo
}

func (topo *CPUTopology) DeepCopy() *CPUTopology {
	dets := make(CPUDetails, len(topo.CPUDetails))
	for id, info := range topo.CPUDetails {
		dets[id] = info
	}
	return &CPUTopology{
		NumCPUs:         topo.NumCPUs,
		NumCores:        topo.NumCores,
		NumSockets:      topo.NumSockets,
		NumNUMANodes:    topo.NumNUMANodes,
		NumUnCoreCaches: topo.NumUnCoreCaches,
		CPUDetails:      dets,
	}
}
