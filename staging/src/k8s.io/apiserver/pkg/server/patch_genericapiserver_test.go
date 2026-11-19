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
