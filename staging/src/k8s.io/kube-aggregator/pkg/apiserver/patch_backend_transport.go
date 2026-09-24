package apiserver

import (
	"net/http"
	"time"

	"golang.org/x/net/http2"

	"k8s.io/client-go/transport"
	"k8s.io/klog/v2"
)

// UPSTREAM: <carry>: fast http2 health checking for aggregated API backend connections
//
// The aggregator proxies requests to aggregated apiservers over pooled http2
// connections.  When such a connection is silently broken (for instance when it was
// established while the pod network on a freshly rebooted control plane node was
// still converging), the default health check parameters
// (ReadIdleTimeout=30s/PingTimeout=15s, see k8s.io/apimachinery/pkg/util/net) keep
// the dead connection pinned for up to ~45 seconds while every request multiplexed
// onto it fails with "http2: client connection lost".  Observed as 10-15s of
// aggregated API disruption during upgrades (OCPBUGS-100065).
//
// Detect and drop dead backend connections within seconds instead.  The values are
// intentionally aggressive: aggregated apiservers are same-cluster backends with
// sub-second round trips, so a connection that cannot answer a ping for a few
// seconds is broken for practical purposes and re-dialing is cheap.
const (
	aggregatedAPIBackendReadIdleTimeout = 5 * time.Second
	aggregatedAPIBackendPingTimeout     = 5 * time.Second
)

// newAggregatedAPIBackendRoundTripper builds the round tripper used to proxy
// requests to an aggregated apiserver.  It mirrors transport.New for the transport
// construction, but configures aggressive http2 connection health checking so that
// broken backend connections are abandoned within seconds.  On any unexpected
// configuration it falls back to the default transport.New behavior.
func newAggregatedAPIBackendRoundTripper(cfg *transport.Config) (http.RoundTripper, error) {
	tlsConfig, err := transport.TLSConfigFor(cfg)
	if err != nil {
		return nil, err
	}
	if tlsConfig == nil || cfg.Transport != nil {
		// no TLS settings or a custom transport: nothing for us to tune, keep the default behavior
		return transport.New(cfg)
	}

	// mirror the transport constructed by client-go's transport.New/tlsCache.get
	t := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: 25,
		IdleConnTimeout:     90 * time.Second,
	}
	if cfg.DialHolder != nil {
		t.DialContext = cfg.DialHolder.Dial
	}

	t2, err := http2.ConfigureTransports(t)
	if err != nil {
		// should not happen; fall back to the default construction rather than failing the APIService
		klog.Warningf("failed to configure http2 health checking for aggregated API backend transport, falling back to defaults: %v", err)
		return transport.New(cfg)
	}
	t2.ReadIdleTimeout = aggregatedAPIBackendReadIdleTimeout
	t2.PingTimeout = aggregatedAPIBackendPingTimeout

	// apply the same wrappers (user agent, auth, WrapTransport such as the x509
	// metrics wrapper) that transport.New would apply
	return transport.HTTPWrappersForConfig(cfg, t)
}
