package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape status %d", rec.Code)
	}
	return rec.Body.String()
}

func assertSeries(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, "\n"+w+"\n") {
			t.Errorf("missing series %q", w)
		}
	}
}

func TestMiddlewareLabelsRequestsByRouteTemplate(t *testing.T) {
	m := New()
	inner := http.NewServeMux()
	inner.HandleFunc("POST /api/run", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "busy", http.StatusServiceUnavailable)
	})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/items/{id}", func(http.ResponseWriter, *http.Request) {})
	mux.Handle("/api/run", inner)
	h := m.Middleware(mux)

	for _, r := range []struct{ method, path string }{
		{"GET", "/api/items/1"},
		{"GET", "/api/items/2"},
		{"POST", "/api/run"},
		{"GET", "/no/such/path"},
		{"BREW", "/api/items/3"},
	} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(r.method, r.path, nil))
	}

	assertSeries(t, scrape(t, m),
		`http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="200",http_route="/api/items/{id}"} 2`,
		// The pattern of the nested mux, without its method.
		`http_server_request_duration_seconds_count{http_request_method="POST",http_response_status_code="503",http_route="/api/run"} 1`,
		// No pattern matched: no route rather than the raw path.
		`http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="404",http_route=""} 1`,
		`http_server_request_duration_seconds_count{http_request_method="_OTHER",http_response_status_code="405",http_route=""} 1`,
		`http_server_active_requests{http_request_method="GET"} 0`,
	)
}

func TestMiddlewareCountsActiveRequests(t *testing.T) {
	m := New()
	entered, release := make(chan struct{}), make(chan struct{})
	h := m.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
		close(done)
	}()
	<-entered
	assertSeries(t, scrape(t, m), `http_server_active_requests{http_request_method="POST"} 1`)
	close(release)
	<-done
	assertSeries(t, scrape(t, m), `http_server_active_requests{http_request_method="POST"} 0`)
}

func TestMiddlewareKeepsResponsesFlushable(t *testing.T) {
	h := New().Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if !rec.Flushed {
		t.Error("response was not flushed")
	}
}

func TestStartTraceRecordsRunsByOutcome(t *testing.T) {
	m := New()
	finish := m.StartTrace()
	assertSeries(t, scrape(t, m), `traceroute_active_runs 1`)
	finish(OutcomeReached)
	m.StartTrace()(OutcomeCanceled)

	assertSeries(t, scrape(t, m),
		`traceroute_active_runs 0`,
		`traceroute_runs_total{outcome="reached"} 1`,
		`traceroute_runs_total{outcome="canceled"} 1`,
		`traceroute_runs_total{outcome="unreached"} 0`,
		`traceroute_runs_total{outcome="error"} 0`,
		`traceroute_run_duration_seconds_count{outcome="reached"} 1`,
	)
}

func TestNilMetricsRecordsNothing(t *testing.T) {
	var m *Metrics
	m.StartTrace()(OutcomeReached)
}

func TestProcessAndRuntimeMetrics(t *testing.T) {
	body := scrape(t, New())
	for _, name := range []string{"process_start_time_seconds", "go_goroutines", "go_memstats_alloc_bytes"} {
		if !strings.Contains(body, "\n"+name+" ") {
			t.Errorf("missing %s", name)
		}
	}
}
