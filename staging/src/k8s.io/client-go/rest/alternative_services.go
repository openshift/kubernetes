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
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	netutils "k8s.io/utils/net"
)

// AltSvcHandler is an interface for handling AltSvc headers
type AltSvcHandler interface {
	// HandleAltSvcHeader is called when an Alt-Svc header is received together with the host of the server
	HandleAltSvcHeader(altSvcString, host string)
	BaseURL() url.URL
}

// NoAltSvcs is an implementation of AltSvcHandler that suppresses warnings.
type NoAltSvcs struct{}

var _ AltSvcHandler = &NoAltSvcs{}

func (n *NoAltSvcs) HandleAltSvcHeader(altSvcString, host string) {}
func (n *NoAltSvcs) BaseURL() url.URL {
	return url.URL{}
}

type cacheEntry struct {
	uri       url.URL
	timestamp time.Time
	ma        time.Duration // max age
	//
	ready   bool
	local   bool
	latency time.Duration
}

type cache struct {
	sync.Mutex
	storage map[string]cacheEntry
}

func (c *cache) NewCache() *cache {
	return &cache{
		storage: map[string]cacheEntry{},
	}
}

func (c *cache) Reset() {
	c.Lock()
	defer c.Unlock()
	c.storage = map[string]cacheEntry{}
}

func (c *cache) Len() int {
	c.Lock()
	defer c.Unlock()
	return len(c.storage)
}

func (c *cache) Get(key string) (cacheEntry, bool) {
	c.Lock()
	defer c.Unlock()
	v, ok := c.storage[key]
	if !ok {
		return cacheEntry{}, false
	}
	clone := v
	return clone, true
}

func (c *cache) Add(key url.URL, entry Alternative) {
	c.Lock()
	defer c.Unlock()
	item := cacheEntry{}
	_, ok := c.storage[key.String()]
	if ok {
		return
	}

	now := time.Now()
	item.uri = key
	item.timestamp = now
	if entry.ma > 0 {
		item.ma = time.Duration(entry.ma) * time.Second
	}
	if c.storage == nil {
		c.storage = map[string]cacheEntry{}
	}
	if isLocal(key) {
		item.local = true
	}
	c.storage[key.String()] = item

}

func (c *cache) List() []string {
	c.Lock()
	defer c.Unlock()
	entries := make([]string, len(c.storage))
	i := 0
	for k := range c.storage {
		entries[i] = k
		i++
	}
	return entries
}

func (c *cache) GetLocal() url.URL {
	c.Lock()
	defer c.Unlock()

	for _, v := range c.storage {
		if v.local && v.ready {
			return v.uri
		}
	}
	return url.URL{}
}

func (c *cache) GetLowLatency() url.URL {
	c.Lock()
	defer c.Unlock()

	if len(c.storage) == 0 {
		return url.URL{}
	}

	type kv struct {
		k string
		v time.Duration
	}
	var latencies []kv
	for k, v := range c.storage {
		klog.Infof("DEBUG getowlatency entries %s %v", v.uri.String(), v.ready)

		if v.ready {
			latencies = append(latencies, kv{k: k, v: v.latency})
		}
	}
	klog.Infof("DEBUG getowlatency %v", latencies)
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i].v < latencies[j].v
	})

	if len(latencies) > 0 {
		return c.storage[latencies[0].k].uri
	}
	return url.URL{}
}

func (c *cache) Refresh(client *http.Client, host string) {
	c.Lock()
	defer c.Unlock()
	t := time.Now()

	for k, v := range c.storage {
		// drop if entry aged
		if v.ma != 0 && t.Sub(v.timestamp) > v.ma {
			klog.Infof("DEBUG Dropping %s from the cache", k)
			delete(c.storage, k)
			continue
		}
		// check if url is ready (cached)
		if v.ready {
			//continue
		}
		// if not ready query the url directly
		v.ready, v.latency = getReadyz(v.uri, client, host)
		klog.Infof("DEBUG refresh %s ready %v latency %v", k, v.ready, v.latency)
		c.storage[k] = v

	}

}

type AlternateServices struct {
	client *http.Client
	cache  cache // keyed by url
	host   string
}

var _ AltSvcHandler = &AlternateServices{}

func (a *AlternateServices) HandleAltSvcHeader(altSvcString, host string) {
	if len(altSvcString) == 0 {
		return
	}

	altSvc, err := ParseAltSvcHeader(altSvcString)
	if err != nil {
		klog.Infof("Error parsing Alt-Svc header %s: %v", altSvcString, err)
		return
	}

	// clear cache
	if altSvc.clear {
		a.cache.Reset()
		return
	}

	for _, alt := range altSvc.altValue {
		// build url
		h, port, err := net.SplitHostPort(alt.altAuthority)
		if err != nil {
			continue
		}

		// use the remote host if the host field is empty
		if h == "" {
			h = host
		}
		// only allow https to avoid possible security issues with Alt-Svc
		uri, err := url.ParseRequestURI("https://" + net.JoinHostPort(h, port))
		if err != nil {
			continue
		}
		a.cache.Add(*uri, alt)
		klog.Infof("DEBUG adding %s to the cache", uri.String())
	}

}

func (a *AlternateServices) BaseURL() url.URL {
	// get one valid URL from the cache
	// maps doesn't guarantee order
	// TODO we can implement the algorithm we want
	// sticky
	// round robin
	// ...

	a.cache.Refresh(a.client, a.host)
	if u := a.cache.GetLocal(); u != (url.URL{}) {
		return u
	}

	u := a.cache.GetLowLatency()
	return u
}

func getReadyz(u url.URL, c *http.Client, host string) (bool, time.Duration) {
	if c == nil {
		c = http.DefaultClient
	}
	now := time.Now()
	var elapsed time.Duration
	u.Path = "/readyz"
	// not interested on api servers with large latencies ("better the devil you know")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			elapsed = time.Since(now)
		},
	}
	ctx = httptrace.WithClientTrace(ctx, trace)
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return false, elapsed
	}
	req.Host = host

	result, err := c.Do(req)
	if err != nil {
		return false, elapsed
	}
	defer result.Body.Close()
	if result.StatusCode == 200 {
		return true, elapsed
	}
	return false, elapsed
}

func isLocal(uri url.URL) bool {
	host := uri.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	localIPs := getLocalAddressSet()
	for _, ip := range ips {
		if localIPs.Has(ip) {
			return true
		}
	}
	return false
}

// getLocalAddrs returns a set with all network addresses on the local system
func getLocalAddressSet() netutils.IPSet {
	localAddrs := netutils.IPSet{}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		klog.InfoS("Error getting local addresses", "error", err)
		return localAddrs
	}

	for _, addr := range addrs {
		ip, _, err := netutils.ParseCIDRSloppy(addr.String())
		if err != nil {
			klog.InfoS("Error getting local addresses", "address", addr.String(), "error", err)
			continue
		}
		localAddrs.Insert(ip)
	}
	return localAddrs
}

// Alt-Svc
// RFC 7838
//    Alt-Svc       = clear / 1#alt-value
// clear         = %s"clear"; "clear", case-sensitive
// alt-value     = alternative *( OWS ";" OWS parameter )
// alternative   = protocol-id "=" alt-authority
// protocol-id   = token ; percent-encoded ALPN protocol name
// alt-authority = quoted-string ; containing [ uri-host ] ":" port
// parameter     = token "=" ( token / quoted-string )
// Caching parameters:
// ma 			 = delta-seconds
// persist       = not clear on network changes
type AltSvcHeader struct {
	clear    bool
	altValue []Alternative
}

type Alternative struct {
	protocolId   ALPNProtocolType
	altAuthority string
	ma           int
	persist      bool
}

type ALPNProtocolType int

const (
	unsupported ALPNProtocolType = iota
	h2
)

func NewAltSvcHeader(h AltSvcHeader) (string, error) {
	// The field value consists either of a list of values, each of which
	// indicates one alternative service, or the keyword "clear".
	// https://datatracker.ietf.org/doc/html/rfc7838#section-3
	if h.clear {
		if len(h.altValue) > 0 {
			return "", fmt.Errorf("option clear and alt-values are exclusive")
		}
		return "clear", nil
	}

	var v string
	var errors []error
	for _, a := range h.altValue {
		if !supportedALPNProto(a.protocolId) || len(a.altAuthority) == 0 {
			errors = append(errors, fmt.Errorf("invalid alternative service %v", a))
			continue
		}

		// validate alt-authority
		_, _, err := net.SplitHostPort(a.altAuthority)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		// comma separated list
		if len(v) > 0 {
			v += fmt.Sprintf(", ")
		}

		// TODO support more ALPN protocols
		v += fmt.Sprintf(`h2="%s"`, a.altAuthority)

		// semi-colon separated list of additional parameters
		if a.ma != 0 {
			v += fmt.Sprintf(`; ma=%d`, a.ma)
		}

		if a.persist {
			v += fmt.Sprintf(`; persist=1`)
		}

	}
	if len(errors) > 0 {
		return "", utilerrors.NewAggregate(errors)
	}

	if len(v) == 0 {
		return "", fmt.Errorf("no alternative services provided")
	}
	return v, nil
}

func ParseAltSvcHeader(header string) (result AltSvcHeader, err error) {
	// tolerate whitespaces
	header = strings.TrimSpace(header)

	if header == "clear" {
		return AltSvcHeader{clear: true}, nil
	}

	var errors []error
	// comma separated list of alternative
	alternatives := strings.Split(header, ",")
	for _, a := range alternatives {
		altValue := Alternative{}
		// semi colon separated list of options per alternative service
		alternative := strings.Split(a, ";")
		if len(alternative) == 0 {
			errors = append(errors, fmt.Errorf("no alternative service present"))
			continue
		}
		// Process first entry
		// alternative   = protocol-id "=" alt-authority
		h := strings.Split(strings.TrimSpace(alternative[0]), "=")
		if len(h) != 2 {
			errors = append(errors, fmt.Errorf("error parsing alternative service %s", alternative))
			continue
		}
		proto := convertToALPNProto(h[0])
		if !supportedALPNProto(proto) {
			errors = append(errors, fmt.Errorf("unsupported protocol %s", h[0]))
			continue
		}

		altValue.protocolId = proto
		// validate alt-authority (it is a quoted string)
		authority := strings.Trim(h[1], "\"")
		_, _, err := net.SplitHostPort(authority)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		altValue.altAuthority = authority
		// process rest of the options
		for i, option := range alternative {
			// skip already processed entry
			if i == 0 {
				continue
			}
			option := strings.TrimSpace(option)
			h := strings.Split(option, "=")
			if len(h) != 2 {
				errors = append(errors, fmt.Errorf("wrong parameter %s", option))
				continue
			}
			switch h[0] {
			case "ma":
				maxAge, err := strconv.Atoi(h[1])
				if err != nil {
					errors = append(errors, fmt.Errorf("wrong ma option %w", err))
					continue
				}
				altValue.ma = maxAge
			case "persist":
				persist, err := strconv.Atoi(h[1])
				if err != nil {
					errors = append(errors, fmt.Errorf("wrong persist option %w", err))
					continue
				}
				// ignore values different than 1
				altValue.persist = persist == 1
			}

		}
		result.altValue = append(result.altValue, altValue)
	}
	if len(errors) > 0 {
		return result, utilerrors.NewAggregate(errors)
	}
	return result, nil
}

func convertToALPNProto(proto string) ALPNProtocolType {
	switch proto {
	case "h2":
		return h2
	}
	return unsupported
}

func supportedALPNProto(alpn ALPNProtocolType) bool {
	switch alpn {
	case h2:
		return true
	}
	return false
}
