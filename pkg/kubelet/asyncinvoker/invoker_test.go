package asyncinvoker

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	statsapi "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
	"k8s.io/kubernetes/pkg/kubelet/server/stats"
)

func TestAsyncInvoker(t *testing.T) {
	// we have a slow provider
	slow := &slowSummaryProvider{t: t}

	// we wrap the slow provider with an async invoker, and pass
	// the instance with the async invoker to the client.
	// NOTE: this only works because GetCPUAndMemoryStats does not
	// accept any request scoped data
	asyncInvoker := NewAsyncInvoker[result](func() result {
		summary, err := slow.GetCPUAndMemoryStats(context.TODO())
		return result{summary: summary, err: err}
	})
	async := &asyncSummaryProvider{ResourceAnalyzer: slow, async: asyncInvoker}

	// run the async invoker
	stopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exit := asyncInvoker.Run(stopCtx)

	t.Run("serial callers", func(t *testing.T) {
		slow.invoked.Swap(0)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// the slow provider should return the following result once invoked
		// and the call should not block at all
		ch := make(chan struct{})
		close(ch)
		slow.r = slowProviderReturn{want: &statsapi.Summary{}, blocked: ch}

		for i := 1; i <= 10; i++ {
			got, err := async.GetCPUAndMemoryStats(ctx)
			if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
			if want := slow.r.want; want != got {
				t.Errorf("expected the summary returned to be identical, want: %p, but got: %p", want, got)
			}
		}

		if want, got := 10, int(slow.invoked.Load()); want != got {
			t.Errorf("expected the invoke count to be %d, but got: %d", want, got)
		}
	})

	t.Run("call in progress", func(t *testing.T) {
		// reset the invoke count
		slow.invoked.Swap(0)

		// the slow provider should return the following result once invoked
		// and the call is taking longer
		ch := make(chan struct{})
		slow.r = slowProviderReturn{want: &statsapi.Summary{}, blocked: ch}

		firstDone := make(chan struct{})
		go func() {
			defer close(firstDone)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			_, err := async.GetCPUAndMemoryStats(ctx)
			if want := context.DeadlineExceeded; !errors.Is(err, want) {
				t.Errorf("expected error: %v, but got: %v", want, err)
			}
		}()

		// wait for the first caller to time out
		<-firstDone
		t.Logf("first caller has timed out")
		// the slow call should still be in progress
		if want, got := 1, int(slow.progress.Load()); want != got {
			t.Fatalf("expected the call to be in progress: %d, but got: %d", want, got)
		}

		// fire off a second call
		secondDone := make(chan struct{})
		go func() {
			defer close(secondDone)
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()

			t.Logf("second caller making a call")
			got, err := async.GetCPUAndMemoryStats(ctx)
			if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
			if want := slow.r.want; want != got {
				t.Errorf("expected the summary returned to be identical, want: %p, but got: %p", want, got)
			}
		}()

		// unblock the second call, after some wait
		<-time.After(100 * time.Millisecond)
		t.Logf("unblocking the slow provider")
		close(ch)

		<-secondDone
		// we expect the call in progress to have finished
		if want, got := 0, int(slow.progress.Load()); want != got {
			t.Errorf("did not expect the call to be in progress: %d, but got: %d", want, got)
		}
		if want, got := 1, int(slow.invoked.Load()); want != got {
			t.Errorf("expected the call to be invoked: %d, but got: %d", want, got)
		}

		// a new call should return immediately
		ch = make(chan struct{})
		close(ch)
		slow.r = slowProviderReturn{want: &statsapi.Summary{}, blocked: ch}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		got, err := async.GetCPUAndMemoryStats(ctx)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}
		if want := slow.r.want; want != got {
			t.Errorf("expected the summary returned to be identical, want: %p, but got: %p", want, got)
		}

		if want, got := 2, int(slow.invoked.Load()); want != got {
			t.Errorf("expected the call to be invoked: %d, but got: %d", want, got)
		}
	})

	t.Run("async runner exits gracefully", func(t *testing.T) {
		cancel()

		select {
		case <-exit.Done():
		case <-time.After(wait.ForeverTestTimeout):
			t.Errorf("expected the async invoker to exit gracefully")
		}
	})
}

type result struct {
	summary *statsapi.Summary
	err     error
}

type asyncSummaryProvider struct {
	stats.ResourceAnalyzer
	async AsyncInvoker[result]
}

func (p *asyncSummaryProvider) GetCPUAndMemoryStats(ctx context.Context) (*statsapi.Summary, error) {
	wait := p.async.Invoke()
	select {
	case ret, ok := <-wait:
		if ok {
			if ret.Panicked != nil {
				panic(ret.Panicked)
			}
			return ret.Result.summary, ret.Result.err
		}
		return nil, fmt.Errorf("we should never be here")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type slowProviderReturn struct {
	// the test can be notified when the call starts and ends
	blocked <-chan struct{}
	want    *statsapi.Summary
}

type slowSummaryProvider struct {
	t                 *testing.T
	invoked, progress atomic.Int32
	r                 slowProviderReturn
}

func (slow *slowSummaryProvider) GetCPUAndMemoryStats(_ context.Context) (*statsapi.Summary, error) {
	slow.invoked.Add(1)
	// we never expect this call to be made concurrent
	slow.progress.Add(1)
	defer func() {
		slow.progress.Add(-1)
	}()
	// it blocks indefinitely, until the test writes to this channel
	now := time.Now()
	<-slow.r.blocked
	slow.t.Logf("slept for: %s", time.Since(now))
	return slow.r.want, nil
}

func (slow *slowSummaryProvider) Get(ctx context.Context, updateStats bool) (*statsapi.Summary, error) {
	return &statsapi.Summary{}, nil
}

func (slow *slowSummaryProvider) Start() {}
func (slow *slowSummaryProvider) GetPodVolumeStats(uid types.UID) (stats.PodVolumeStats, bool) {
	return stats.PodVolumeStats{}, false
}
