package resolver

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v1 "k8s.io/api/core/v1"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/resolver/dns"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	netutils "k8s.io/utils/net"
)

// atomicBool uses load/store operations on an int32 to simulate an atomic boolean.
type atomicBool struct {
	v int32
}

// set sets the int32 to the given boolean.
func (a *atomicBool) set(value bool) {
	if value {
		atomic.StoreInt32(&a.v, 1)
		return
	}
	atomic.StoreInt32(&a.v, 0)
}

// get returns true if the int32 == 1
func (a *atomicBool) get() bool {
	return atomic.LoadInt32(&a.v) == 1
}

// ResolverOption defines the functional option type for resolver
type ResolverOption func(*resolver) *resolver

func WithTimeout(timeout time.Duration) func(*resolver) {
	return func(r *resolver) {
		r.timeout = timeout
	}
}

// resolver to store API server IP addresses
type resolver struct {
	mu        sync.Mutex
	refresh   *atomicBool
	cache     []net.IP  // API server IP addresses
	timestamp time.Time // store the time the resolved is contacted once the cache has already been populated
	host      string    // API server configured hostname
	port      string    // API server configured port

	// time after the cache entries are considered stale and pruned
	timeout time.Duration
	// RESTclient configuration
	client *rest.RESTClient
}

// NewResolver returns an in memory net.Resolver that resolves the API server
// Host name with the addresses obtained from the API server published Endpoints
// resources.
// The resolver polls periodically the API server to refresh the local cache.
// The resolver will fall back to the default golang resolver if:
// - Is not able to obtain the API server Endpoints.
// - The configured API server host name is not resolvable via DNS, per example,
//   is not an IP address or is resolved via /etc/hosts.
// - The configured API server URL has a different port
//   than the one used in the Endpoints.
func NewResolver(ctx context.Context, c *rest.Config, options ...ResolverOption) (*net.Resolver, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}

	host, port, err := getHostPort(config.Host)
	if err != nil {
		return nil, err
	}

	if netutils.ParseIPSloppy(host) != nil {
		return net.DefaultResolver, fmt.Errorf("APIServerResolver only works for domain names")
	}

	// defaulting
	r := &resolver{
		host:    host,
		port:    port,
		timeout: 100 * time.Second,
		refresh: &atomicBool{1},
	}

	// options
	for _, o := range options {
		o(r)
	}

	f := &dns.MemResolver{
		LookupIP: func(ctx context.Context, network, host string) ([]net.IP, error) {
			return r.lookupIP(ctx, network, host)
		},
	}

	resolver := dns.NewMemoryResolver(f)
	config.Resolver = resolver

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	r.client = client
	// Initialize cache and close idle connections so next connections goes
	// directly to one of the API server IPs, this is useful for cases
	// that use a load balancer for bootstrapping
	r.refreshCache(ctx)
	utilnet.CloseIdleConnectionsFor(r.client.Client.Transport)
	return resolver, nil
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/api"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

func (r *resolver) lookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	// Use the default resolver if is not trying to resolve the configured API server hostname
	if !strings.HasPrefix(host, r.host) {
		klog.V(7).Infof("Resolver Trace: use default resolver for host %s, different than API server hostname %s", host, r.host)
		return net.DefaultResolver.LookupIP(ctx, network, host)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Use the default resolver if the cache is empty and try to renew the cache.
	if len(r.cache) == 0 {
		klog.V(7).Infof("Resolver Trace: use default resolver for host %s, cache is empty", host)
		addrs, err := net.DefaultResolver.LookupIP(ctx, network, host)
		if err != nil {
			return addrs, err
		}
		// don't try to renew the cache until we know it's possible
		if len(addrs) > 0 {
			klog.V(7).Infof("Resolver Trace: refreshing cache for first time for host %s", host)
			go r.refreshCache(ctx)
		}
		return addrs, nil
	}

	// Check if is the first time we query the cache and record the time, it is a fresh cache
	// so don't try to refresh it again, if something goes wrong next time will be refreshed.
	// If is not it the first time, it means that previous connection didn't work out
	// so we return the IPs in a different order to reach a different API server.
	if r.timestamp.IsZero() {
		r.timestamp = time.Now()
	} else {
		rand.Shuffle(len(r.cache), func(i, j int) {
			r.cache[i], r.cache[j] = r.cache[j], r.cache[i]
		})
		if r.refresh.get() {
			klog.V(7).Infof("Resolver Trace: refreshing cache for host %s", host)
			go r.refreshCache(ctx)
		}
	}

	// Return the IP addresses from the cache.
	ips := make([]net.IP, len(r.cache))
	copy(ips, r.cache)
	klog.V(7).Infof("Resolver Trace: host %s resolves to %v", host, ips)
	return ips, nil
}

// refreshCache refresh the cache with the API server IP addresses.
// If it can not refresh the IPs because of network errors during
// a predefined time, it falls back to the default resolver.
// If it can not refresh the IPs because of other type of errors, per
// example, it is able to connect to the API server but this is not ready
// or is not able to reply, it shuffles the IPs so it will retry randomly.
// If there are no errors, local IP addresses are returned first, so it
// favors direct connectivity.
func (r *resolver) refreshCache(ctx context.Context) {
	// avoid multiple refresh queries in paralell
	r.refresh.set(false)
	defer r.refresh.set(true)
	// Kubernetes conformance clusters require: The cluster MUST have a service
	// named "kubernetes" on the default namespace referencing the API servers.
	// The "kubernetes.default" service MUST have Endpoints and EndpointSlices
	// pointing to each API server instance.
	// Endpoints managed by API servers are removed if they API server is not ready.
	endpoint := &v1.Endpoints{}
	err := r.client.Get().
		Resource("endpoints").
		Namespace("default").
		Name("kubernetes").
		Do(ctx).
		Into(endpoint)
	// error handling
	if err != nil {
		klog.V(7).Infof("Resolver Trace: error getting apiserver addresses from Endpoints: %v", err)
		// nothing to do here, continue
		if len(r.cache) == 0 {
			return
		}
		stale := false
		if !r.timestamp.IsZero() {
			stale = time.Now().After(r.timestamp.Add(r.timeout))
		}
		// give up if there are errors and we could not renew the entries during the specified timeout
		if stale {
			klog.V(7).Infof("Resolver Trace: falling back to default resolver, too many errors to connect to %s:%s on %v : %v", r.host, r.port, r.cache, err)
			r.mu.Lock()
			r.cache = []net.IP{}
			r.mu.Unlock()
		}
		return
	}
	// Get IPs from the Endpoint.
	ips := []net.IP{}
	for _, ss := range endpoint.Subsets {
		for _, e := range ss.Addresses {
			ips = append(ips, netutils.ParseIPSloppy(e.IP))
		}
		// Unsupported configurations:
		// - API Server with multiple endpoints
		// - Configured URL and Endpoints with different ports
		if len(ss.Ports) != 1 || strconv.Itoa(int(ss.Ports[0].Port)) != r.port {
			r.mu.Lock()
			r.cache = []net.IP{}
			r.timestamp = time.Time{}
			r.mu.Unlock()
			return
		}
	}
	// Do nothing if there are no IPs published.
	if len(ips) == 0 {
		return
	}

	// Update the cache and exit (optimize for one IP)
	if len(ips) == 1 {
		r.mu.Lock()
		r.cache = []net.IP{ips[0]}
		r.timestamp = time.Time{}
		r.mu.Unlock()
		return
	}

	// Shuffle the ips so different clients don't end in the same API server.
	rand.Shuffle(len(ips), func(i, j int) {
		ips[i], ips[j] = ips[j], ips[i]
	})
	// Favor local, returning it first because dialParallel races two copies of
	// dialSerial, giving the first a head start. It returns the first
	// established connection and closes the others.
	localAddresses := getLocalAddressSet()
	for _, ip := range ips {
		if localAddresses.Has(ip) {
			moveToFront(ip, ips)
			break
		}
	}

	r.mu.Lock()
	r.cache = make([]net.IP, len(ips))
	copy(r.cache, ips)
	r.timestamp = time.Time{}
	r.mu.Unlock()
	return
}

// getHostPort returns the host and port from an URL defaulting http and https ports
func getHostPort(h string) (string, string, error) {
	url, err := url.Parse(h)
	if err != nil {
		return "", "", err
	}

	port := url.Port()
	if port == "" {
		switch url.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", "", fmt.Errorf("Unsupported URL scheme")
		}
	}
	return url.Hostname(), port, nil
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

// https://github.com/golang/go/wiki/SliceTricks#move-to-front-or-prepend-if-not-present-in-place-if-possible
// moveToFront moves needle to the front of haystack, in place if possible.
func moveToFront(needle net.IP, haystack []net.IP) []net.IP {
	if len(haystack) != 0 && haystack[0].Equal(needle) {
		return haystack
	}
	prev := needle
	for i, elem := range haystack {
		switch {
		case i == 0:
			haystack[0] = needle
			prev = elem
		case elem.Equal(needle):
			haystack[i] = prev
			return haystack
		default:
			haystack[i] = prev
			prev = elem
		}
	}
	return append(haystack, prev)
}
