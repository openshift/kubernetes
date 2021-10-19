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

package filters

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	fsMu        sync.Mutex // guards fsCache and fsTimestamp
	fsCache     string
	fsTimestamp time.Time
)

const cacheTTL = 60 * time.Second

// WithAternativeServices sets the Alt-Svc header based on the available api servers
// See RFC7838
func WithAternativeServices(handler http.Handler, config *rest.Config) http.Handler {
	clientset := kubernetes.NewForConfigOrDie(config)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hdr := altSvc(clientset)

		w.Header().Set("Alt-Svc", hdr)
		handler.ServeHTTP(w, req)
	})
}

func altSvc(clientset kubernetes.Interface) string {
	now := time.Now()

	fsMu.Lock()
	defer fsMu.Unlock()
	// check the cache first
	if !fsTimestamp.IsZero() && now.Sub(fsTimestamp) < cacheTTL {
		return fsCache
	}
	// update the cache without blocking
	go updateCachedAlternativeServices(clientset)
	return ""
}

func updateCachedAlternativeServices(clientset kubernetes.Interface) {
	fsMu.Lock()
	fsTimestamp = time.Now()
	fsMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	// Update the alt-svc header with the current apiserver endpoints
	endpoint, err := clientset.CoreV1().Endpoints("default").Get(ctx, "kubernetes", metav1.GetOptions{})
	if err != nil {
		return
	}

	ips := getEndpointIPs(endpoint)
	if len(ips) < 2 {
		return
	}
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Alt-Svc
	// TODO ma equal to the lease time
	// ma=<max-age>
	// persist=1 not cleared on network configuration change
	options := "ma=600"
	// TODO order implies preference
	var hdr string
	for i, a := range ips {
		if i != 0 {
			hdr += ", "
		}
		hdr += fmt.Sprintf(`h2="%s"; %s`, net.JoinHostPort(a, "6443"), options)
	}
	fsMu.Lock()
	fsCache = hdr
	fsMu.Unlock()
}

// return the unique endpoint IPs
func getEndpointIPs(endpoints *corev1.Endpoints) []string {
	endpointMap := make(map[string]bool)
	ips := make([]string, 0)
	for _, subset := range endpoints.Subsets {
		for _, address := range subset.Addresses {
			if _, ok := endpointMap[address.IP]; !ok {
				endpointMap[address.IP] = true
				ips = append(ips, address.IP)
			}
		}
	}
	return ips
}
