/*
Copyright 2015 The Kubernetes Authors.

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

package sliceutils

import (
	"sort"

	v1 "k8s.io/api/core/v1"
	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

// StringInSlice returns true if s is in list
func StringInSlice(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}

	return false
}

// PodsByCreationTime makes an array of pods sortable by their creation
// timestamps in ascending order.
type PodsByCreationTime []*v1.Pod

func (s PodsByCreationTime) Len() int {
	return len(s)
}

func (s PodsByCreationTime) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s PodsByCreationTime) Less(i, j int) bool {
	return s[i].CreationTimestamp.Before(&s[j].CreationTimestamp)
}

// ByImageSize makes an array of images sortable by their size in descending
// order.
type ByImageSize []kubecontainer.Image

func (a ByImageSize) Less(i, j int) bool {
	if a[i].Size == a[j].Size {
		return a[i].ID > a[j].ID
	}
	return a[i].Size > a[j].Size
}
func (a ByImageSize) Len() int      { return len(a) }
func (a ByImageSize) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// Cmp compares p1 and p2 and returns:
//
//   -1 if p1 <  p2
//    0 if p1 == p2
//   +1 if p1 >  p2
//
type cmpFunc func(p1, p2 *v1.Pod) int

// MultiSorter implements the Sort interface, sorting changes within.
type MultiSorter struct {
	pods []*v1.Pod
	cmp  []cmpFunc
}

// Sort sorts the argument slice according to the less functions passed to OrderedBy.
func (ms *MultiSorter) Sort(pods []*v1.Pod) {
	ms.pods = pods
	sort.Sort(ms)
}

// OrderedBy returns a Sorter that sorts using the cmp functions, in order.
// Call its Sort method to sort the data.
func OrderedBy(cmp ...cmpFunc) *MultiSorter {
	return &MultiSorter{
		cmp: cmp,
	}
}

// Len is part of sort.Interface.
func (ms *MultiSorter) Len() int {
	return len(ms.pods)
}

// Swap is part of sort.Interface.
func (ms *MultiSorter) Swap(i, j int) {
	ms.pods[i], ms.pods[j] = ms.pods[j], ms.pods[i]
}

// Less is part of sort.Interface.
func (ms *MultiSorter) Less(i, j int) bool {
	p1, p2 := ms.pods[i], ms.pods[j]
	var k int
	for k = 0; k < len(ms.cmp)-1; k++ {
		cmpResult := ms.cmp[k](p1, p2)
		// p1 is less than p2
		if cmpResult < 0 {
			return true
		}
		// p1 is greater than p2
		if cmpResult > 0 {
			return false
		}
		// we don't know yet
	}
	// the last cmp func is the final decider
	return ms.cmp[k](p1, p2) < 0
}

// PriorityFirst compares pods by Priority, if priority is enabled.
func PriorityFirst(p1, p2 *v1.Pod) int {
	priority1 := corev1helpers.PodPriority(p1)
	priority2 := corev1helpers.PodPriority(p2)
	if priority1 == priority2 {
		return 0
	}
	if priority1 < priority2 {
		return 1
	}
	return -1
}
