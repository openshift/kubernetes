/*
Copyright 2020 The Kubernetes Authors.

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
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/klog/v2"
	"k8s.io/streaming/pkg/httpstream"
)

// Maximum number of forwarded connections. In practice we don't
// need more than one per sidecar and kubelet. Keeping this reasonably
// small ensures that we don't establish connections through the apiserver
// and the remote kernel which then aren't needed.
//
// The proxy code below establishes this many connections in advance,
// without waiting for a client on the remote side. On the local side
// a gRPC server will accept the same number of connections and then wait
// for data from a future client.
//
// This approach has the advantage that a client on the remote side can
// immediately start communicating, without the delay caused by establishing
// the connection. That delay is large enough that clients like the
// node-driver-registrar with a very small timeout for gRPC did indeed
// time out unnecessarily.
const maxConcurrentConnections = 10

// This delay determines how quickly we notice when someone has
// connected inside the cluster. With socat, we cannot make this too small
// because otherwise we get many rejected connections. With the mock
// driver as proxy that doesn't happen as long as we don't
// ask for too many concurrent connections because the mock driver
// keeps the listening port open at all times and the Linux
// kernel automatically accepts our connection requests.
const connectionPollInterval = 100 * time.Millisecond

// Listen creates a listener which returns new connections whenever someone connects
// to a socat or mock driver proxy instance running inside the given pod.
//
// socat must by started with "<listen>,fork TCP-LISTEN:<port>,reuseport"
// for this to work. "<listen>" can be anything that accepts connections,
// for example "UNIX-LISTEN:/csi/csi.sock". In this mode, socat will
// accept exactly one connection on the given port for each connection
// that socat itself accepted.
//
// Listening stops when the context is done or Close() is called.
func Listen(ctx context.Context, clientset kubernetes.Interface, restConfig *rest.Config, addr Addr) (net.Listener, error) {
	// We connect through port forwarding. Strictly
	// speaking this is overkill because we don't need a local
	// port. But this way we can reuse client-go/tools/portforward
	// instead of having to replicate handleConnection
	// in our own code.
	restClient := clientset.CoreV1().RESTClient()
	if restConfig.GroupVersion == nil {
		restConfig.GroupVersion = &schema.GroupVersion{}
	}
	if restConfig.NegotiatedSerializer == nil {
		restConfig.NegotiatedSerializer = scheme.Codecs
	}

	// The setup code around the actual portforward is from
	// https://github.com/kubernetes/kubernetes/blob/c652ffbe4a29143623a1aaec39f745575f7e43ad/staging/src/k8s.io/kubectl/pkg/cmd/portforward/portforward.go
	req := restClient.Post().
		Resource("pods").
		Namespace(addr.Namespace).
		Name(addr.PodName).
		SubResource("portforward")
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create round tripper: %w", err)
	}
	return listenWithDialer(ctx, clientset, spdyDialer(upgrader, &http.Client{Transport: transport}, req.URL()), addr), nil
}

// dialFunc opens a new SPDY connection to the pod. It takes a context because
// httpstream.Dialer.Dial does not.
type dialFunc func(ctx context.Context) (httpstream.Connection, error)

// spdyDialer returns a dialFunc which puts ctx on the port-forward request, so
// that a stalled connection attempt is actually aborted when ctx is canceled.
//
// spdy.NewDialerForStreaming cannot do this: its Dial builds the request with
// http.NewRequest, so the request always carries context.Background() and the
// DialContext and HandshakeContext calls inside SpdyRoundTripper have nothing
// to cancel on. spdy.NegotiateStreaming runs the same negotiation but takes
// the request from us, so we can supply one that carries ctx.
//
// Cancellation reaches the TCP connect and the TLS handshake, which is where a
// connection to an unresponsive apiserver gets stuck. It does not reach the
// request write and the response read that follow a completed handshake:
// SpdyRoundTripper performs those directly on the connection without a
// deadline. A stall in that window therefore still blocks this dial, so
// callers must not hold a lock across it.
func spdyDialer(upgrader spdy.Upgrader, client *http.Client, reqURL *url.URL) dialFunc {
	return func(ctx context.Context) (httpstream.Connection, error) {
		req, err := http.NewRequestWithContext(ctx, "POST", reqURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		conn, _, err := spdy.NegotiateStreaming(upgrader, client, req, portforward.PortForwardProtocolV1Name)
		return conn, err
	}
}

// listenWithDialer implements Listen for an already constructed dialer. It
// exists so that tests can drive the listener with a stub dialer.
func listenWithDialer(ctx context.Context, clientset kubernetes.Interface, dialer dialFunc, addr Addr) net.Listener {
	prefix := fmt.Sprintf("port forwarding for %s", addr)
	ctx, cancel := context.WithCancel(ctx)
	l := &listener{
		ctx:    ctx,
		cancel: cancel,
		addr:   addr,
	}

	var connectionsCreated int

	runForwarding := func() {
		klog.V(2).Infof("%s: starting connection polling", prefix)
		defer klog.V(2).Infof("%s: connection polling ended", prefix)

		tryConnect := time.NewTicker(connectionPollInterval)
		defer tryConnect.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tryConnect.C:
				if !l.hasFreeSlot() {
					continue
				}

				// The mutex must not be held while dialing: dial can block
				// for a long time (it has no deadline of its own beyond the
				// context) and Accept and Close need the mutex to stay
				// responsive. Dialing here is still serialized, because this
				// is the only goroutine that dials.
				//
				// This can fail repeatedly while the apiserver is down. This is nothing to worry about.
				klog.V(7).Infof("%s: trying to create a new connection #%d", prefix, connectionsCreated)
				stream, err := dial(ctx, fmt.Sprintf("%s #%d", prefix, connectionsCreated), dialer, addr.Port)
				if err != nil {
					klog.V(7).Infof("%s: no connection: %v", prefix, err)
					continue
				}
				// Make the connection available to Accept below. The listener
				// may have been closed or filled up while dialing, in which
				// case the stream is ours to discard.
				if !l.addConnection(stream, connectionsCreated) {
					klog.V(5).Infof("%s: discarding connection #%d, listener no longer accepts it", prefix, connectionsCreated)
					stream.Close()
					continue
				}
				klog.V(5).Infof("%s: created a new connection #%d", prefix, connectionsCreated)
				connectionsCreated++
			}
		}
	}

	// Portforwarding and polling for connections run in the background.
	go func() {
		for {
			running := false
			pod, err := clientset.CoreV1().Pods(addr.Namespace).Get(ctx, addr.PodName, metav1.GetOptions{})
			if err != nil {
				klog.V(5).Infof("checking for container %q in pod %s/%s: %v", addr.ContainerName, addr.Namespace, addr.PodName, err)
			}
			for i, status := range pod.Status.ContainerStatuses {
				if pod.Spec.Containers[i].Name == addr.ContainerName &&
					status.State.Running != nil {
					running = true
					break
				}
			}

			if running {
				klog.V(2).Infof("container %q in pod %s/%s is running", addr.ContainerName, addr.Namespace, addr.PodName)
				runForwarding()
			}

			select {
			case <-ctx.Done():
				return
			// Sleep a bit before restarting. This is
			// where we potentially wait for the pod to
			// start.
			case <-time.After(1 * time.Second):
			}
		}
	}()

	return l
}

// Addr contains all relevant parameters for a certain port in a pod.
// The container must be running before connections are attempted.
type Addr struct {
	Namespace, PodName, ContainerName string
	Port                              int
}

var _ net.Addr = Addr{}

func (a Addr) Network() string {
	return "port-forwarding"
}

func (a Addr) String() string {
	return fmt.Sprintf("%s/%s:%d", a.Namespace, a.PodName, a.Port)
}

type stream struct {
	httpstream.Stream
	streamConn httpstream.Connection
}

// dial creates a new port-forwarding stream. Canceling ctx aborts the
// connection attempt as far as the transport allows; see spdyDialer.
func dial(ctx context.Context, prefix string, dialer dialFunc, port int) (s *stream, finalErr error) {
	streamConn, err := dialer(ctx)
	if err != nil {
		return nil, fmt.Errorf("dialer failed: %w", err)
	}
	requestID := "1"
	defer func() {
		if finalErr != nil {
			streamConn.Close()
		}
	}()

	// create error stream
	headers := http.Header{}
	headers.Set(v1.StreamType, v1.StreamTypeError)
	headers.Set(v1.PortHeader, fmt.Sprintf("%d", port))
	headers.Set(v1.PortForwardRequestIDHeader, requestID)

	// We're not writing to this stream, just reading an error message from it.
	// This happens asynchronously.
	errorStream, err := streamConn.CreateStream(headers)
	if err != nil {
		return nil, fmt.Errorf("error creating error stream: %w", err)
	}
	errorStream.Close()
	go func() {
		message, err := io.ReadAll(errorStream)
		switch {
		case err != nil:
			klog.Errorf("%s: error reading from error stream: %v", prefix, err)
		case len(message) > 0:
			klog.Errorf("%s: an error occurred connecting to the remote port: %v", prefix, string(message))
		}
	}()

	// create data stream
	headers.Set(v1.StreamType, v1.StreamTypeData)
	dataStream, err := streamConn.CreateStream(headers)
	if err != nil {
		return nil, fmt.Errorf("error creating data stream: %w", err)
	}

	return &stream{
		Stream:     dataStream,
		streamConn: streamConn,
	}, nil
}

func (s *stream) Close() {
	s.Stream.Close()
	s.streamConn.Close()
}

type listener struct {
	addr   Addr
	ctx    context.Context
	cancel func()

	mutex       sync.Mutex
	closed      bool
	connections [maxConcurrentConnections]*connection
}

var _ net.Listener = &listener{}

// hasFreeSlot reports whether a new connection could be stored. It is only a
// hint: the answer may already be stale when the caller acts on it, which is
// why addConnection checks again.
func (l *listener) hasFreeSlot() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.closed {
		return false
	}
	for _, c := range l.connections {
		if c == nil {
			return true
		}
	}
	return false
}

// addConnection publishes a freshly dialed stream so that Accept can return
// it. It reports false when the listener was closed or ran out of slots while
// the dial was in flight; the caller then still owns the stream and must close
// it.
func (l *listener) addConnection(s *stream, counter int) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.closed {
		return false
	}
	for i, c := range l.connections {
		if c == nil {
			l.connections[i] = &connection{
				l:       l,
				stream:  s,
				addr:    l.addr,
				counter: counter,
			}
			return true
		}
	}
	return false
}

func (l *listener) Close() error {
	klog.V(5).Infof("forward listener for %s: closing", l.addr)
	l.cancel()

	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.closed = true
	for i, c := range l.connections {
		if c != nil {
			c.stream.Close()
			// Drop the reference: a closed listener owns nothing, so it
			// must not keep connections and their streams alive.
			l.connections[i] = nil
		}
	}

	return nil
}

func (l *listener) Accept() (net.Conn, error) {
	tryAccept := time.NewTicker(connectionPollInterval)
	defer tryAccept.Stop()
	for {
		select {
		case <-l.ctx.Done():
			return nil, errors.New("listening was stopped")
		case <-tryAccept.C:
			conn := func() net.Conn {
				l.mutex.Lock()
				defer l.mutex.Unlock()

				for _, c := range l.connections {
					if c != nil && !c.accepted {
						klog.V(5).Infof("forward listener for %s: got a new connection #%d", l.addr, c.counter)
						c.accepted = true
						return c
					}
				}
				return nil
			}()
			if conn != nil {
				return conn, nil
			}
		}
	}
}

type connection struct {
	l                *listener
	stream           *stream
	addr             Addr
	counter          int
	mutex            sync.Mutex
	accepted, closed bool
}

var _ net.Conn = &connection{}

func (c *connection) LocalAddr() net.Addr {
	return c.addr
}

func (c *connection) RemoteAddr() net.Addr {
	return c.addr
}

func (c *connection) SetDeadline(t time.Time) error {
	return nil
}

func (c *connection) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *connection) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *connection) Read(b []byte) (int, error) {
	n, err := c.stream.Read(b)
	if errors.Is(err, io.EOF) {
		klog.V(5).Infof("forward connection #%d for %s: remote side closed the stream", c.counter, c.addr)
	}
	return n, err
}

func (c *connection) Write(b []byte) (int, error) {
	n, err := c.stream.Write(b)
	if errors.Is(err, io.EOF) {
		klog.V(5).Infof("forward connection #%d for %s: remote side closed the stream", c.counter, c.addr)
	}
	return n, err
}

func (c *connection) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !c.closed {
		// Do the logging and book-keeping only once. The function itself may be called more than once.
		klog.V(5).Infof("forward connection #%d for %s: closing our side", c.counter, c.addr)

		c.l.mutex.Lock()
		defer c.l.mutex.Unlock()
		for i, c2 := range c.l.connections {
			if c2 == c {
				c.l.connections[i] = nil
				break
			}
		}
	}
	c.stream.Close()

	return nil
}

func (l *listener) Addr() net.Addr {
	return l.addr
}
