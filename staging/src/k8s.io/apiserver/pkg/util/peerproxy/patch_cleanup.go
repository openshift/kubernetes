package peerproxy

// TransportCloser can be implemented by a peerproxy.Interface to release
// transport references during shutdown, allowing the GC to collect the
// underlying trackedTransport and fire its runtime.AddCleanup callback
// which cancels the cert-rotation goroutines.
type TransportCloser interface {
	CloseTransport()
}

// CloseTransport releases the proxy transport reference so that the
// trackedTransport wrapper can be garbage collected.
func (h *peerProxyHandler) CloseTransport() {
	h.proxyTransport = nil
}
