package asyncinvoker

import (
	"context"
	"fmt"
	"sync"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
)

func NewAsyncInvoker[T any](f func() T) *asyncInvoker[T] {
	invoker := &asyncInvoker[T]{
		f: f,
		// a buffer of length 1, so
		invoke: make(chan struct{}, 1),
	}
	return invoker
}

type AsyncInvoker[T any] interface {
	Invoke() <-chan Return[T]
}

type Return[T any] struct {
	Panicked any
	Result   T
	Latency  time.Duration
}

type asyncInvoker[T any] struct {
	f func() T

	lock    sync.Mutex
	invoke  chan struct{}
	waiters []chan Return[T]
}

// Invoke returns a channel
func (i *asyncInvoker[T]) Invoke() <-chan Return[T] {
	// the waiter is a buffered channel of length 1, so neither the
	// caller, nor the async invoker hangs on each other
	waiter := make(chan Return[T], 1)

	i.lock.Lock()
	defer i.lock.Unlock()

	// let's add the caller to the waiting list
	i.waiters = append(i.waiters, waiter)
	// signal the async invoker that it should invoke the function:
	// a) the async invoker has not started yet, and the channel
	// is empty, we send to the channel and wait
	// b) the async invoker has not started yet, and the channel
	// is not empty, we have already added the caller to the waiting list
	// c) the async invoker is blocked, waiting to receive on the channel
	// d) the async is unblocked, and is in making the call
	// e) the async invoker is sending the result to each waiter
	//
	// since this caller has the lock now, e is impossible
	//
	select {
	case i.invoke <- struct{}{}:
	default:
	}

	return waiter
}

func (i *asyncInvoker[T]) Run(stopCtx context.Context) context.Context {
	done, cancel := context.WithCancel(context.Background())
	go func() {
		klog.InfoS("AsyncInvoker: start")
		defer func() {
			klog.InfoS("AsyncInvoker: end")
			cancel()
		}()

		for {
			select {
			case <-stopCtx.Done():
				return
			case _, ok := <-i.invoke:
				if !ok {
					return
				}
			}

			var empty bool
			i.lock.Lock()
			empty = len(i.waiters) == 0
			i.lock.Unlock()
			if empty {
				continue
			}

			// while the call is in progress, we allow any new
			// caller to add itslef to the waiting list.
			ret := Return[T]{}
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						ret.Panicked = recovered
						utilruntime.HandleError(fmt.Errorf("panic from AsyncInvoker Run: %v", recovered))
					}
				}()

				func() {
					now := time.Now()
					defer func() {
						ret.Latency = time.Since(now)
					}()
					ret.Result = i.f()
				}()
			}()

			// we have just invoked the function, return the result
			// to the callers waiting, some callers might have given
			// up already,
			func() {
				i.lock.Lock()
				defer i.lock.Unlock()

				for _, waiter := range i.waiters {
					// this should never block, we created
					// this channel with a buffer of 1
					waiter <- ret
					close(waiter)
				}
				// reset the slice to zero-length
				i.waiters = i.waiters[:0]
			}()
		}
	}()

	return done
}
