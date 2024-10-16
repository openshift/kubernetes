package extensiontests

import "sync/atomic"

type Task interface {
	Run()
}

type RepeatableTask struct {
	fn func()
}

func (t *RepeatableTask) Run() {
	t.fn()
}

type OneTimeTask struct {
	fn       func()
	executed int32 // Atomic boolean to indicate whether the function has been run
}

func (t *OneTimeTask) Run() {
	// Ensure one-time tasks are only run once
	if atomic.CompareAndSwapInt32(&t.executed, 0, 1) {
		t.fn()
	}
}
