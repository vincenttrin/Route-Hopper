// Package metrics exposes the server's Prometheus metrics: HTTP requests named
// after the OpenTelemetry HTTP semantic conventions, so one dashboard reads them
// for every service, traceroute runs, and the Go runtime and process. The
// service label is added by whatever scrapes them (see alloy/config.alloy).
package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Outcome is how a traceroute run ended.
type Outcome string

const (
	// OutcomeReached is a run the destination itself answered.
	OutcomeReached Outcome = "reached"
	// OutcomeUnreached is a run that finished, or hit its time limit, without
	// an answer from the destination. Many hosts drop probes, so it is common.
	OutcomeUnreached Outcome = "unreached"
	// OutcomeError is a run that failed, such as traceroute not being installed.
	OutcomeError Outcome = "error"
	// OutcomeCanceled is a run the client left before it finished.
	OutcomeCanceled Outcome = "canceled"
)

// Metrics records the server's metrics in a registry of its own.
type Metrics struct {
	registry        *prometheus.Registry
	requestDuration *prometheus.HistogramVec
	activeRequests  *prometheus.GaugeVec
	traceRuns       *prometheus.CounterVec
	traceDuration   *prometheus.HistogramVec
	activeTraces    prometheus.Gauge
}

// New returns Metrics with every series registered.
func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_server_request_duration_seconds",
			Help: "Duration of HTTP server requests.",
			// The OpenTelemetry defaults, then room for traces, which take up to
			// the 60 second UDP limit plus the 15 second ICMP retry.
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10, 30, 60, 90},
		}, []string{"http_request_method", "http_route", "http_response_status_code"}),
		activeRequests: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "http_server_active_requests",
			Help: "Number of HTTP server requests in flight.",
		}, []string{"http_request_method"}),
		traceRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "traceroute_runs_total",
			Help: "Traceroute runs finished, by how they ended.",
		}, []string{"outcome"}),
		traceDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "traceroute_run_duration_seconds",
			Help:    "Duration of traceroute runs, including the ICMP retry, by how they ended.",
			Buckets: []float64{1, 2.5, 5, 10, 15, 20, 30, 45, 60, 75, 90},
		}, []string{"outcome"}),
		activeTraces: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "traceroute_active_runs",
			Help: "Number of traceroute runs in flight.",
		}),
	}
	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requestDuration, m.activeRequests, m.traceRuns, m.traceDuration, m.activeTraces,
	)
	// Start every outcome at zero so rates and ratios exist before the first run.
	for _, o := range []Outcome{OutcomeReached, OutcomeUnreached, OutcomeError, OutcomeCanceled} {
		m.traceRuns.WithLabelValues(string(o))
	}
	return m
}

// Handler serves the metrics in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})
}

// Middleware records the duration and count of every request next serves.
// The route is the pattern of the http.ServeMux that matched the request
// (next or one nested in it), without its method and host, so it is the
// route template and never the raw path. A request no pattern matched has no
// route.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := normalizeMethod(r.Method)
		active := m.activeRequests.WithLabelValues(method)
		active.Inc()
		defer active.Dec()

		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		m.requestDuration.WithLabelValues(method, route(r.Pattern), strconv.Itoa(sw.status)).
			Observe(time.Since(start).Seconds())
	})
}

// StartTrace records the start of a traceroute run. The returned function
// records its end and must be called exactly once. A nil Metrics records
// nothing.
func (m *Metrics) StartTrace() func(Outcome) {
	if m == nil {
		return func(Outcome) {}
	}
	start := time.Now()
	m.activeTraces.Inc()
	return func(o Outcome) {
		m.activeTraces.Dec()
		m.traceRuns.WithLabelValues(string(o)).Inc()
		m.traceDuration.WithLabelValues(string(o)).Observe(time.Since(start).Seconds())
	}
}

// route strips the method and host from a ServeMux pattern such as
// "POST /api/trace".
func route(pattern string) string {
	if i := strings.Index(pattern, "/"); i >= 0 {
		return pattern[i:]
	}
	return ""
}

// normalizeMethod keeps the label to a fixed set: clients choose the method,
// so any other value is "_OTHER", as the semantic conventions specify.
func normalizeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	}
	return "_OTHER"
}

// statusWriter remembers the status code of a response. Unwrap lets
// http.ResponseController reach the underlying writer, which the streaming
// endpoint flushes.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status, w.wroteHeader = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
