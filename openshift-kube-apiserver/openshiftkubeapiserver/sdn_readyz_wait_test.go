package openshiftkubeapiserver

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

func endpointsWithAddresses(ips ...string) *corev1.Endpoints {
	addresses := []corev1.EndpointAddress{}
	for _, ip := range ips {
		addresses = append(addresses, corev1.EndpointAddress{IP: ip})
	}
	return &corev1.Endpoints{
		Subsets: []corev1.EndpointSubset{
			{Addresses: addresses},
		},
	}
}

// newTLSServer starts a TLS server returning 200 and gives back its host and port.
func newTLSServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("failed to split test server host/port: %v", err)
	}
	return server, host, port
}

func TestAllEndpointsReachableAllUp(t *testing.T) {
	server, host, port := newTLSServer(t)
	defer server.Close()

	client := server.Client()
	client.Timeout = 1 * time.Second

	if !allEndpointsReachable(client, endpointsWithAddresses(host), port) {
		t.Errorf("expected reachable when the only endpoint is up")
	}
}

func TestAllEndpointsReachableAnyHTTPResponseCounts(t *testing.T) {
	// a 500 response still means we made contact
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(u.Host)

	client := server.Client()
	client.Timeout = 1 * time.Second

	if !allEndpointsReachable(client, endpointsWithAddresses(host), port) {
		t.Errorf("expected reachable when the endpoint answers with any http response")
	}
}

func TestAllEndpointsReachableOneDown(t *testing.T) {
	server, host, port := newTLSServer(t)
	defer server.Close()

	// grab a port with no listener for the down endpoint
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate port: %v", err)
	}
	downHost, downPort, _ := net.SplitHostPort(listener.Addr().String())
	listener.Close()
	if downPort != port {
		// allEndpointsReachable probes a single port for all addresses; to simulate one
		// down endpoint we need an address that refuses connections on the same port.
		// 127.0.0.2 on the server port is not listening in the test environment.
		downHost = "127.0.0.2"
	}

	client := server.Client()
	client.Timeout = 1 * time.Second

	if allEndpointsReachable(client, endpointsWithAddresses(host, downHost), port) {
		t.Errorf("expected not reachable when one of two endpoints is down")
	}
}

func TestAllEndpointsReachableNoAddresses(t *testing.T) {
	client := &http.Client{Timeout: 1 * time.Second}

	if allEndpointsReachable(client, &corev1.Endpoints{}, "8443") {
		t.Errorf("expected not reachable when there are no endpoint addresses")
	}
	if allEndpointsReachable(client, endpointsWithAddresses(), "8443") {
		t.Errorf("expected not reachable when the subset lists no addresses")
	}
}
