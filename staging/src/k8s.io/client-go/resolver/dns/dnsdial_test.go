package dns

// https://github.com/golang/go/blob/master/src/net/dnsclient_unix_test.go

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Test address from 192.0.2.0/24 block, reserved by RFC 5737 for documentation.
var TestAddr = [4]byte{0xc0, 0x00, 0x02, 0x01}

// Test address from 2001:db8::/32 block, reserved by RFC 3849 for documentation.
var TestAddr6 = [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

func mustNewName(name string) dnsmessage.Name {
	nn, err := dnsmessage.NewName(name)
	if err != nil {
		panic(fmt.Sprint("creating name: ", err))
	}
	return nn
}

func mustQuestion(name string, qtype dnsmessage.Type, class dnsmessage.Class) dnsmessage.Question {
	return dnsmessage.Question{
		Name:  mustNewName(name),
		Type:  qtype,
		Class: class,
	}
}

func makeDNSMessage() []byte {
	r := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID: 1,
		},
		Questions: []dnsmessage.Question{
			mustQuestion("test.", dnsmessage.TypeA, dnsmessage.ClassINET),
		},
	}
	buf, err := r.Pack()
	if err != nil {
		panic(err)
	}
	return buf
}

func makeLargeDNSMessage() []byte {
	r := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:       1,
			Response: true,
			RCode:    dnsmessage.RCodeSuccess,
		},
		Questions: []dnsmessage.Question{
			mustQuestion("test.", dnsmessage.TypeA, dnsmessage.ClassINET),
		},
		Answers: []dnsmessage.Resource{
			{
				Header: dnsmessage.ResourceHeader{
					Name:  mustNewName("test."),
					Type:  dnsmessage.TypeTXT,
					Class: dnsmessage.ClassINET,
				},
				Body: &dnsmessage.TXTResource{
					TXT: []string{"ok",
						strings.Repeat("abcde12345", 25), // 10 * 25  = 250
						strings.Repeat("abcde12345", 25), // 10 * 25  = 250
						strings.Repeat("abcde12345", 25), // 10 * 25  = 250
						strings.Repeat("abcde12345", 25), // 10 * 25  = 250
					}, // 250 * 4 + 2 = 1002
				},
			},
		},
	}
	buf, err := r.Pack()
	if err != nil {
		panic(err)
	}
	return buf
}

func makeMemDNSConn() net.Conn {
	f := func(id uint16, q dnsmessage.Question) []byte {
		r := dnsmessage.Message{
			Header: dnsmessage.Header{
				ID:       id,
				Response: true,
				RCode:    dnsmessage.RCodeSuccess,
			},
			Questions: []dnsmessage.Question{q},
			Answers: []dnsmessage.Resource{
				{
					Header: dnsmessage.ResourceHeader{
						Name:   q.Name,
						Type:   dnsmessage.TypeA,
						Class:  dnsmessage.ClassINET,
						Length: 4,
					},
					Body: &dnsmessage.AResource{
						A: TestAddr,
					},
				},
			},
		}

		buf, err := r.Pack()
		if err != nil {
			panic(err)
		}
		return buf

	}
	r := MemDNSDialer{DNSHandler: f}
	c, err := r.Dial(context.Background(), "", "8.8.8.8")
	if err != nil {
		panic(err)
	}
	return c
}

func TestDNSDial(t *testing.T) {
	t.Parallel()
	tests := []struct {
		network string
	}{
		{"tcp"},
		{"udp"},
	}

	f := func(id uint16, q dnsmessage.Question) []byte {
		r := dnsmessage.Message{
			Header: dnsmessage.Header{
				ID:       id,
				Response: true,
				RCode:    dnsmessage.RCodeSuccess,
			},
			Questions: []dnsmessage.Question{q},
			Answers: []dnsmessage.Resource{
				{
					Header: dnsmessage.ResourceHeader{
						Name:   q.Name,
						Type:   dnsmessage.TypeA,
						Class:  dnsmessage.ClassINET,
						Length: 4,
					},
					Body: &dnsmessage.AResource{
						A: TestAddr,
					},
				},
			},
		}

		buf, err := r.Pack()
		if err != nil {
			panic(err)
		}
		return buf

	}
	r := MemDNSDialer{DNSHandler: f}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < len(tests); i++ {
		tt := tests[i]
		c, err := r.Dial(ctx, tt.network, "8.8.8.8")
		if err != nil {
			t.Error(err)
		}
		testRoundtrip(t, c)
	}

}

func TestDNSLargeDialTCP(t *testing.T) {
	f := func(id uint16, q dnsmessage.Question) []byte {
		return makeLargeDNSMessage()

	}
	r := MemDNSDialer{DNSHandler: f}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := r.Dial(ctx, "tcp", "8.8.8.8")
	if err != nil {
		t.Error(err)
	}
	c.Write(makeDNSMessage())
	buf := make([]byte, 2048)
	c.Read(buf)
	if len(buf) < 1000 {
		t.Fatalf("expected a large packet")
	}
	var p dnsmessage.Parser
	h, err := p.Start(buf)
	if err != nil {
		t.Fatalf("can't parse dns message: %v", err)
	}
	questions, err := p.AllQuestions()
	if err != nil {
		t.Fatalf("can't parse dns questions: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, received: %v", len(questions))
	}
	if h.RCode != dnsmessage.RCodeSuccess {
		t.Errorf("got %v; want %v", h.RCode, dnsmessage.RCodeSuccess)
	}
	a, err := p.AnswerHeader()
	if err != nil {
		t.Fatalf("can't parse dns answer: %v", err)
	}

	if a.Type != dnsmessage.TypeTXT || a.Class != dnsmessage.ClassINET {
		t.Fatalf("unexpected Type: got %v want TXT; Class: got %v want INET", a.Type, a.Class)

	}

	txt, err := p.TXTResource()
	if err != nil {
		t.Fatalf("can't parse dns answer resource: %v", err)
	}
	if txt.TXT[0] != "ok" {
		t.Fatalf("expected addr: %v, got: %v", TestAddr, txt.TXT[0])

	}

}

var (
	aLongTimeAgo = time.Unix(233431200, 0)
	neverTimeout = time.Time{}
)

// TestRacyRead tests that it is safe to mutate the input Read buffer
// immediately after cancelation has occurred.
func TestRacyRead(t *testing.T) {

	c := makeMemDNSConn()

	var wg sync.WaitGroup
	defer wg.Wait()

	c.SetReadDeadline(time.Now().Add(time.Millisecond))
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			b1 := make([]byte, 1024)
			b2 := make([]byte, 1024)
			for j := 0; j < 100; j++ {
				_, err := c.Read(b1)
				copy(b1, b2) // Mutate b1 to trigger potential race
				if err != nil {
					checkForTimeoutError(t, err)
					c.SetReadDeadline(time.Now().Add(time.Millisecond))
				}
			}
		}()
	}
}

// TestRacyWrite tests that it is safe to mutate the input Write buffer
// immediately after cancelation has occurred.
func TestRacyWrite(t *testing.T) {
	c := makeMemDNSConn()
	go c.Write(makeDNSMessage())

	var wg sync.WaitGroup
	defer wg.Wait()

	c.SetWriteDeadline(time.Now().Add(time.Millisecond))
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			b1 := makeDNSMessage()
			b2 := makeDNSMessage()
			for j := 0; j < 100; j++ {
				_, err := c.Write(b1)
				copy(b1, b2) // Mutate b1 to trigger potential race
				if err != nil {
					checkForTimeoutError(t, err)
					c.SetWriteDeadline(time.Now().Add(time.Millisecond))
				}
			}
		}()
	}
}

// TestWriteReadTimeout tests that Write timeouts do not affect Read.
// DNSConn blocks reads until there is some data, so Read has to timeout.
func TestWriteReadTimeout(t *testing.T) {
	c := makeMemDNSConn()
	go c.Write(makeDNSMessage())

	c.SetWriteDeadline(aLongTimeAgo)
	_, err := c.Write(make([]byte, 1024))
	checkForTimeoutError(t, err)

	c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	c.Write(makeDNSMessage())
	_, err = c.Read(make([]byte, 1024))
	checkForTimeoutError(t, err)

}

// testPastTimeout tests that a deadline set in the past immediately times out
// Read and Write requests.
func TestPastTimeout(t *testing.T) {
	c := makeMemDNSConn()

	c.SetDeadline(aLongTimeAgo)
	n, err := c.Write(makeDNSMessage())
	if n != 0 {
		t.Errorf("unexpected Write count: got %d, want 0", n)
	}
	checkForTimeoutError(t, err)
	n, err = c.Read(make([]byte, 1024))
	if n != 0 {
		t.Errorf("unexpected Read count: got %d, want 0", n)
	}
	checkForTimeoutError(t, err)
}

// testPresentTimeout tests that a past deadline set while there are pending
// Read and Write operations immediately times out those operations.
func TestPresentTimeout(t *testing.T) {
	f := func(id uint16, q dnsmessage.Question) []byte {
		time.Sleep(200 * time.Millisecond)
		return makeDNSMessage()

	}
	r := MemDNSDialer{DNSHandler: f}
	c, err := r.Dial(context.Background(), "", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Add(3)

	deadlineSet := make(chan bool, 1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		deadlineSet <- true
		c.SetReadDeadline(aLongTimeAgo)
		c.SetWriteDeadline(aLongTimeAgo)
	}()
	go func() {
		defer wg.Done()
		n, err := c.Read(make([]byte, 1024))
		if n != 0 {
			t.Errorf("unexpected Read count: got %d, want 0", n)
		}
		checkForTimeoutError(t, err)
		if len(deadlineSet) == 0 {
			t.Error("Read timed out before deadline is set")
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		for err == nil {
			_, err = c.Write(makeDNSMessage())
		}
		checkForTimeoutError(t, err)
		if len(deadlineSet) == 0 {
			t.Error("Write timed out before deadline is set")
		}
	}()
}

// testFutureTimeout tests that a future deadline will eventually time out
// Read and Write operations.
func TestFutureTimeout(t *testing.T) {
	f := func(id uint16, q dnsmessage.Question) []byte {
		time.Sleep(200 * time.Millisecond)
		return makeDNSMessage()
	}
	r := MemDNSDialer{DNSHandler: f}
	c, err := r.Dial(context.Background(), "", "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)

	c.SetDeadline(time.Now().Add(100 * time.Millisecond))
	go func() {
		defer wg.Done()
		_, err := c.Read(make([]byte, 1024))
		checkForTimeoutError(t, err)
	}()
	go func() {
		defer wg.Done()
		var err error
		for err == nil {
			_, err = c.Write(makeDNSMessage())
		}
		checkForTimeoutError(t, err)
	}()
	wg.Wait()

}

// testCloseTimeout tests that calling Close immediately times out pending
// Read and Write operations.
func TestCloseTimeout(t *testing.T) {
	c := makeMemDNSConn()
	go c.Write(makeDNSMessage())

	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Add(3)

	// Test for cancelation upon connection closure.
	c.SetDeadline(neverTimeout)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		c.Close()
	}()
	go func() {
		defer wg.Done()
		var err error
		buf := make([]byte, 1024)
		for err == nil {
			_, err = c.Read(buf)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		for err == nil {
			_, err = c.Write(makeDNSMessage())
		}
	}()
}

// testConcurrentMethods tests that the methods of net.Conn can safely
// be called concurrently.
func TestConcurrentMethods(t *testing.T) {
	c := makeMemDNSConn()
	go c.Write(makeDNSMessage())
	// The results of the calls may be nonsensical, but this should
	// not trigger a race detector warning.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(7)
		go func() {
			defer wg.Done()
			c.Read(make([]byte, 1024))
		}()
		go func() {
			defer wg.Done()
			c.Write(makeDNSMessage())
		}()
		go func() {
			defer wg.Done()
			c.SetDeadline(time.Now().Add(10 * time.Millisecond))
		}()
		go func() {
			defer wg.Done()
			c.SetReadDeadline(aLongTimeAgo)
		}()
		go func() {
			defer wg.Done()
			c.SetWriteDeadline(aLongTimeAgo)
		}()
		go func() {
			defer wg.Done()
			c.LocalAddr()
		}()
		go func() {
			defer wg.Done()
			c.RemoteAddr()
		}()
	}
	wg.Wait() // At worst, the deadline is set 10ms into the future

	resyncConn(t, c)
	testRoundtrip(t, c)
}

// checkForTimeoutError checks that the error satisfies the Error interface
// and that Timeout returns true.
func checkForTimeoutError(t *testing.T, err error) {
	t.Helper()
	if nerr, ok := err.(net.Error); ok {
		if !nerr.Timeout() {
			t.Errorf("err.Timeout() = false, want true")
		}
	} else {
		t.Errorf("got %T, want net.Error", err)
	}
}

// testRoundtrip writes something into c and reads it back.
func testRoundtrip(t *testing.T, c net.Conn) {
	t.Helper()

	if err := c.SetDeadline(neverTimeout); err != nil {
		t.Errorf("roundtrip SetDeadline error: %v", err)
	}

	_, err := c.Write(makeDNSMessage())
	if err != nil {
		t.Fatalf("can't write to the connection: %v", err)
	}
	buf := make([]byte, 2048)
	if _, err := c.Read(buf); err != nil {
		t.Errorf("roundtrip Read error: %v", err)
	}
	var p dnsmessage.Parser
	h, err := p.Start(buf)
	if err != nil {
		t.Fatalf("can't parse dns message: %v", err)
	}
	questions, err := p.AllQuestions()
	if err != nil {
		t.Fatalf("can't parse dns questions: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, received: %v", len(questions))
	}
	if h.RCode != dnsmessage.RCodeSuccess {
		t.Errorf("got %v; want %v", h.RCode, dnsmessage.RCodeSuccess)
	}
	a, err := p.AnswerHeader()
	if err != nil {
		t.Fatalf("can't parse dns answer: %v", err)
	}

	if a.Type != dnsmessage.TypeA || a.Class != dnsmessage.ClassINET {
		t.Fatalf("unexpected Type: got %v want A; Class: got %v want INET", a.Type, a.Class)

	}
	ar, err := p.AResource()
	if err != nil {
		t.Fatalf("can't parse dns answe resourcer: %v", err)
	}
	if !reflect.DeepEqual(TestAddr, ar.A) {
		t.Fatalf("expected addr: %v, got: %v", TestAddr, ar.A)

	}
}

// resyncConn resynchronizes the connection into a sane state.
// It just need to drain the read buffer, we have to set a timeout because it
// can be blocked until there is one Write.
func resyncConn(t *testing.T, c net.Conn) {
	t.Helper()
	c.SetDeadline(time.Now().Add(500 * time.Millisecond))
	// read from the buffer
	c.Read(make([]byte, 1024))
}
