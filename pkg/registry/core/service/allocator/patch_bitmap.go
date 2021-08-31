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
	"math/big"
	"math/rand"
	"time"
)

// NewAllocationMapReserved creates an allocation bitmap using a modified random scan strategy
// that maintains a reserved offset for OpenShift.
func NewAllocationMapReserved(max int, rangeSpec string) *AllocationBitmap {
	// OpenShift Reserved Offsets:
	reserved := make(map[int]struct{})
	// - OpenShift DNS always uses the .10 address (0 counts so we reserve the 9 offset)
	reserved[9] = struct{}{}

	a := AllocationBitmap{
		strategy: randomScanReservedStrategy{
			rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
			reserved: reserved,
		},
		allocated: big.NewInt(0),
		count:     0,
		max:       max,
		rangeSpec: rangeSpec,
	}
	return &a
}

// randomScanReservedStrategy chooses a random address from the provided big.Int, and then
// scans forward looking for the next available address (it will wrap the range if necessary).
// randomScanReservedStrategy omit some offsets that are "special" so they can't be allocated
// randomly, only explicitly
type randomScanReservedStrategy struct {
	rand     *rand.Rand
	reserved map[int]struct{}
}

func (rss randomScanReservedStrategy) AllocateBit(allocated *big.Int, max, count int) (int, bool) {
	if count >= max {
		return 0, false
	}
	offset := rss.rand.Intn(max)
	for i := 0; i < max; i++ {
		at := (offset + i) % max
		// skip reserved values
		if _, ok := rss.reserved[at]; ok {
			continue
		}
		if allocated.Bit(at) == 0 {
			return at, true
		}
	}
	return 0, false
}

var _ bitAllocator = randomScanReservedStrategy{}
