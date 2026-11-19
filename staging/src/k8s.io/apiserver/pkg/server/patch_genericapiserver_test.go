package server

import (
	"net/http"
	"testing"
)

func TestIsKnownLoopbackClient(t *testing.T) {
	clients := map[string][]string{
		"op-a": {"x"},
		"op-b": {"y", "z"},
	}
	tests := []struct {
		name    string
		clients map[string][]string
		ua      string
		want    bool
	}{
		{"match", clients, "op-a/v1/x", true},
		{"second suffix", clients, "op-b/v1/z", true},
		{"suffix of another binary", clients, "op-a/v1/y", false},
		{"nil map", nil, "op-a/v1/x", false},
		{"kube-apiserver needs no map entry", nil, "kube-apiserver/v1", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{Header: http.Header{"User-Agent": []string{tc.ua}}}
			if got := isKnownLoopbackClient(req, tc.clients); got != tc.want {
				t.Errorf("user agent %q: got %v, want %v", tc.ua, got, tc.want)
			}
		})
	}
}
