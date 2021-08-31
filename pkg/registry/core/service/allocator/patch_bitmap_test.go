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

package allocator

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestAllocate_BitmapReserved(t *testing.T) {
	max := 20
	m := NewAllocationMapReserved(max, "test")

	if _, ok, _ := m.AllocateNext(); !ok {
		t.Fatalf("unexpected error")
	}
	if m.count != 1 {
		t.Errorf("expect to get %d, but got %d", 1, m.count)
	}
	if f := m.Free(); f != max-1 {
		t.Errorf("expect to get %d, but got %d", max-1, f)
	}
}

// TestAllocateMaxReserved depends on the number of reserved offsets
// Currently there is only one value reserved, if this value change
// in the future the test has to be modified accordingly
func TestAllocateMax_BitmapReserved(t *testing.T) {
	max := 20
	// modify if necessary
	reserved := 1
	m := NewAllocationMapReserved(max, "test")
	for i := 0; i < max-reserved; i++ {
		if _, ok, _ := m.AllocateNext(); !ok {
			t.Fatalf("unexpected error")
		}
	}

	if _, ok, _ := m.AllocateNext(); ok {
		t.Errorf("unexpected success")
	}
	if f := m.Free(); f != reserved {
		t.Errorf("expect to get %d, but got %d", 0, f)
	}
}

func TestAllocateError_BitmapReserved(t *testing.T) {
	m := NewAllocationMapReserved(20, "test")
	if ok, _ := m.Allocate(3); !ok {
		t.Errorf("error allocate offset %v", 3)
	}
	if ok, _ := m.Allocate(3); ok {
		t.Errorf("unexpected success")
	}
}

// 9 is a reserved value used for OpenshiftDNS
// it can only be allocated explicitly using Allocate()
func TestAllocateReservedOffset_BitmapReserved(t *testing.T) {
	m := NewAllocationMapReserved(20, "test")
	if ok, _ := m.Allocate(9); !ok {
		t.Errorf("error allocate offset %v", 9)
	}
	if ok, _ := m.Allocate(9); ok {
		t.Errorf("unexpected success")
	}
}

func TestPreAllocateReservedFull_BitmapReserved(t *testing.T) {
	max := 20
	reserved := 1
	m := NewAllocationMapReserved(max, "test")
	// Allocate the reserved value
	if ok, _ := m.Allocate(9); !ok {
		t.Errorf("error allocate offset %v", 9)
	}
	// Allocate all possible values except the reserved
	for i := 0; i < max-reserved; i++ {
		if _, ok, _ := m.AllocateNext(); !ok {
			t.Fatalf("unexpected error")
		}
	}

	if _, ok, _ := m.AllocateNext(); ok {
		t.Errorf("unexpected success")
	}
	if m.count != max {
		t.Errorf("expect to get %d, but got %d", max, m.count)
	}
	if f := m.Free(); f != 0 {
		t.Errorf("expect to get %d, but got %d", max-1, f)
	}
}

func TestPostAllocateReservedFull_BitmapReserved(t *testing.T) {
	max := 20
	reserved := 1
	m := NewAllocationMapReserved(max, "test")

	// Allocate all possible values except the reserved
	for i := 0; i < max-reserved; i++ {
		if _, ok, _ := m.AllocateNext(); !ok {
			t.Fatalf("unexpected error")
		}
	}

	if _, ok, _ := m.AllocateNext(); ok {
		t.Errorf("unexpected success")
	}
	// Allocate the reserved value
	if ok, _ := m.Allocate(9); !ok {
		t.Errorf("error allocate offset %v", 9)
	}
	if m.count != max {
		t.Errorf("expect to get %d, but got %d", max, m.count)
	}
	if f := m.Free(); f != 0 {
		t.Errorf("expect to get %d, but got %d", max-1, f)
	}
}

func TestRelease_BitmapReserved(t *testing.T) {
	offset := 3
	m := NewAllocationMapReserved(20, "test")
	if ok, _ := m.Allocate(offset); !ok {
		t.Errorf("error allocate offset %v", offset)
	}

	if !m.Has(offset) {
		t.Errorf("expect offset %v allocated", offset)
	}

	if err := m.Release(offset); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if m.Has(offset) {
		t.Errorf("expect offset %v to have been released", offset)
	}
}

func TestForEach_BitmapReserved(t *testing.T) {
	testCases := []sets.Int{
		sets.NewInt(),
		sets.NewInt(0),
		sets.NewInt(0, 2, 5, 9),
		sets.NewInt(0, 1, 2, 3, 4, 5, 6, 7, 8, 9),
	}

	for i, tc := range testCases {
		m := NewAllocationMapReserved(20, "test")
		for offset := range tc {
			if ok, _ := m.Allocate(offset); !ok {
				t.Errorf("[%d] error allocate offset %v", i, offset)
			}
			if !m.Has(offset) {
				t.Errorf("[%d] expect offset %v allocated", i, offset)
			}
		}
		calls := sets.NewInt()
		m.ForEach(func(i int) {
			calls.Insert(i)
		})
		if len(calls) != len(tc) {
			t.Errorf("[%d] expected %d calls, got %d", i, len(tc), len(calls))
		}
		if !calls.Equal(tc) {
			t.Errorf("[%d] expected calls to equal testcase: %v vs %v", i, calls.List(), tc.List())
		}
	}
}
