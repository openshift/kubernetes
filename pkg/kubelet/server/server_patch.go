package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/endpoints/responsewriter"
	"k8s.io/klog/v2"
	statsapi "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
	"k8s.io/kubernetes/pkg/kubelet/server/stats"

	"github.com/google/uuid"
)

func WithHTTPLogging(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		logger := newHTTPLogger(w, req)
		defer logger.log()

		w = responsewriter.WrapForHTTP1Or2(logger)
		handler.ServeHTTP(w, req)
	})
}

func newHTTPLogger(w http.ResponseWriter, req *http.Request) *httpLogger {
	auditID := audit.GetAuditIDTruncated(req.Context())
	if len(auditID) == 0 {
		auditID = uuid.New().String()
	}
	return &httpLogger{
		w:          w,
		startedAt:  time.Now(),
		method:     req.Method,
		requestURI: req.RequestURI,
		auditID:    auditID,
		userAgent:  req.UserAgent(),
		srcIP:      req.RemoteAddr,
	}
}

type httpLogger struct {
	w http.ResponseWriter

	method     string
	requestURI string
	auditID    string
	userAgent  string
	srcIP      string

	startedAt      time.Time
	writeLatency   time.Duration
	flushLatency   time.Duration
	writeBytes     int
	statusRecorded bool
	statusCode     int
}

var _ http.ResponseWriter = &httpLogger{}
var _ responsewriter.UserProvidedDecorator = &httpLogger{}

func (l *httpLogger) Unwrap() http.ResponseWriter {
	return l.w
}

// Header implements http.ResponseWriter.
func (l *httpLogger) Header() http.Header {
	return l.w.Header()
}

// Write implements http.ResponseWriter.
func (l *httpLogger) Write(b []byte) (int, error) {
	if !l.statusRecorded {
		l.record(http.StatusOK) // Default if WriteHeader hasn't been called
	}
	now := time.Now()
	var written int
	defer func() {
		l.writeLatency += time.Since(now)
		l.writeBytes += written
	}()
	written, err := l.w.Write(b)
	return written, err
}

func (l *httpLogger) Flush() {
	now := time.Now()
	defer func() {
		l.flushLatency += time.Since(now)
	}()
	l.w.(http.Flusher).Flush()
}

// WriteHeader implements http.ResponseWriter.
func (l *httpLogger) WriteHeader(status int) {
	l.record(status)
	l.w.WriteHeader(status)
}

func (l *httpLogger) record(status int) {
	l.statusCode = status
	l.statusRecorded = true
}

func (l *httpLogger) log() {
	latency := time.Since(l.startedAt)
	kvs := []interface{}{
		"startedAt", l.startedAt,
		"method", l.method,
		"URI", l.requestURI,
		"latency", latency,
		"userAgent", l.userAgent,
		"audit-ID", l.auditID,
		"srcIP", l.srcIP,
		"status", l.statusCode,
		"writeLatency", l.writeLatency,
		"writtenBytes", fmt.Sprintf("%dK", l.writeBytes/1024),
		"flushLatency", l.flushLatency,
	}
	klog.V(1).InfoSDepth(1, "HTTP", kvs...)
}

type SummaryProviderTracker struct {
	stats.ResourceAnalyzer
}

func (t *SummaryProviderTracker) GetCPUAndMemoryStats(ctx context.Context) (*statsapi.Summary, error) {
	now := time.Now()
	defer func() {
		klog.InfoS("GetCPUAndMemoryStats", "latency", time.Since(now))
	}()
	return t.ResourceAnalyzer.GetCPUAndMemoryStats(ctx)
}
