package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vincenttrin/Route-Hopper/backend/geo"
	"github.com/vincenttrin/Route-Hopper/backend/metrics"
	"github.com/vincenttrin/Route-Hopper/backend/trace"
)

func TestTracesAreCountedByOutcome(t *testing.T) {
	m := metrics.New()
	newAPI := func(tracer *fakeTracer) *API {
		api := NewAPI(tracer, fakeResolver{}, geo.Nop{}, 1)
		api.Metrics = m
		return api
	}
	reached := []trace.Hop{{Number: 1, IP: addr("192.168.1.1")}, {Number: 2, IP: addr("203.0.113.53")}}
	unreached := []trace.Hop{{Number: 1, IP: addr("192.168.1.1")}, {Number: 2}}
	body := `{"endpoint":"203.0.113.53"}`

	post(newAPI(&fakeTracer{hops: reached}), body)
	postStream(newAPI(&fakeTracer{hops: reached}), body)
	post(newAPI(&fakeTracer{hops: unreached}), body)
	postStream(newAPI(&fakeTracer{err: errors.New("boom")}), body)
	// The client leaves while the tracer runs.
	ctx, cancel := context.WithCancel(context.Background())
	postStreamCtx(ctx, newAPI(&fakeTracer{block: true, pre: func(trace.Sink) { cancel() }}), body)
	// Invalid requests never start traceroute.
	post(newAPI(&fakeTracer{hops: reached}), `{"endpoint":""}`)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, want := range []string{
		`traceroute_runs_total{outcome="reached"} 2`,
		`traceroute_runs_total{outcome="unreached"} 1`,
		`traceroute_runs_total{outcome="error"} 1`,
		`traceroute_runs_total{outcome="canceled"} 1`,
		`traceroute_active_runs 0`,
	} {
		if !strings.Contains(rec.Body.String(), "\n"+want+"\n") {
			t.Errorf("missing series %q", want)
		}
	}
}
