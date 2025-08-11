package kubelet

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"
	statsapi "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
	"k8s.io/kubernetes/pkg/kubelet/asyncinvoker"
	"k8s.io/kubernetes/pkg/kubelet/server/stats"
)

func NewAsyncInvokerForGetCPUAndMemoryStats(ctx context.Context, slow stats.ResourceAnalyzer) stats.ResourceAnalyzer {
	// we wrap the slow provider with an async invoker, and pass
	// the instance with the async invoker to the client.
	// NOTE: this only works because GetCPUAndMemoryStats does not
	// accept any request scoped data
	asyncInvoker := asyncinvoker.NewAsyncInvoker[result](func() result {
		now := time.Now()
		summary, err := slow.GetCPUAndMemoryStats(context.TODO())
		klog.InfoS("slow.GetCPUAndMemoryStats", "latency", time.Since(now))
		return result{summary: summary, err: err}
	})
	go asyncInvoker.Run(ctx)
	return &asyncSummaryProvider{ResourceAnalyzer: slow, async: asyncInvoker}
}

type result struct {
	summary *statsapi.Summary
	err     error
}

type asyncSummaryProvider struct {
	stats.ResourceAnalyzer
	async asyncinvoker.AsyncInvoker[result]
}

func (p *asyncSummaryProvider) GetCPUAndMemoryStats(ctx context.Context) (*statsapi.Summary, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
	}

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
		return nil, fmt.Errorf("did not return within the time allotted - %w", ctx.Err())
	}
}
