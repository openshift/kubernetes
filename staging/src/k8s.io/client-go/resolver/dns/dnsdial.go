package dns

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// dnsHandlerFunc signature for the function used to process the DNS packets
type dnsHandlerFunc func(id uint16, q dnsmessage.Question) []byte

// dnsConn implements an in memory DNS stream or packet connection
type dnsConn struct {
	wrMu sync.Mutex // Serialize Write operations
	rdMu sync.Mutex // Serialize Read operation

	readCh     chan []byte // Used to communicate Write and Read
	readBuffer bytes.Buffer

	once sync.Once // Protects closing the connection
	done chan struct{}

	readDeadline  connDeadline
	writeDeadline connDeadline

	streamConn bool // Stream or Packet connection

	dnsHandler dnsHandlerFunc // dns handler hook
}

var _ net.PacketConn = &dnsConn{}

// makeDNSConn creates a half-duplex, in-memory, connection where data written on the
// connection is processed by the dnsHandler hook and then read back on the same
// connection. Reads and Write are serialized, Writes are blocked by Reads.
func makeDNSConn(fn dnsHandlerFunc, streamConn bool) *dnsConn {
	return &dnsConn{
		readCh:        make(chan []byte, 1), // Serialize
		readBuffer:    bytes.Buffer{},
		done:          make(chan struct{}),
		readDeadline:  makeConnDeadline(),
		writeDeadline: makeConnDeadline(),
		streamConn:    streamConn,
		dnsHandler:    fn,
	}

}

// connection parameters (obtained from net.Pipe)
// https://cs.opensource.google/go/go/+/refs/tags/go1.17:src/net/pipe.go;bpv=0;bpt=1

// connDeadline is an abstraction for handling timeouts.
type connDeadline struct {
	mu     sync.Mutex // Guards timer and cancel
	timer  *time.Timer
	cancel chan struct{} // Must be non-nil
}

func makeConnDeadline() connDeadline {
	return connDeadline{cancel: make(chan struct{})}
}

// set sets the point in time when the deadline will time out.
// A timeout event is signaled by closing the channel returned by waiter.
// Once a timeout has occurred, the deadline can be refreshed by specifying a
// t value in the future.
//
// A zero value for t prevents timeout.
func (c *connDeadline) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.timer != nil && !c.timer.Stop() {
		<-c.cancel // Wait for the timer callback to finish and close cancel
	}
	c.timer = nil

	// Time is zero, then there is no deadline.
	closed := isClosedChan(c.cancel)
	if t.IsZero() {
		if closed {
			c.cancel = make(chan struct{})
		}
		return
	}

	// Time in the future, setup a timer to cancel in the future.
	if dur := time.Until(t); dur > 0 {
		if closed {
			c.cancel = make(chan struct{})
		}
		c.timer = time.AfterFunc(dur, func() {
			close(c.cancel)
		})
		return
	}

	// Time in the past, so close immediately.
	if !closed {
		close(c.cancel)
	}
}

// wait returns a channel that is closed when the deadline is exceeded.
func (c *connDeadline) wait() chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cancel
}

func isClosedChan(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

type memDNSAddress struct {
	addr string
}

func (m memDNSAddress) Network() string {
	return "MemDNS"
}
func (m memDNSAddress) String() string {
	return "MemDNS"
}

func (d *dnsConn) LocalAddr() net.Addr {
	return memDNSAddress{}
}
func (d *dnsConn) RemoteAddr() net.Addr {
	return memDNSAddress{}
}

func (d *dnsConn) Read(b []byte) (int, error) {
	n, err := d.read(b)
	if err != nil && err != io.EOF && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "read", Net: "MemDNS", Err: err}
	}
	return n, err
}

func (d *dnsConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := d.Read(b)
	return n, memDNSAddress{}, err
}

func (d *dnsConn) read(b []byte) (n int, err error) {
	d.rdMu.Lock()
	defer d.rdMu.Unlock()

	if len(b) == 0 {
		return 0, io.EOF
	}

	switch {
	case isClosedChan(d.done):
		return 0, io.ErrClosedPipe
	case isClosedChan(d.readDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	}

	// if the buffer was drained wait for new data
	if d.readBuffer.Len() == 0 {
		select {
		case <-d.done:
			return 0, io.EOF
		case <-d.readDeadline.wait():
			return 0, os.ErrDeadlineExceeded
		case bw := <-d.readCh:
			d.readBuffer.Write(bw)
		}
	}
	return d.readBuffer.Read(b)
}

func (d *dnsConn) Write(b []byte) (int, error) {
	n, err := d.write(b)
	if err != nil && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "write", Net: "MemDNS", Err: err}
	}
	return n, err
}

func (d *dnsConn) WriteTo(b []byte, _ net.Addr) (int, error) {
	return d.Write(b)
}

func (d *dnsConn) write(b []byte) (n int, err error) {
	switch {
	case isClosedChan(d.done):
		return 0, io.ErrClosedPipe
	case isClosedChan(d.writeDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	}

	// ensure the buffer is processed together
	d.wrMu.Lock()
	defer d.wrMu.Unlock()
	buf := make([]byte, len(b))
	nr := copy(buf, b)

	select {
	case <-d.done:
		return n, io.ErrClosedPipe
	case <-d.writeDeadline.wait():
		return n, os.ErrDeadlineExceeded
	case d.readCh <- d.dnsRoundTrip(buf):
		return nr, nil
	}

}

func (d *dnsConn) SetDeadline(t time.Time) error {
	if isClosedChan(d.done) {
		return io.ErrClosedPipe
	}
	d.readDeadline.set(t)
	d.writeDeadline.set(t)
	return nil
}

func (d *dnsConn) SetReadDeadline(t time.Time) error {
	if isClosedChan(d.done) {
		return io.ErrClosedPipe
	}
	d.readDeadline.set(t)
	return nil
}

func (d *dnsConn) SetWriteDeadline(t time.Time) error {
	if isClosedChan(d.done) {
		return io.ErrClosedPipe
	}
	d.writeDeadline.set(t)
	return nil
}

func (d *dnsConn) Close() error {
	d.once.Do(func() { close(d.done) })
	return nil
}

func (d *dnsConn) dnsRoundTrip(b []byte) []byte {
	debug := os.Getenv("DNS_DEBUG")

	var p dnsmessage.Parser
	hdr, err := p.Start(b)
	if err != nil {
		return dnsErrorMessage(hdr.ID, dnsmessage.RCodeFormatError, dnsmessage.Question{})
	}
	if d.dnsHandler == nil {
		return dnsErrorMessage(hdr.ID, dnsmessage.RCodeNotImplemented, dnsmessage.Question{})
	}
	// RFC1035 max 512 bytes for UDP
	if !d.streamConn && len(b) > 512 {
		return dnsErrorMessage(hdr.ID, dnsmessage.RCodeFormatError, dnsmessage.Question{})
	}

	if debug == "true" {
		var m dnsmessage.Message
		err := m.Unpack(b)
		if err != nil {
			log.Printf("Can't parse DNS packet: %v", err)
		}
		log.Printf("Received DNS packet: %s", m.GoString())
	}
	// Only support 1 question, ref:
	// https://cs.opensource.google/go/x/net/+/e898025e:dns/dnsmessage/message.go
	// Multiple questions are valid according to the spec,
	// but servers don't actually support them. There will
	// be at most one question here.
	questions, err := p.AllQuestions()
	if err != nil {
		return dnsErrorMessage(hdr.ID, dnsmessage.RCodeFormatError, dnsmessage.Question{})
	}
	if len(questions) > 1 {
		return dnsErrorMessage(hdr.ID, dnsmessage.RCodeNotImplemented, dnsmessage.Question{})
	} else if len(questions) == 0 {
		return dnsErrorMessage(hdr.ID, dnsmessage.RCodeFormatError, dnsmessage.Question{})
	}

	answer := d.dnsHandler(hdr.ID, questions[0])
	// Return a truncated packet if the answer is too big
	if !d.streamConn && len(answer) > 512 {
		answer = dnsTruncatedMessage(hdr.ID, questions[0])
	}
	if debug == "true" {
		var m dnsmessage.Message
		err := m.Unpack(answer)
		if err != nil {
			log.Printf("Can't parse DNS packet: %v", err)
		}
		log.Printf("Sending DNS packet: %s", m.GoString())
	}

	return answer
}

// dnsErrorMessage return an encoded dns error message
func dnsErrorMessage(id uint16, rcode dnsmessage.RCode, q dnsmessage.Question) []byte {
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:            id,
			Response:      true,
			Authoritative: true,
			RCode:         rcode,
		},
		Questions: []dnsmessage.Question{q},
	}
	buf, err := msg.Pack()
	if err != nil {
		panic(err)
	}
	return buf
}

func dnsTruncatedMessage(id uint16, q dnsmessage.Question) []byte {
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:            id,
			Response:      true,
			Authoritative: true,
			Truncated:     true,
			RCode:         dnsmessage.RCodeFormatError,
		},
		Questions: []dnsmessage.Question{q},
	}
	buf, err := msg.Pack()
	if err != nil {
		panic(err)
	}
	return buf
}

// MemDNSDialer allow to use an in memory connection to process DNS packets with a custom function.
type MemDNSDialer struct {
	DNSHandler dnsHandlerFunc
}

// Dial creates an in memory connection that is processed by the packet handler
func (m *MemDNSDialer) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	streamConn := false
	if strings.Contains(network, "tcp") {
		streamConn = true
	}
	return makeDNSConn(m.DNSHandler, streamConn), nil
}
