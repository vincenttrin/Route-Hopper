package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tamtrinh/traffic-visualizer/backend/geo"
	"github.com/tamtrinh/traffic-visualizer/backend/trace"
)

const destIP = "93.184.216.34"

// Where the test source sits: Omaha, behind a Cox router. Every one of these
// strings identifies it and must never reach a client.
var sourceSecrets = []string{
	"192.168.1.1", "gateway.lan", "10.20.0.1", "100.64.7.7",
	"68.1.4.1", "ae-1.rtr.omaha.cox.net", "68.1.4.2", "ae-2.rtr.omaha.cox.net",
	"68.1.1.37", "chgil-cr1.cox.net", "Omaha", "Chicago", "41.25", "-95.94", "Cox",
}

func sourceGeo() fakeGeo {
	return fakeGeo{
		addr("68.1.4.1"):     {Lat: 41.25, Lng: -95.94, City: "Omaha", Country: "US", Org: "Cox"},
		addr("68.1.4.2"):     {Lat: 41.3, Lng: -96.0, City: "Omaha", Country: "US", Org: "Cox"},     // a few km from the first
		addr("68.1.1.37"):    {Lat: 41.88, Lng: -87.63, City: "Chicago", Country: "US", Org: "Cox"}, // Cox's backbone, 700 km away
		addr("4.69.201.6"):   {Lat: 39.04, Lng: -77.49, City: "Ashburn", Country: "US", Org: "Lumen"},
		addr("213.200.80.1"): {Lat: 53.35, Lng: -6.26, City: "Dublin", Country: "IE", Org: "GTT"},
		addr(destIP):         {Lat: 52.37, Lng: 4.9, City: "Amsterdam", Country: "NL", Org: "Edgecast"},
		addr("192.168.1.1"):  {Lat: 41.26, Lng: -95.93, City: "Omaha"}, // must never be used: private
		addr("100.64.7.7"):   {Lat: 41.26, Lng: -95.93, City: "Omaha"},
	}
}

func hop(n int, ip, host string, rtt float64) trace.Hop {
	h := trace.Hop{Number: n, Hostname: host, RTT: rtt}
	if ip != "" {
		h.IP = addr(ip)
	}
	return h
}

// run feeds hops through a redactor the way the handlers do and returns what the client gets.
func run(g geo.Locator, dest string, hops ...trace.Hop) []hopJSON {
	api := NewAPI(&fakeTracer{}, fakeResolver{}, g, 1)
	red := api.redactor(addr(dest))
	var out []hopJSON
	for _, h := range hops {
		out = append(out, red.Add(h)...)
	}
	return append(out, red.Flush()...)
}

func ips(hops []hopJSON) string {
	var out []string
	for _, h := range hops {
		switch {
		case h.Hidden:
			out = append(out, "hidden")
		case h.IP == "":
			out = append(out, "silent")
		default:
			out = append(out, h.IP)
		}
	}
	return strings.Join(out, ",")
}

func sourceTrace() []trace.Hop {
	return []trace.Hop{
		hop(1, "192.168.1.1", "gateway.lan", 0.9),
		hop(2, "10.20.0.1", "", 2),
		hop(3, "", "", 0),
		hop(4, "100.64.7.7", "", 3),
		hop(5, "68.1.4.1", "ae-1.rtr.omaha.cox.net", 8),
		hop(6, "68.1.4.2", "ae-2.rtr.omaha.cox.net", 9),
		hop(7, "68.1.1.37", "chgil-cr1.cox.net", 19),
		hop(8, "4.69.201.6", "ae2.cr2.iad1.lumen.net", 35),
		hop(9, "", "", 0),
		hop(10, "213.200.80.1", "ae-12.dub.gtt.net", 113),
		hop(11, destIP, "example.com", 128),
	}
}

func TestRedactCutsTheSourceNetwork(t *testing.T) {
	got := run(sourceGeo(), destIP, sourceTrace()...)
	// LAN, silent and CGNAT hops and all of Cox (the ISP's first public router, its neighbour in the
	// same metro area, and its backbone hop in Chicago) are gone; one hidden hop stands in for them,
	// and the rest is renumbered.
	want := "hidden,4.69.201.6,silent,213.200.80.1," + destIP
	if g := ips(got); g != want {
		t.Fatalf("hops = %v, want %v", g, want)
	}
	for i, h := range got {
		if h.HopNumber != i+1 {
			t.Errorf("hop %d is numbered %d", i+1, h.HopNumber)
		}
	}
	if got[0] != (hopJSON{HopNumber: 1, Hidden: true}) {
		t.Errorf("hidden hop carries data: %+v", got[0])
	}
	if got[1].City != "Ashburn" || got[1].Hostname != "ae2.cr2.iad1.lumen.net" || got[1].Org != "Lumen" {
		t.Errorf("hop past the source network lost its details: %+v", got[1])
	}
	body, _ := json.Marshal(got)
	for _, secret := range sourceSecrets {
		if strings.Contains(string(body), secret) {
			t.Errorf("response leaks %q: %s", secret, body)
		}
	}
}

func TestRedactEndToEnd(t *testing.T) {
	// Both endpoints must apply it: the JSON body and every streamed event.
	api := NewAPI(&fakeTracer{hops: sourceTrace()}, fakeResolver{}, sourceGeo(), 2)
	streamed := postStream(api, `{"endpoint":"`+destIP+`"}`).Body.String()
	for name, body := range map[string]string{
		"trace":  post(api, `{"endpoint":"`+destIP+`"}`).Body.String(),
		"stream": streamed,
	} {
		for _, secret := range sourceSecrets {
			if strings.Contains(body, secret) {
				t.Errorf("%s response leaks %q: %s", name, secret, body)
			}
		}
	}
	var hops []hopJSON
	for _, ev := range decodeEvents(t, streamed) {
		if ev.Type == "hop" {
			hops = append(hops, *ev.Hop)
		}
	}
	if g := ips(hops); g != "hidden,4.69.201.6,silent,213.200.80.1,"+destIP || hops[4].HopNumber != 5 {
		t.Errorf("streamed hops = %v", g)
	}
}

func TestRedactEndsWhereTheNetworkChanges(t *testing.T) {
	t.Run("a hop in the same metro area is hidden even in another network", func(t *testing.T) {
		g := sourceGeo()
		g[addr("4.68.62.9")] = geo.Location{Lat: 41.3, Lng: -96.1, City: "Omaha", Country: "US", Org: "Lumen"} // 10 km from Cox's router
		got := run(g, destIP, hop(1, "192.168.1.1", "", 1), hop(2, "68.1.4.1", "", 8), hop(3, "4.68.62.9", "omaha.lumen.net", 12), hop(4, "4.69.201.6", "", 35), hop(5, destIP, "", 30))
		if s := ips(got); s != "hidden,4.69.201.6,"+destIP {
			t.Errorf("hops = %v", s)
		}
	})
	t.Run("the same network owner far away is still the origin", func(t *testing.T) {
		// Cox's own backbone hop is 700 km from its first router and has a city code in its name.
		got := run(sourceGeo(), destIP,
			hop(1, "192.168.1.1", "", 1), hop(2, "68.1.4.1", "", 8), hop(3, "68.1.1.37", "chgil-cr1.cox.net", 19), hop(4, "4.69.201.6", "", 35), hop(5, destIP, "", 30))
		if s := ips(got); s != "hidden,4.69.201.6,"+destIP {
			t.Errorf("hops = %v", s)
		}
	})
	t.Run("the same host name domain counts when nothing is located", func(t *testing.T) {
		got := run(geo.Nop{}, destIP,
			hop(1, "192.168.1.1", "", 1), hop(2, "68.1.4.1", "ae-1.rtr.omaha.cox.net", 8), hop(3, "68.1.1.37", "chgil-cr1.cox.net", 19), hop(4, "4.69.201.6", "ae2.cr2.iad1.lumen.net", 35), hop(5, destIP, "", 30))
		if s := ips(got); s != "hidden,4.69.201.6,"+destIP {
			t.Errorf("hops = %v", s)
		}
	})
	t.Run("a nearby hop is hidden even when its owner is unknown", func(t *testing.T) {
		g := sourceGeo()
		g[addr("68.2.2.2")] = geo.Location{Lat: 41.31, Lng: -96.0, City: "Omaha", Country: "US"} // no owner, no name
		got := run(g, destIP, hop(1, "192.168.1.1", "", 1), hop(2, "68.1.4.1", "", 8), hop(3, "68.2.2.2", "", 9), hop(4, "4.69.201.6", "", 35))
		if s := ips(got); s != "hidden,4.69.201.6" {
			t.Errorf("hops = %v", s)
		}
	})
	t.Run("an unrelated unlocated hop ends the origin", func(t *testing.T) {
		// 68.9.9.9 has no database entry and a different name: nothing says it is the source's network, so it is shown.
		got := run(sourceGeo(), destIP,
			hop(1, "192.168.1.1", "", 1), hop(2, "68.1.4.1", "", 8), hop(3, "68.9.9.9", "core.example.net", 12), hop(4, destIP, "", 30))
		if s := ips(got); s != "hidden,68.9.9.9,"+destIP {
			t.Errorf("hops = %v", s)
		}
	})
	t.Run("silent hops between hops of the origin are part of it", func(t *testing.T) {
		got := run(sourceGeo(), destIP,
			hop(1, "192.168.1.1", "", 1), hop(2, "68.1.4.1", "", 8), hop(3, "", "", 0), hop(4, "68.1.4.2", "", 9), hop(5, "4.69.201.6", "", 35))
		if s := ips(got); s != "hidden,4.69.201.6" {
			t.Errorf("hops = %v", s)
		}
	})
	t.Run("silent hops after the origin are kept", func(t *testing.T) {
		got := run(sourceGeo(), destIP, hop(1, "192.168.1.1", "", 1), hop(2, "68.1.4.1", "", 8), hop(3, "", "", 0), hop(4, "", "", 0))
		if s := ips(got); s != "hidden,silent,silent" {
			t.Errorf("hops = %v", s)
		}
	})
}

func TestRedactWithoutADatabaseHidesTheFirstPublicHop(t *testing.T) {
	// Without locations or names nothing says which later hops belong to the source's network.
	got := run(geo.Nop{}, destIP,
		hop(1, "192.168.1.1", "gateway.lan", 1), hop(2, "68.1.4.1", "", 8), hop(3, "68.1.4.2", "", 9), hop(4, destIP, "", 30))
	if g := ips(got); g != "hidden,68.1.4.2,"+destIP {
		t.Errorf("hops = %v", g)
	}
	body, _ := json.Marshal(got)
	for _, secret := range []string{"192.168.1.1", "gateway.lan", "68.1.4.1"} {
		if strings.Contains(string(body), secret) {
			t.Errorf("response leaks %q", secret)
		}
	}
}

func TestRedactNeverHidesTheDestination(t *testing.T) {
	t.Run("a destination next to the source", func(t *testing.T) {
		got := run(sourceGeo(), "68.1.4.2", hop(1, "192.168.1.1", "", 1), hop(2, "68.1.4.1", "", 8), hop(3, "68.1.4.2", "", 9))
		if g := ips(got); g != "hidden,68.1.4.2" {
			t.Errorf("hops = %v", g)
		}
	})
	t.Run("a destination that is the first hop", func(t *testing.T) {
		got := run(sourceGeo(), destIP, hop(1, destIP, "", 1))
		if g := ips(got); g != destIP {
			t.Errorf("hops = %v", g)
		}
	})
	t.Run("a destination on the local network", func(t *testing.T) {
		got := run(geo.Nop{}, "192.168.1.50", hop(1, "192.168.1.1", "", 1), hop(2, "192.168.1.50", "", 2))
		if g := ips(got); g != "hidden,192.168.1.50" {
			t.Errorf("hops = %v", g)
		}
	})
}

func TestRedactTracesThatNeverLeave(t *testing.T) {
	// Docker Desktop style: only the gateway answers. The client still gets a hop to draw.
	got := run(sourceGeo(), destIP, hop(1, "172.21.0.1", "", 0.1), hop(2, "", "", 0), hop(3, "", "", 0))
	if g := ips(got); g != "hidden" {
		t.Errorf("hops = %v", g)
	}
	if got := run(sourceGeo(), destIP); len(got) != 0 {
		t.Errorf("an empty trace gained hops: %v", got)
	}
}

func TestRedactResetStartsOver(t *testing.T) {
	api := NewAPI(&fakeTracer{}, fakeResolver{}, sourceGeo(), 1)
	red := api.redactor(addr(destIP))
	red.Add(hop(1, "192.168.1.1", "", 1))
	red.Add(hop(2, "68.1.4.1", "", 8))
	red.Add(hop(3, "", "", 0))
	red.reset()
	out := red.Add(hop(1, "192.168.1.1", "", 1))
	out = append(out, red.Add(hop(2, "68.1.4.1", "", 8))...)
	out = append(out, red.Add(hop(3, "4.69.201.6", "", 19))...)
	out = append(out, red.Add(hop(4, destIP, "", 30))...)
	if g := ips(out); g != "hidden,4.69.201.6,"+destIP || out[2].HopNumber != 3 {
		t.Errorf("hops after reset = %v", g)
	}
}

func TestRegisteredDomain(t *testing.T) {
	for host, want := range map[string]string{
		"ae-1.rtr.omaha.cox.net": "cox.net", "CHGIL-cr1.Cox.NET.": "cox.net", "example.com": "example.com", "localhost": "", "": "",
	} {
		if got := registeredDomain(host); got != want {
			t.Errorf("registeredDomain(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestLeavesNetwork(t *testing.T) {
	for ip, want := range map[string]bool{
		"8.8.8.8": true, "93.184.216.34": true, "2606:4700::1111": true,
		"10.0.0.1": false, "192.168.1.1": false, "172.16.0.1": false, "127.0.0.1": false,
		"169.254.1.1": false, "100.64.0.1": false, "100.127.255.254": false, "100.128.0.1": true,
		"::1": false, "fd00::1": false, "fe80::1": false,
	} {
		if got := leavesNetwork(addr(ip)); got != want {
			t.Errorf("leavesNetwork(%s) = %v, want %v", ip, got, want)
		}
	}
}
