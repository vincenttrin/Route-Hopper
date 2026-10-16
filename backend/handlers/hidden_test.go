package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tamtrinh/traffic-visualizer/backend/geo"
	"github.com/tamtrinh/traffic-visualizer/backend/trace"
)

// secrets are the details of the first two hops of leakyRoute. None of them may
// reach a client when two hops are hidden.
var secrets = []string{"192.168.77.1", "secret-gateway.lan", "10.99.88.77", "inner-isp.example", "Secretville", "SecretNet", "7.77", "8.88"}

func leakyRoute() []trace.Hop {
	return []trace.Hop{
		{Number: 1, IP: addr("192.168.77.1"), Hostname: "secret-gateway.lan", RTT: 7.77},
		{Number: 2, IP: addr("10.99.88.77"), Hostname: "inner-isp.example", RTT: 8.88},
		{Number: 3, IP: addr("96.34.20.4"), Hostname: "edge.cox.net", RTT: 9},
		{Number: 4},
		{Number: 5, IP: addr("93.184.216.34"), Hostname: "example.com", RTT: 12},
	}
}

func leakGeo() fakeGeo {
	return fakeGeo{
		addr("96.34.20.4"):    {Lat: 41.25, Lng: -95.94, City: "Omaha", Country: "US", Org: "Cox"},
		addr("93.184.216.34"): {Lat: 52.37, Lng: 4.9, City: "Amsterdam", Country: "NL", Org: "Edgecast"},
		// Private hops are never located, but a database entry must not matter either way.
		addr("192.168.77.1"): {Lat: 1.1, Lng: 2.2, City: "Secretville", Org: "SecretNet"},
		addr("10.99.88.77"):  {Lat: 3.3, Lng: 4.4, City: "Secretville", Org: "SecretNet"},
	}
}

func hiddenAPI(f *fakeTracer, hidden int) *API {
	api := NewAPI(f, fakeResolver{}, leakGeo(), 2)
	api.HiddenHops = hidden
	return api
}

func noSecrets(t *testing.T, what, body string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(body, s) {
			t.Errorf("%s leaks %q:\n%s", what, s, body)
		}
	}
}

func hopNumbers(hops []hopJSON) []int {
	var n []int
	for _, h := range hops {
		n = append(n, h.HopNumber)
	}
	return n
}

func TestHiddenHopsNeverReachTheClient(t *testing.T) {
	// The UDP pass sees the whole route but not the destination, so the ICMP
	// retry replays it from hop 1 after a reset.
	udp := leakyRoute()[:4]
	retry := &fakeTracer{hops: leakyRoute(), pre: func(s trace.Sink) {
		for _, h := range udp {
			s.Hop(h)
		}
		s.Phase(trace.PhaseICMP)
		s.Reset()
	}}
	plain := &fakeTracer{hops: leakyRoute()}

	t.Run("plain", func(t *testing.T) {
		rec := post(hiddenAPI(plain, 2), `{"endpoint":"93.184.216.34"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		noSecrets(t, "plain response", rec.Body.String())
		var got traceResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if n := hopNumbers(got.Hops); len(n) != 3 || n[0] != 1 || n[1] != 2 || n[2] != 3 {
			t.Errorf("hop numbers = %v, want 1 2 3", n)
		}
		if got.Hops[0].IP != "96.34.20.4" || got.Hops[2].IP != "93.184.216.34" || !got.Reached {
			t.Errorf("hops = %+v reached=%v", got.Hops, got.Reached)
		}
	})

	for name, tr := range map[string]*fakeTracer{"stream": plain, "stream with ICMP retry": retry} {
		t.Run(name, func(t *testing.T) {
			rec := postStream(hiddenAPI(tr, 2), `{"endpoint":"93.184.216.34"}`)
			noSecrets(t, "stream", rec.Body.String())
			events := decodeEvents(t, rec.Body.String())
			var shown []hopJSON
			for _, ev := range events {
				if ev.Type == "hop" {
					shown = append(shown, *ev.Hop)
				}
				if ev.Type == "reset" {
					shown = nil
				}
			}
			if n := hopNumbers(shown); len(n) != 3 || n[0] != 1 || n[1] != 2 || n[2] != 3 {
				t.Errorf("hop numbers after the last reset = %v, want 1 2 3", n)
			}
			if done := events[len(events)-1]; done.Type != "done" || done.Reached == nil || !*done.Reached {
				t.Errorf("last event = %+v", done)
			}
		})
	}
}

func TestHiddenHopsStayOutOfWarnings(t *testing.T) {
	// Docker Desktop style: the gateway answers, then nothing. The warning is
	// still given, and says nothing about the hops.
	dockerLike := []trace.Hop{
		{Number: 1, IP: addr("192.168.77.1"), Hostname: "secret-gateway.lan"},
		{Number: 2}, {Number: 3}, {Number: 4},
	}
	api := hiddenAPI(&fakeTracer{hops: dockerLike}, 2)
	rec := post(api, `{"endpoint":"8.8.8.8"}`)
	noSecrets(t, "plain response", rec.Body.String())
	var got traceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Warning, "Docker Desktop") || len(got.Hops) != 2 {
		t.Errorf("response = %+v", got)
	}

	rec = postStream(api, `{"endpoint":"8.8.8.8"}`)
	noSecrets(t, "stream", rec.Body.String())
	events := decodeEvents(t, rec.Body.String())
	if done := events[len(events)-1]; !strings.Contains(done.Warning, "Docker Desktop") {
		t.Errorf("done = %+v", done)
	}
}

func TestLocationWarningIgnoresHiddenHops(t *testing.T) {
	// The hidden hops are located but the visible ones are not: the warning is about what the user sees.
	hops := []trace.Hop{
		{Number: 1, IP: addr("192.168.77.1")}, {Number: 2, IP: addr("96.34.20.4")},
		{Number: 3, IP: addr("93.184.216.34")}, {Number: 4, IP: addr("93.184.216.35")},
	}
	g := fakeGeo{addr("96.34.20.4"): {Lat: 41.25, Lng: -95.94}}
	api := NewAPI(&fakeTracer{hops: hops}, fakeResolver{}, g, 1)
	api.HiddenHops = 2
	if got := decodeTrace(t, api, "93.184.216.35"); !strings.Contains(got.Warning, "GeoIP") {
		t.Errorf("warning = %q", got.Warning)
	}
}

func TestShortTraceHasAMessageNotAnEmptyResult(t *testing.T) {
	tests := []struct {
		name string
		hops []trace.Hop
		dest string
	}{
		{"destination is the second hop", leakyRoute()[:2], "10.99.88.77"},
		{"destination never answered past the second hop", []trace.Hop{leakyRoute()[0], leakyRoute()[1]}, "93.184.216.34"},
		{"one hop", leakyRoute()[:1], "192.168.77.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := hiddenAPI(&fakeTracer{hops: tt.hops}, 2)
			api.AllowPrivate = true

			rec := post(api, `{"endpoint":"`+tt.dest+`"}`)
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "first 2 hops") {
				t.Errorf("plain: %d %q", rec.Code, rec.Body)
			}
			noSecrets(t, "plain error", rec.Body.String())

			rec = postStream(api, `{"endpoint":"`+tt.dest+`"}`)
			events := decodeEvents(t, rec.Body.String())
			last := events[len(events)-1]
			if last.Type != "error" || last.Status != http.StatusUnprocessableEntity || !strings.Contains(last.Error, "first 2 hops") {
				t.Errorf("stream last event = %+v", last)
			}
			for _, ev := range events {
				if ev.Type == "hop" || ev.Type == "done" {
					t.Errorf("unexpected %s event", ev.Type)
				}
			}
			// The destination is what the user typed, so it may appear; the other hops may not.
			body := strings.ReplaceAll(rec.Body.String(), tt.dest, "")
			noSecrets(t, "stream", body)
		})
	}
}

func TestHiddenHopsCanBeDisabled(t *testing.T) {
	api := NewAPI(&fakeTracer{hops: leakyRoute()}, fakeResolver{}, geo.Nop{}, 1)
	got := decodeTrace(t, api, "93.184.216.34")
	if len(got.Hops) != 5 || got.Hops[0].IP != "192.168.77.1" || got.Hops[0].HopNumber != 1 {
		t.Errorf("hops = %+v", got.Hops)
	}
}
