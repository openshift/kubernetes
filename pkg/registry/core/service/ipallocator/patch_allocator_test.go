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
package ipallocator

import (
	"net"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/registry/core/service/allocator"
)

func TestAllocate_BitmapReserved(t *testing.T) {
	testCases := []struct {
		name             string
		cidr             string
		family           api.IPFamily
		free             int
		released         string
		outOfRange       []string
		alreadyAllocated string
		reserved         []string
	}{
		{
			name:     "IPv4",
			cidr:     "192.168.1.0/24",
			family:   api.IPv4Protocol,
			free:     254,
			released: "192.168.1.5",
			outOfRange: []string{
				"192.168.0.1",   // not in 192.168.1.0/24
				"192.168.1.0",   // reserved (base address)
				"192.168.1.255", // reserved (broadcast address)
				"192.168.2.2",   // not in 192.168.1.0/24
			},
			alreadyAllocated: "192.168.1.1",
			reserved:         []string{"192.168.1.10"}, // can only be allocated explicitly not randomly
		},
		{
			name:     "IPv6",
			cidr:     "2001:db8:1::/48",
			family:   api.IPv6Protocol,
			free:     65535,
			released: "2001:db8:1::5",
			outOfRange: []string{
				"2001:db8::1",     // not in 2001:db8:1::/48
				"2001:db8:1::",    // reserved (base address)
				"2001:db8:1::1:0", // not in the low 16 bits of 2001:db8:1::/48
				"2001:db8:2::2",   // not in 2001:db8:1::/48
			},
			alreadyAllocated: "2001:db8:1::1",
			reserved:         []string{"2001:db8:1::a"}, // can only be allocated explicitly not randomly

		},
	}
	for _, tc := range testCases {
		_, cidr, err := net.ParseCIDR(tc.cidr)
		if err != nil {
			t.Fatal(err)
		}
		r, err := NewAllocatorCIDRRange(cidr, func(max int, rangeSpec string) (allocator.Interface, error) {
			return allocator.NewAllocationMapReserved(max, rangeSpec), nil
		})

		if err != nil {
			t.Fatal(err)
		}
		t.Logf("base: %v", r.base.Bytes())
		if f := r.Free(); f != tc.free {
			t.Errorf("Test %s unexpected free %d", tc.name, f)
		}

		rCIDR := r.CIDR()
		if rCIDR.String() != tc.cidr {
			t.Errorf("allocator returned a different cidr")
		}

		if r.IPFamily() != tc.family {
			t.Errorf("allocator returned wrong IP family")
		}

		if f := r.Used(); f != 0 {
			t.Errorf("Test %s unexpected used %d", tc.name, f)
		}
		found := sets.NewString()
		count := 0
		reserved := len(tc.reserved)
		for r.Free() > reserved {
			ip, err := r.AllocateNext()
			if err != nil {
				t.Fatalf("Test %s error @ %d: %v", tc.name, count, err)
			}
			count++
			if !cidr.Contains(ip) {
				t.Fatalf("Test %s allocated %s which is outside of %s", tc.name, ip, cidr)
			}
			if found.Has(ip.String()) {
				t.Fatalf("Test %s allocated %s twice @ %d", tc.name, ip, count)
			}
			found.Insert(ip.String())
		}
		// at this point all the IPs are allocated except the ones reserved
		if _, err := r.AllocateNext(); err != ErrFull {
			t.Fatal(err)
		}
		// check that the random allocated IPs didn't allocate the reserved IPs
		for _, ip := range tc.reserved {
			if found.Has(ip) {
				t.Fatalf("Test %s allocated reserved IP %s randomly", tc.name, ip)
			}
		}

		released := net.ParseIP(tc.released)
		if err := r.Release(released); err != nil {
			t.Fatal(err)
		}
		if f := r.Free(); f != (1 + reserved) {
			t.Errorf("Test %s unexpected free %d", tc.name, f)
		}
		if f := r.Used(); f != (tc.free - (1 + reserved)) {
			t.Errorf("Test %s unexpected free %d", tc.name, f)
		}
		ip, err := r.AllocateNext()
		if err != nil {
			t.Fatal(err)
		}
		if !released.Equal(ip) {
			t.Errorf("Test %s unexpected %s : %s", tc.name, ip, released)
		}

		if err := r.Release(released); err != nil {
			t.Fatal(err)
		}
		for _, outOfRange := range tc.outOfRange {
			err = r.Allocate(net.ParseIP(outOfRange))
			if _, ok := err.(*ErrNotInRange); !ok {
				t.Fatal(err)
			}
		}
		if err := r.Allocate(net.ParseIP(tc.alreadyAllocated)); err != ErrAllocated {
			t.Fatal(err)
		}
		// allocate the reserved IPs
		for _, ip := range tc.reserved {
			if err := r.Allocate(net.ParseIP(ip)); err != nil {
				t.Fatal(err)
			}
		}
		if f := r.Free(); f != 1 {
			t.Errorf("Test %s unexpected free %d", tc.name, f)
		}
		if f := r.Used(); f != (tc.free - 1) {
			t.Errorf("Test %s unexpected free %d", tc.name, f)
		}
		if err := r.Allocate(released); err != nil {
			t.Fatal(err)
		}
		if f := r.Free(); f != 0 {
			t.Errorf("Test %s unexpected free %d", tc.name, f)
		}
		if f := r.Used(); f != tc.free {
			t.Errorf("Test %s unexpected free %d", tc.name, f)
		}
	}
}
