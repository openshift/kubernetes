/*
Copyright 2021 The Kubernetes Authors.

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

package rest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestParseAltSvcHeader(t *testing.T) {

	tests := []struct {
		name    string
		header  string
		want    AltSvcHeader
		wantErr bool
	}{
		{
			name:   "single alternate service without host",
			header: `h2=":443"`,
			want: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `:443`,
					},
				},
			},
		},
		{
			name:   "single alternate service dns name",
			header: `h2="www.domain.com:443"`,
			want: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `www.domain.com:443`,
					},
				},
			},
		},
		{
			name:   "single alternate service IPv4 address",
			header: `h2="192.168.1.2:443"`,
			want: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `192.168.1.2:443`,
					},
				},
			},
		},
		{
			name:   "single alternate service IPv6 address",
			header: `h2="[2001:db8:aaaa:bbbb::]:443"`,
			want: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `[2001:db8:aaaa:bbbb::]:443`,
					},
				},
			},
		},
		{
			name:   "single alternate service and max age",
			header: `h2=":443"; ma=100`,
			want: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `:443`,
						ma:           100,
					},
				},
			},
		},
		{
			name:   "single alternate service and persist",
			header: `h2=":443"; persist=1`,
			want: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `:443`,
						persist:      true,
					},
				},
			},
		},
		{
			name:   "multiple hosts with options",
			header: `h2="alt.example.com:443"; ma=2592000,  h2="test.com:443"; persist=1`,
			want: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `alt.example.com:443`,
						ma:           2592000,
					},
					{
						protocolId:   h2,
						altAuthority: `test.com:443`,
						persist:      true,
					},
				},
			},
		},
		{
			name:   "ignore unknown options",
			header: `h2="alt.example.com:443"; ma=2592000; v=7,  h2="test.com:443"; persist=1`,
			want: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `alt.example.com:443`,
						ma:           2592000,
					},
					{
						protocolId:   h2,
						altAuthority: `test.com:443`,
						persist:      true,
					},
				},
			},
		},
		{
			name:    "single alternate invalid service IPv6 address (not enclosed square brackets)",
			header:  `h2="2001:db8:aaaa:bbbb:::443"`,
			wantErr: true,
		},
		{
			name:    "missing port",
			header:  `h2="www.domain.com"`,
			wantErr: true,
		},
		{
			name:    "unsupported alpn protocol",
			header:  `h3=":443"`,
			wantErr: true,
		},
		{
			name:    "missing alternative",
			header:  `ma=2500`,
			wantErr: true,
		},
		{
			name:    "invalid parameters",
			header:  `test=test2`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotResult, err := ParseAltSvcHeader(tt.header)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAltSvcHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResult, tt.want) {
				t.Errorf("ParseAltSvcHeader() = %v, want %v", gotResult, tt.want)
			}
		})
	}
}

func TestNewAltSvcHeader(t *testing.T) {

	tests := []struct {
		name    string
		h       AltSvcHeader
		want    string
		wantErr bool
	}{
		{
			name: "clear",
			h: AltSvcHeader{
				clear: true,
			},
			want: "clear",
		},
		{
			name: "single host",
			h: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `www.domain.com:443`,
					},
				},
			},
			want: `h2="www.domain.com:443"`,
		},
		{
			name: "single IPv6",
			h: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `[::1]:443`,
					},
				},
			},
			want: `h2="[::1]:443"`,
		},
		{
			name: "multiple host",
			h: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `alt.example.com:443`,
						ma:           2592000,
					},
					{
						protocolId:   h2,
						altAuthority: `test.com:443`,
						persist:      true,
					},
				},
			},
			want: `h2="alt.example.com:443"; ma=2592000, h2="test.com:443"; persist=1`,
		},
		{
			name: "invalid combination clear and alternative services",
			h: AltSvcHeader{
				clear: true,
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `[::1]:443`,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid host",
			h: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   h2,
						altAuthority: `::1:443`,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "unsopported protocol",
			h: AltSvcHeader{
				altValue: []Alternative{
					{
						protocolId:   100,
						altAuthority: `[::1]:443`,
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAltSvcHeader(tt.h)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAltSvcHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NewAltSvcHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlternateServicesHandler(t *testing.T) {

	altSvcHandler := &AlternateServices{}
	tests := []struct {
		name         string
		altSvc       string
		host         string
		expectedURLs []string
	}{
		{
			name:         "only one host",
			altSvc:       fmt.Sprintf(`h2=":443"; ma=2592000`),
			host:         "localhost",
			expectedURLs: []string{"https://localhost:443"},
		},
		{
			name:         "two hosts",
			altSvc:       fmt.Sprintf(`h2=":443"; ma=2592000, h2="www.domain.com:8443"`),
			host:         "localhost",
			expectedURLs: []string{"https://localhost:443", "https://www.domain.com:8443"},
		},
		{
			name:         "two hosts and one IPv6 server",
			altSvc:       fmt.Sprintf(`h2=":443"; ma=2592000, h2="www.domain.com:8443", h2="[::1]:889`),
			host:         "localhost",
			expectedURLs: []string{"https://localhost:443", "https://www.domain.com:8443", "https://[::1]:889"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// test
			altSvcHandler.HandleAltSvcHeader(tt.altSvc, tt.host)
			// verify that new request can go to other hosts
			urls := sets.NewString(tt.expectedURLs...)
			if len(urls) != altSvcHandler.cache.Len() {
				t.Fatalf("expected cache with %d entries, got %d", len(urls), altSvcHandler.cache.Len())
			}
			for _, k := range altSvcHandler.cache.List() {
				if !urls.Has(k) {
					t.Fatalf("url %s doesn't found, got %v", k, urls.List())
				}
			}
			// clear alt-svc cache
			altSvcHandler.HandleAltSvcHeader("clear", "")
			if altSvcHandler.cache.Len() > 0 {
				t.Fatalf("expected empty cache, got %v", altSvcHandler.cache.List())
			}

		})
	}
}

func TestClientWithAlternateServices(t *testing.T) {
	var altSvcHeader string
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Alt-Svc", altSvcHeader)
		fmt.Fprintf(w, "Hello, %s", r.Proto)
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	altTs := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello alternate, %s", r.Proto)
	}))
	altTs.EnableHTTP2 = true
	altTs.StartTLS()
	altURL, err := url.ParseRequestURI(altTs.URL)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	altSvcHeader = fmt.Sprintf(`h2="%s"`, altURL.Host)
	defer altTs.Close()

	transport, ok := ts.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("failed to assert *http.Transport")
	}

	altSvcHandler := &AlternateServices{
		client: *ts.Client(),
	}
	config := &Config{
		Host:          ts.URL,
		Transport:     utilnet.SetTransportDefaults(transport),
		AltSvcHandler: altSvcHandler,
		// These fields are required to create a REST client.
		ContentConfig: ContentConfig{
			GroupVersion:         &schema.GroupVersion{},
			NegotiatedSerializer: &serializer.CodecFactory{},
		},
	}
	client, err := RESTClientFor(config)
	if err != nil {
		t.Fatalf("failed to create REST client: %v", err)
	}
	data, err := client.Get().AbsPath("/").DoRaw(context.TODO())
	if err != nil {
		t.Fatalf("unexpected err: %s: %v", data, err)
	}
	if string(data) != "Hello, HTTP/2.0" {
		t.Fatalf("unexpected response: %s", data)
	}
	time.Sleep(1 * time.Second)
	data, err = client.Get().AbsPath("/").DoRaw(context.TODO())
	if err != nil {
		t.Fatalf("unexpected err: %s: %v", data, err)
	}
	if string(data) != "Hello alternate, HTTP/2.0" {
		t.Fatalf("unexpected response: %s", data)
	}
}
