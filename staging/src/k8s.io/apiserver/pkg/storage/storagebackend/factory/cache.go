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

package factory

import (
	"sort"
	"strings"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/apiserver/pkg/storage/storagebackend"
)

var etcdCache = &etcdClientCache{clients: make(map[transportCacheKey]*clientv3.Client)}

type etcdClientCache struct {
	mu      sync.Mutex
	clients map[transportCacheKey]*clientv3.Client
}

func (e *etcdClientCache) get(c storagebackend.TransportConfig) (*clientv3.Client, error) {
	key, canCache := tlsConfigKey(c)
	if !canCache {
		return newETCD3Client(c)
	}
	etcdCache.mu.Lock()
	defer etcdCache.mu.Unlock()
	if client, ok := e.clients[key]; ok {
		return client, nil
	}
	client, err := newETCD3Client(c)
	if err != nil {
		return nil, err
	}
	e.clients[key] = client
	return client, nil
}

type transportCacheKey struct {
	serverList    string
	keyFile       string
	certFile      string
	trustedCAFile string
}

// tlsConfigKey returns a unique key for tls.Config objects returned from TLSConfigFor
func tlsConfigKey(c storagebackend.TransportConfig) (transportCacheKey, bool) {
	k := transportCacheKey{
		keyFile:       c.KeyFile,
		certFile:      c.CertFile,
		trustedCAFile: c.TrustedCAFile,
	}

	// functions can't be compared
	if c.EgressLookup != nil || c.TracerProvider != nil {
		return k, false
	}

	// sort and join server list
	servers := make([]string, len(c.ServerList))
	copy(servers, c.ServerList)
	sort.Strings(servers)
	k.serverList = strings.Join(servers, ",")
	return k, true
}
