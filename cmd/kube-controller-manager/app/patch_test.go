package app

import (
	"testing"
)

func TestMergeCh(t *testing.T) {
	testCases := []struct {
		name    string
		chan1   chan struct{}
		chan2   chan struct{}
		closeFn func(chan struct{}, chan struct{})
	}{
		{
			name:  "chan1 gets closed",
			chan1: make(chan struct{}),
			chan2: make(chan struct{}),
			closeFn: func(a, b chan struct{}) {
				close(a)
			},
		},
		{
			name:  "chan2 gets closed",
			chan1: make(chan struct{}),
			chan2: make(chan struct{}),
			closeFn: func(a, b chan struct{}) {
				close(b)
			},
		},
		{
			name:  "both channels get closed",
			chan1: make(chan struct{}),
			chan2: make(chan struct{}),
			closeFn: func(a, b chan struct{}) {
				close(a)
				close(b)
			},
		},
		{
			name:  "channel receives data and returned channel is closed",
			chan1: make(chan struct{}),
			chan2: make(chan struct{}),
			closeFn: func(a, b chan struct{}) {
				a <- struct{}{}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			go tc.closeFn(tc.chan1, tc.chan2)
			merged := mergeCh(tc.chan1, tc.chan2)
			if _, ok := <-merged; ok {
				t.Fatalf("expected closed channel, got data")
			}
		})
	}
}
