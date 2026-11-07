/*
Copyright The Kubernetes Authors.

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

package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/streaming/pkg/httpstream"
)

// settleDelay is how long a test waits for the connection poller or a blocked
// Accept to reach the state the test is about to probe. It is a multiple of
// connectionPollInterval so that several poll iterations can happen.
const settleDelay = 5 * connectionPollInterval

// reactionTimeout bounds operations that must not wait for an in-flight dial.
// It is deliberately generous: the assertions below distinguish "returns
// without waiting for the dial" from "blocks until the dial finishes", they do
// not measure latency.
const reactionTimeout = 30 * time.Second

// --- test doubles for the httpstream interfaces -----------------------------

type fakeStream struct {
	headers http.Header

	closeOnce sync.Once
	closed    chan struct{}
}

var _ httpstream.Stream = &fakeStream{}

func newFakeStream(headers http.Header) *fakeStream {
	// The caller keeps modifying its header object, so copy it.
	return &fakeStream{headers: headers.Clone(), closed: make(chan struct{})}
}

// Read blocks until the stream is closed and then reports EOF. That mirrors
// the real error stream, whose reader goroutine only finishes once the stream
// goes away, and so lets the leak checks below catch a stream nobody closed.
func (s *fakeStream) Read(p []byte) (int, error) {
	<-s.closed
	return 0, io.EOF
}

func (s *fakeStream) Write(p []byte) (int, error) {
	select {
	case <-s.closed:
		return 0, io.ErrClosedPipe
	default:
		return len(p), nil
	}
}

func (s *fakeStream) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func (s *fakeStream) Reset() error         { return s.Close() }
func (s *fakeStream) Headers() http.Header { return s.headers }
func (s *fakeStream) Identifier() uint32   { return 1 }

type fakeConnection struct {
	closeOnce sync.Once
	closeChan chan bool

	mutex   sync.Mutex
	streams []*fakeStream
}

var _ httpstream.Connection = &fakeConnection{}

func newFakeConnection() *fakeConnection {
	return &fakeConnection{closeChan: make(chan bool)}
}

func (c *fakeConnection) CreateStream(headers http.Header) (httpstream.Stream, error) {
	s := newFakeStream(headers)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.streams = append(c.streams, s)
	return s, nil
}

func (c *fakeConnection) Close() error {
	c.closeOnce.Do(func() {
		close(c.closeChan)
		c.mutex.Lock()
		streams := append([]*fakeStream(nil), c.streams...)
		c.mutex.Unlock()
		for _, s := range streams {
			s.Close()
		}
	})
	return nil
}

func (c *fakeConnection) CloseChan() <-chan bool                     { return c.closeChan }
func (c *fakeConnection) SetIdleTimeout(timeout time.Duration)       {}
func (c *fakeConnection) RemoveStreams(streams ...httpstream.Stream) {}

func (c *fakeConnection) waitClosed(d time.Duration) bool {
	select {
	case <-c.closeChan:
		return true
	case <-time.After(d):
		return false
	}
}

// fakeDialer is a stub dialFunc. Its dial blocks until releaseDials is called,
// which is how these tests reproduce a connection attempt that hangs.
type fakeDialer struct {
	// honorContext selects which half of the real transport this models.
	//
	// true reproduces the TCP connect and the TLS handshake, which
	// SpdyRoundTripper performs with tls.Dialer.DialContext and
	// HandshakeContext and which therefore abort when the request context is
	// canceled. That is where a connection attempt to an unresponsive
	// apiserver actually gets stuck.
	//
	// false reproduces the window after a completed handshake, where
	// SpdyRoundTripper writes the request and reads the response straight off
	// the connection with no deadline and no context. Nothing but the peer
	// can end that wait, so the dial ignores cancellation entirely.
	honorContext bool

	release     chan struct{}
	releaseOnce sync.Once
	entered     chan struct{} // one token per dial call, buffered

	mutex sync.Mutex
	conns []*fakeConnection
}

func newFakeDialer(honorContext bool) *fakeDialer {
	return &fakeDialer{
		honorContext: honorContext,
		release:      make(chan struct{}),
		entered:      make(chan struct{}, 1024),
	}
}

func (d *fakeDialer) dial(ctx context.Context) (httpstream.Connection, error) {
	select {
	case d.entered <- struct{}{}:
	default:
	}

	if d.honorContext {
		select {
		case <-d.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		<-d.release
	}

	conn := newFakeConnection()
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.conns = append(d.conns, conn)
	return conn, nil
}

// releaseDials unblocks all current and future dials. It is safe to call more
// than once and from more than one goroutine.
func (d *fakeDialer) releaseDials() {
	d.releaseOnce.Do(func() { close(d.release) })
}

// waitDialStarted waits until the poller has entered the dial at least once.
func (d *fakeDialer) waitDialStarted(t *testing.T) {
	t.Helper()
	select {
	case <-d.entered:
	case <-time.After(reactionTimeout):
		t.Fatal("the connection poller never dialed")
	}
}

func (d *fakeDialer) connections() []*fakeConnection {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return append([]*fakeConnection(nil), d.conns...)
}

// --- helpers ----------------------------------------------------------------

var testAddr = Addr{
	Namespace:     "test-namespace",
	PodName:       "test-pod",
	ContainerName: "test-container",
	Port:          9000,
}

// newTestListener starts a listener whose pod is already running, so that the
// connection poller starts immediately.
func newTestListener(ctx context.Context, dialer dialFunc) *listener {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testAddr.Namespace,
			Name:      testAddr.PodName,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: testAddr.ContainerName}},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{{
				Name:  testAddr.ContainerName,
				State: v1.ContainerState{Running: &v1.ContainerStateRunning{}},
			}},
		},
	}
	return listenWithDialer(ctx, fake.NewClientset(pod), dialer, testAddr).(*listener)
}

// storedConnections returns the number of connections currently published for
// Accept to hand out.
func storedConnections(l *listener) int {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	n := 0
	for _, c := range l.connections {
		if c != nil {
			n++
		}
	}
	return n
}

func mustNotBlock(t *testing.T, what string, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(reactionTimeout):
		t.Fatalf("%s did not return while a dial was in flight; it is blocked on the listener mutex", what)
		return nil
	}
}

// waitFor polls until condition holds, failing the test if it never does.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(reactionTimeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// --- tests ------------------------------------------------------------------

// TestStalledDialDoesNotStarveListener is the regression test for a hang seen
// in CI: the connection poller held the listener mutex across the dial, so a
// connection attempt that stalled for minutes also blocked Accept and Close
// for minutes.
//
// The dial here ignores cancellation, which is the worst case the transport
// can produce. Accept and Close must stay responsive even then, because what
// unblocks them is the absence of the lock, not the cancellation.
func TestStalledDialDoesNotStarveListener(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dialer := newFakeDialer(false)
	defer dialer.releaseDials()

	l := newTestListener(ctx, dialer.dial)
	dialer.waitDialStarted(t)

	// Accept has to be running before Close, because the buggy version
	// wedged it on the mutex rather than on its own ctx check.
	acceptDone := make(chan error, 1)
	go func() {
		_, err := l.Accept()
		acceptDone <- err
	}()
	time.Sleep(settleDelay)

	closeDone := make(chan error, 1)
	go func() { closeDone <- l.Close() }()
	if err := mustNotBlock(t, "Close", closeDone); err != nil {
		t.Errorf("Close: %v", err)
	}

	if err := mustNotBlock(t, "Accept", acceptDone); err == nil {
		t.Error("Accept returned a connection, want an error after Close")
	}

	if n := storedConnections(l); n != 0 {
		t.Errorf("a dial that never completed published %d connection(s)", n)
	}
}

// TestCancelAbortsDialWithoutRelease proves that cancellation alone is enough
// to wind everything down: the dial is never released, so if the fix merely
// abandoned the connection attempt instead of aborting it, the goroutine doing
// it would still be parked here and goleak would fail.
//
// This is the property that makes the dial bounded rather than merely
// non-blocking, and it only holds because the request carries a context.
func TestCancelAbortsDialWithoutRelease(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Deliberately no releaseDials anywhere in this test.
	dialer := newFakeDialer(true)

	l := newTestListener(ctx, dialer.dial)
	dialer.waitDialStarted(t)

	closeDone := make(chan error, 1)
	go func() { closeDone <- l.Close() }()
	if err := mustNotBlock(t, "Close", closeDone); err != nil {
		t.Errorf("Close: %v", err)
	}

	if n := len(dialer.connections()); n != 0 {
		t.Errorf("an aborted dial produced %d connection(s), want 0", n)
	}
	if n := storedConnections(l); n != 0 {
		t.Errorf("an aborted dial published %d connection(s), want 0", n)
	}
}

// TestLateDialCompletionIsNotPublished covers the window the context cannot
// reach: the dial ignores cancellation and succeeds after the listener is
// already closed. That connection must be closed rather than leaked, and it
// must not be published to a caller that has already given up.
func TestLateDialCompletionIsNotPublished(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dialer := newFakeDialer(false)
	defer dialer.releaseDials()

	l := newTestListener(ctx, dialer.dial)
	dialer.waitDialStarted(t)

	closeDone := make(chan error, 1)
	go func() { closeDone <- l.Close() }()
	if err := mustNotBlock(t, "Close", closeDone); err != nil {
		t.Errorf("Close: %v", err)
	}

	// Let the stalled dial finish, too late to be of any use.
	dialer.releaseDials()

	waitFor(t, "the late dial to complete", func() bool {
		return len(dialer.connections()) > 0
	})
	for i, c := range dialer.connections() {
		if !c.waitClosed(reactionTimeout) {
			t.Errorf("connection %d from the late dial was leaked instead of closed", i)
		}
	}
	if n := storedConnections(l); n != 0 {
		t.Errorf("a late dial published %d connection(s) on a closed listener", n)
	}
}

// TestAcceptAndClose covers the normal path, so that the fix cannot regress
// into never handing out connections.
func TestAcceptAndClose(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dialer := newFakeDialer(true)
	dialer.releaseDials()

	l := newTestListener(ctx, dialer.dial)

	conn, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if conn.LocalAddr().String() != testAddr.String() {
		t.Errorf("LocalAddr = %q, want %q", conn.LocalAddr(), testAddr)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Everything the dialer produced belongs to the listener and must be
	// closed by Close, including connections dialed but never accepted.
	for i, c := range dialer.connections() {
		if !c.waitClosed(reactionTimeout) {
			t.Errorf("connection %d was still open after Close", i)
		}
	}

	if _, err := l.Accept(); err == nil {
		t.Error("Accept after Close returned a connection, want an error")
	}
}

// TestAcceptedConnectionFreesSlot checks that closing an accepted connection
// releases its slot and that the poller then refills it. Accept alone must not
// free the slot, because the caller is still using the connection.
func TestAcceptedConnectionFreesSlot(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dialer := newFakeDialer(true)
	dialer.releaseDials()

	l := newTestListener(ctx, dialer.dial)
	defer l.Close()

	waitFor(t, "all slots to fill", func() bool {
		return storedConnections(l) == maxConcurrentConnections
	})

	conn, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if n := storedConnections(l); n != maxConcurrentConnections {
		t.Errorf("Accept freed a slot: %d connections stored, want %d", n, maxConcurrentConnections)
	}

	dialedBefore := len(dialer.connections())
	if err := conn.Close(); err != nil {
		t.Fatalf("connection Close: %v", err)
	}

	waitFor(t, "the freed slot to be refilled", func() bool {
		return storedConnections(l) == maxConcurrentConnections &&
			len(dialer.connections()) > dialedBefore
	})
}

// TestConcurrentCloseAndDial exercises publication of new connections against
// concurrent Close and Accept calls. It is meant to be run under -race.
func TestConcurrentCloseAndDial(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dialer := newFakeDialer(true)
	dialer.releaseDials()

	l := newTestListener(ctx, dialer.dial)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := l.Accept()
			if err == nil {
				conn.Close()
			}
		}()
	}
	// Let a few connections get established first, so that Close races with
	// in-flight publication rather than with an idle listener.
	time.Sleep(settleDelay)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := l.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	wg.Wait()

	for i, c := range dialer.connections() {
		if !c.waitClosed(reactionTimeout) {
			t.Errorf("connection %d was still open after Close", i)
		}
	}
	if n := storedConnections(l); n != 0 {
		t.Errorf("%d connection(s) published after Close", n)
	}
}

// TestNoDialWhenAllSlotsAreTaken makes sure the free-slot check still bounds
// the number of connections now that it no longer happens under the same lock
// acquisition as the dial itself.
func TestNoDialWhenAllSlotsAreTaken(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dialer := newFakeDialer(true)
	dialer.releaseDials()

	l := newTestListener(ctx, dialer.dial)
	defer l.Close()

	waitFor(t, "all slots to fill", func() bool {
		return storedConnections(l) == maxConcurrentConnections
	})

	// Give the poller several more ticks: it must not dial past the limit.
	time.Sleep(settleDelay)
	if n := len(dialer.connections()); n > maxConcurrentConnections {
		t.Errorf("dialed %d connections, want at most %d", n, maxConcurrentConnections)
	}
	if n := storedConnections(l); n != maxConcurrentConnections {
		t.Errorf("published %d connections, want %d", n, maxConcurrentConnections)
	}
}

// testRequestURL is the shape of URL the port-forward request carries: the
// apiserver endpoint plus the pod path.
const testRequestURL = "https://api.test.example.com:6443/api/v1/namespaces/ns/pods/pod/portforward"

// transportError reproduces what client-go hands back when the SPDY upgrade
// request fails. spdy.Negotiate flattens the *url.Error with %v, not %w, so
// the context classification is already gone by the time dial sees it.
func transportError(cause error) error {
	return fmt.Errorf("error sending request: %v", &url.Error{
		Op:  "Post",
		URL: testRequestURL,
		Err: cause,
	})
}

// TestCanceledDialErrorOmitsRequestURL covers the one error the cancellation
// support newly produces. Shutting the listener down is routine, so the log
// line it leaves behind should not repeat the apiserver URL, and it has to
// stay classifiable as a cancellation.
func TestCanceledDialErrorOmitsRequestURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := dial(ctx, "test", func(ctx context.Context) (httpstream.Connection, error) {
		return nil, transportError(ctx.Err())
	}, testAddr.Port)

	if err == nil {
		t.Fatal("dial returned no error for a canceled context")
	}
	if got := err.Error(); strings.Contains(got, testRequestURL) ||
		strings.Contains(got, "api.test.example.com") {
		t.Errorf("canceled dial leaked the request URL: %q", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("canceled dial is not classifiable as context.Canceled: %v", err)
	}
}

// TestFailedDialErrorIsPreserved is the other half: a real failure must keep
// its full text. Diagnosing an unreachable apiserver needs the URL, so the
// sanitizing above must apply only to cancellation.
func TestFailedDialErrorIsPreserved(t *testing.T) {
	cause := errors.New("dial tcp 10.0.0.1:6443: connect: connection refused")

	// Not canceled: this is a genuine failure, not a shutdown.
	_, err := dial(context.Background(), "test", func(ctx context.Context) (httpstream.Connection, error) {
		return nil, transportError(cause)
	}, testAddr.Port)

	if err == nil {
		t.Fatal("dial returned no error for a failing dialer")
	}
	got := err.Error()
	if !strings.Contains(got, testRequestURL) {
		t.Errorf("failed dial dropped the request URL, losing diagnosis: %q", got)
	}
	if !strings.Contains(got, "connection refused") {
		t.Errorf("failed dial dropped the cause: %q", got)
	}
}
