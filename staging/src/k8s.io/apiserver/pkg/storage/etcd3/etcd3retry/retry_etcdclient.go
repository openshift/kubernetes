package etcd3retry

import (
	"context"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
)

var EnableEtcdRetry = false

var defaultRetry = wait.Backoff{
	Duration: 300 * time.Millisecond,
	Factor:   2, // double the timeout for every failure
	Steps:    4, // .3 + .6 + 1.2 + 2.4 = 5ish  this let's us smooth out short bumps but not long ones and keeps retry behavior closer.
}

type retryClient struct {
	// embed because we only want to override a few states
	storage.Interface
}

// New returns an etcd3 implementation of storage.Interface.
func NewRetryingEtcdStorage(delegate storage.Interface) storage.Interface {
	if !EnableEtcdRetry {
		return delegate
	}
	return &retryClient{Interface: delegate}
}

// Create adds a new object at a key unless it already exists. 'ttl' is time-to-live
// in seconds (0 means forever). If no error is returned and out is not nil, out will be
// set to the read value from database.
func (c *retryClient) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	return onError(defaultRetry, isRetriableEtcdError, func() error {
		return c.Interface.Create(ctx, key, obj, out, ttl)
	})
}

// Delete removes the specified key and returns the value that existed at that spot.
// If key didn't exist, it will return NotFound storage error.
func (c *retryClient) Delete(ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions, validateDeletion storage.ValidateObjectFunc) error {
	return onError(defaultRetry, isRetriableEtcdError, func() error {
		return c.Interface.Delete(ctx, key, out, preconditions, validateDeletion)
	})
}

// Watch begins watching the specified key. Events are decoded into API objects,
// and any items selected by 'p' are sent down to returned watch.Interface.
// resourceVersion may be used to specify what version to begin watching,
// which should be the current resourceVersion, and no longer rv+1
// (e.g. reconnecting without missing any updates).
// If resource version is "0", this interface will get current object at given key
// and send it in an "ADDED" event, before watch starts.
func (c *retryClient) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	var ret watch.Interface
	err := onError(defaultRetry, alwaysRetry, func() error {
		var innerErr error
		ret, innerErr = c.Interface.Watch(ctx, key, opts)
		return innerErr
	})
	return ret, err
}

// WatchList begins watching the specified key's items. Items are decoded into API
// objects and any item selected by 'p' are sent down to returned watch.Interface.
// resourceVersion may be used to specify what version to begin watching,
// which should be the current resourceVersion, and no longer rv+1
// (e.g. reconnecting without missing any updates).
// If resource version is "0", this interface will list current objects directory defined by key
// and send them in "ADDED" events, before watch starts.
func (c *retryClient) WatchList(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	var ret watch.Interface
	err := onError(defaultRetry, alwaysRetry, func() error {
		var innerErr error
		ret, innerErr = c.Interface.WatchList(ctx, key, opts)
		return innerErr
	})
	return ret, err
}

// Get unmarshals json found at key into objPtr. On a not found error, will either
// return a zero object of the requested type, or an error, depending on 'opts.ignoreNotFound'.
// Treats empty responses and nil response nodes exactly like a not found error.
// The returned contents may be delayed, but it is guaranteed that they will
// match 'opts.ResourceVersion' according 'opts.ResourceVersionMatch'.
func (c *retryClient) Get(ctx context.Context, key string, opts storage.GetOptions, objPtr runtime.Object) error {
	return onError(defaultRetry, alwaysRetry, func() error {
		return c.Interface.Get(ctx, key, opts, objPtr)
	})
}

// GetToList unmarshals json found at key and opaque it into *List api object
// (an object that satisfies the runtime.IsList definition).
// The returned contents may be delayed, but it is guaranteed that they will
// match 'opts.ResourceVersion' according 'opts.ResourceVersionMatch'.
func (c *retryClient) GetToList(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	return onError(defaultRetry, alwaysRetry, func() error {
		return c.Interface.GetToList(ctx, key, opts, listObj)
	})
}

// List unmarshalls jsons found at directory defined by key and opaque them
// into *List api object (an object that satisfies runtime.IsList definition).
// The returned contents may be delayed, but it is guaranteed that they will
// match 'opts.ResourceVersion' according 'opts.ResourceVersionMatch'.
func (c *retryClient) List(ctx context.Context, key string, opts storage.ListOptions, listObj runtime.Object) error {
	return onError(defaultRetry, alwaysRetry, func() error {
		return c.Interface.List(ctx, key, opts, listObj)
	})
}

// GuaranteedUpdate keeps calling 'tryUpdate()' to update key 'key' (of type 'ptrToType')
// retrying the update until success if there is index conflict.
// Note that object passed to tryUpdate may change across invocations of tryUpdate() if
// other writers are simultaneously updating it, so tryUpdate() needs to take into account
// the current contents of the object when deciding how the update object should look.
// If the key doesn't exist, it will return NotFound storage error if ignoreNotFound=false
// or zero value in 'ptrToType' parameter otherwise.
// If the object to update has the same value as previous, it won't do any update
// but will return the object in 'ptrToType' parameter.
// If 'suggestion' can contain zero or one element - in such case this can be used as
// a suggestion about the current version of the object to avoid read operation from
// storage to get it.
//
// Example:
//
// s := /* implementation of Interface */
// err := s.GuaranteedUpdate(
//     "myKey", &MyType{}, true,
//     func(input runtime.Object, res ResponseMeta) (runtime.Object, *uint64, error) {
//       // Before each invocation of the user defined function, "input" is reset to
//       // current contents for "myKey" in database.
//       curr := input.(*MyType)  // Guaranteed to succeed.
//
//       // Make the modification
//       curr.Counter++
//
//       // Return the modified object - return an error to stop iterating. Return
//       // a uint64 to alter the TTL on the object, or nil to keep it the same value.
//       return cur, nil, nil
//    },
// )
func (c *retryClient) GuaranteedUpdate(
	ctx context.Context, key string, ptrToType runtime.Object, ignoreNotFound bool,
	precondtions *storage.Preconditions, tryUpdate storage.UpdateFunc, suggestion ...runtime.Object) error {
	return onError(defaultRetry, isRetriableEtcdError, func() error {
		return c.Interface.GuaranteedUpdate(ctx, key, ptrToType, ignoreNotFound, precondtions, tryUpdate, suggestion...)
	})
}

func isRetriableEtcdError(err error) bool {
	if err == nil {
		return true
	}

	// if we have a connection refused, we can retry
	if net.IsConnectionRefused(err) {
		// TODO this needs a metric
		return true
	}

	// TODO can we retry connection reset?  I don't actually know.

	// if the leader changed, we didn't do anything.
	// TODO this needs a metric
	if strings.Contains(err.Error(), "etcdserver: leader changed") {
		return true
	}

	return false
}

func alwaysRetry(_ error) bool {
	// TODO this needs a metric
	return true
}

// onError allows the caller to retry fn in case the error returned by fn is retriable
// according to the provided function. backoff defines the maximum retries and the wait
// interval between two retries.
func onError(backoff wait.Backoff, retriable func(error) bool, fn func() error) error {
	var lastErr error
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := fn()
		switch {
		case err == nil:
			return true, nil
		case retriable(err):
			lastErr = err
			return false, nil
		default:
			return false, err
		}
	})
	if err == wait.ErrWaitTimeout {
		err = lastErr
	}
	return err
}
