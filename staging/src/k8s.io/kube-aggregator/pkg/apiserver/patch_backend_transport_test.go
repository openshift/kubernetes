package apiserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/transport"
)

func TestNewAggregatedAPIBackendRoundTripperServesRequests(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rt, err := newAggregatedAPIBackendRoundTripper(&transport.Config{
		TLS: transport.TLSConfig{Insecure: true},
	})
	if err != nil {
		t.Fatalf("unexpected error building round tripper: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("unexpected error building request: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected round trip error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNewAggregatedAPIBackendRoundTripperFallsBackWithoutTLS(t *testing.T) {
	// no TLS settings: must fall back to transport.New without error
	rt, err := newAggregatedAPIBackendRoundTripper(&transport.Config{})
	if err != nil {
		t.Fatalf("unexpected error building fallback round tripper: %v", err)
	}
	if rt == nil {
		t.Fatalf("expected a round tripper")
	}
}

func TestNewAggregatedAPIBackendRoundTripperAppliesWrappers(t *testing.T) {
	wrapped := false
	cfg := &transport.Config{
		TLS: transport.TLSConfig{Insecure: true},
	}
	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		wrapped = true
		return rt
	})
	if _, err := newAggregatedAPIBackendRoundTripper(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wrapped {
		t.Errorf("expected the config WrapTransport to be applied")
	}
}
