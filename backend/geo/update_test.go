package geo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

var now = time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)

// mmdb builds a one-record database. A City database knows 8.8.8.8 as `place`.
func mmdb(t *testing.T, dbType string, built time.Time, place string) []byte {
	t.Helper()
	w, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: dbType, RecordSize: 24, BuildEpoch: built.Unix(), IncludeReservedNetworks: true})
	if err != nil {
		t.Fatal(err)
	}
	var rec mmdbtype.Map
	if strings.Contains(dbType, "ASN") {
		rec = mmdbtype.Map{"autonomous_system_number": mmdbtype.Uint32(15169), "autonomous_system_organization": mmdbtype.String(place)}
	} else {
		rec = mmdbtype.Map{
			"city":     mmdbtype.Map{"names": mmdbtype.Map{"en": mmdbtype.String(place)}},
			"country":  mmdbtype.Map{"iso_code": mmdbtype.String("US")},
			"location": mmdbtype.Map{"latitude": mmdbtype.Float64(37.4), "longitude": mmdbtype.Float64(-122.1)},
		}
	}
	_, n, _ := net.ParseCIDR("8.8.8.0/24")
	if err := w.Insert(n, rec); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := w.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gz(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(b)
	zw.Close()
	return buf.Bytes()
}

func tgz(t *testing.T, name string, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg})
	tw.Write(b)
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

// publisher serves DB-IP style files for the months in `have`.
type publisher struct {
	t        *testing.T
	have     map[string]bool // "2026-10"
	built    time.Time
	place    string
	cityType string // the City file's database_type
	garbage  bool   // serve gzipped text, as an error page or a rate limiter would
	requests atomic.Int32
	fail     atomic.Bool
}

func newPublisher(t *testing.T, built time.Time, place string, months ...string) *publisher {
	p := &publisher{t: t, have: map[string]bool{}, built: built, place: place, cityType: "DBIP-City-Lite"}
	for _, m := range months {
		p.have[m] = true
	}
	return p
}

func (p *publisher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.requests.Add(1)
	if p.fail.Load() {
		http.Error(w, "down", http.StatusInternalServerError)
		return
	}
	// /free/dbip-city-lite-2026-10.mmdb.gz
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/free/dbip-"), ".mmdb.gz")
	kind, month, ok := strings.Cut(name, "-lite-")
	if !ok || !p.have[month] {
		http.NotFound(w, r)
		return
	}
	if p.garbage {
		w.Write(gz(p.t, []byte("<html>rate limited</html>")))
		return
	}
	if kind == "city" {
		w.Write(gz(p.t, mmdb(p.t, p.cityType, p.built, p.place)))
	} else {
		w.Write(gz(p.t, mmdb(p.t, "DBIP-ASN-Lite (compat=GeoLite2-ASN)", p.built, p.place)))
	}
}

func newUpdater(t *testing.T, srv *httptest.Server) (*Updater, *Swappable, string) {
	t.Helper()
	dir := t.TempDir()
	src := &DBIPSource{Client: srv.Client(), BaseURL: srv.URL + "/free", Now: func() time.Time { return now }}
	target := NewSwappable(Nop{})
	u := NewUpdater(filepath.Join(dir, "GeoLite2-City.mmdb"), filepath.Join(dir, "GeoLite2-ASN.mmdb"), src, target)
	u.Now = func() time.Time { return now }
	u.Logf = t.Logf
	return u, target, dir
}

func lookupPlace(t *testing.T, l Locator) string {
	t.Helper()
	loc, ok := l.Lookup(netip.MustParseAddr("8.8.8.8"))
	if !ok {
		return ""
	}
	return loc.City + "/" + loc.Org
}

func serve(t *testing.T, p *publisher) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdateInstallsAndSwaps(t *testing.T) {
	p := newPublisher(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "Mountain View", "2026-10")
	u, target, dir := newUpdater(t, serve(t, p))

	if info := target.Info(); info.Available {
		t.Fatalf("starts available: %+v", info)
	}
	updated, err := u.Update(context.Background(), false)
	if err != nil || !updated {
		t.Fatalf("Update = %v, %v", updated, err)
	}
	if got := lookupPlace(t, target); got != "Mountain View/Mountain View" {
		t.Errorf("lookup after swap = %q", got)
	}
	want := Info{Available: true, Provider: "DB-IP Lite", Updated: "2026-10-01"}
	if info := target.Info(); info != want {
		t.Errorf("Info = %+v, want %+v", info, want)
	}
	for _, f := range []string{"GeoLite2-City.mmdb", "GeoLite2-ASN.mmdb"} {
		if !nonEmpty(filepath.Join(dir, f)) {
			t.Errorf("%s was not installed", f)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
	if !u.Present() || u.Due() {
		t.Errorf("Present = %v, Due = %v after installing", u.Present(), u.Due())
	}
}

func TestUpdateFallsBackToLastMonth(t *testing.T) {
	// It is early October and DB-IP has not published October's file yet.
	p := newPublisher(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "Sept", "2026-09")
	u, target, _ := newUpdater(t, serve(t, p))
	if updated, err := u.Update(context.Background(), false); err != nil || !updated {
		t.Fatalf("Update = %v, %v", updated, err)
	}
	if got := target.Info().Updated; got != "2026-09-01" {
		t.Errorf("Updated = %q", got)
	}
}

func TestUpdateNewerDatabaseReplacesOlderWithoutRestart(t *testing.T) {
	p := newPublisher(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "Old", "2026-09", "2026-10")
	u, target, dir := newUpdater(t, serve(t, p))
	// September's database is in use, from an earlier run.
	if _, err := u.Update(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	p.have["2026-10"], p.built, p.place = true, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "New"
	old := lookupPlace(t, target)

	updated, err := u.Update(context.Background(), false)
	if err != nil || !updated {
		t.Fatalf("Update = %v, %v", updated, err)
	}
	if got := lookupPlace(t, target); old == got || got != "New/New" {
		t.Errorf("lookup = %q (was %q)", got, old)
	}
	// And the files on disk are the new ones, for the next start.
	m, err := OpenMaxMind(filepath.Join(dir, "GeoLite2-City.mmdb"), filepath.Join(dir, "GeoLite2-ASN.mmdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if got := lookupPlace(t, m); got != "New/New" {
		t.Errorf("file on disk = %q", got)
	}
}

func TestUpdateKeepsTheOldDatabaseWhenTheRefreshFails(t *testing.T) {
	good := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		break_ func(p *publisher, srv *httptest.Server)
	}{
		{"server error", func(p *publisher, _ *httptest.Server) { p.fail.Store(true) }},
		{"nothing published", func(p *publisher, _ *httptest.Server) { p.have = map[string]bool{} }},
		{"not an mmdb", func(p *publisher, _ *httptest.Server) { p.garbage = true }},
		{"wrong database type", func(p *publisher, _ *httptest.Server) { p.cityType = "DBIP-Country-Lite"; p.have["2026-10"] = true }},
		{"server gone", func(_ *publisher, srv *httptest.Server) { srv.Close() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPublisher(t, good, "Old", "2026-09", "2026-10")
			srv := serve(t, p)
			u, target, dir := newUpdater(t, srv)
			if _, err := u.Update(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(dir, "GeoLite2-City.mmdb"))
			p.built = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			tt.break_(p, srv)

			updated, err := u.Update(context.Background(), false)
			if err == nil || updated {
				t.Fatalf("Update = %v, %v; want an error", updated, err)
			}
			if got := lookupPlace(t, target); got != "Old/Old" {
				t.Errorf("databases in use changed to %q", got)
			}
			if after, _ := os.ReadFile(filepath.Join(dir, "GeoLite2-City.mmdb")); !bytes.Equal(before, after) {
				t.Error("the installed file was modified")
			}
			if left, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(left) != 0 {
				t.Errorf("temporary files left behind: %v", left)
			}
		})
	}
}

func TestUpdateSkipsADatabaseThatIsNotNewer(t *testing.T) {
	p := newPublisher(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "Same", "2026-10")
	u, target, _ := newUpdater(t, serve(t, p))
	if _, err := u.Update(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	// A month later the publisher still serves the same build: the age of the files is reset, not the databases.
	u.Now = func() time.Time { return now.Add(40 * 24 * time.Hour) }
	if !u.Due() {
		t.Fatal("not due after 40 days")
	}
	updated, err := u.Update(context.Background(), false)
	if err != nil || updated {
		t.Fatalf("Update = %v, %v; want no update", updated, err)
	}
	if u.Due() {
		t.Error("still due after confirming the databases are current")
	}
	if target.Info().Updated != "2026-10-01" {
		t.Errorf("Info = %+v", target.Info())
	}
}

func TestUpdateWithoutTargetOnlyWritesFiles(t *testing.T) {
	p := newPublisher(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "Disk", "2026-10")
	u, _, dir := newUpdater(t, serve(t, p))
	u.Target = nil
	if updated, err := u.Update(context.Background(), true); err != nil || !updated {
		t.Fatalf("Update = %v, %v", updated, err)
	}
	if !nonEmpty(filepath.Join(dir, "GeoLite2-City.mmdb")) {
		t.Error("city database not written")
	}
}

func TestDue(t *testing.T) {
	u := NewUpdater(filepath.Join(t.TempDir(), "c.mmdb"), filepath.Join(t.TempDir(), "a.mmdb"), nil, nil)
	if !u.Due() {
		t.Error("missing files are due")
	}
	for _, p := range []string{u.CityPath, u.ASNPath} {
		os.WriteFile(p, []byte("x"), 0o644)
	}
	u.Now = func() time.Time { return time.Now().Add(29 * 24 * time.Hour) }
	if u.Due() {
		t.Error("29 days old is not due")
	}
	u.Now = func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }
	if !u.Due() {
		t.Error("31 days old is due")
	}
}

func TestRunDownloadsOnFirstStartAndRetriesWhileOffline(t *testing.T) {
	p := newPublisher(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "First", "2026-10")
	p.fail.Store(true)
	u, target, _ := newUpdater(t, serve(t, p))
	u.RetryMissing = 10 * time.Millisecond
	u.CheckEvery = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { u.Run(ctx); close(done) }()

	// Offline: it keeps trying and the server keeps running without locations.
	deadline := time.Now().Add(5 * time.Second)
	for p.requests.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if p.requests.Load() < 3 || target.Info().Available {
		t.Fatalf("requests = %d, available = %v", p.requests.Load(), target.Info().Available)
	}
	// The network comes back.
	p.fail.Store(false)
	for !target.Info().Available && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := lookupPlace(t, target); got != "First/First" {
		t.Errorf("lookup = %q", got)
	}
	cancel()
	<-done
}

func TestSwappableLookupsSurviveSwaps(t *testing.T) {
	// Lookups racing swaps must never touch a closed database (run with -race).
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	open := func(place string) *MaxMind {
		built := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		m, err := OpenMaxMind(write(place+"-c", mmdb(t, "DBIP-City-Lite", built, place)), write(place+"-a", mmdb(t, "DBIP-ASN-Lite (compat=GeoLite2-ASN)", built, place)))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	s := NewSwappable(open("A"))
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if got := lookupPlace(t, s); got != "A/A" && got != "B/B" {
					t.Errorf("lookup = %q", got)
					return
				}
			}
		}()
	}
	for i := range 50 {
		s.Swap(open([]string{"B", "A"}[i%2]))
	}
	close(stop)
	wg.Wait()
	s.Close()
}

func TestSwapDropsTheLookupCache(t *testing.T) {
	s := NewSwappable(&counting{})
	ip := netip.MustParseAddr("1.1.1.1")
	s.Lookup(ip)
	s.Swap(Nop{})
	if _, ok := s.Lookup(ip); ok {
		t.Error("a cached result outlived the database it came from")
	}
}

func TestMaxMindSource(t *testing.T) {
	built := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	const key = "SECRETKEY123"
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("license_key") != key {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		asked = append(asked, q.Get("edition_id")+"/"+q.Get("suffix"))
		edition := strings.TrimPrefix(q.Get("edition_id"), "GeoLite2-")
		w.Write(tgz(t, "GeoLite2-"+edition+"_20261006/GeoLite2-"+edition+".mmdb", mmdb(t, q.Get("edition_id"), built, "MM")))
	}))
	defer srv.Close()

	dir := t.TempDir()
	mk := func(k string) *Updater {
		src := &MaxMindSource{Client: srv.Client(), BaseURL: srv.URL + "/app/geoip_download", LicenseKey: k}
		u := NewUpdater(filepath.Join(dir, "c.mmdb"), filepath.Join(dir, "a.mmdb"), src, NewSwappable(Nop{}))
		u.Now = func() time.Time { return now }
		return u
	}
	u := mk(key)
	if updated, err := u.Update(context.Background(), false); err != nil || !updated {
		t.Fatalf("Update = %v, %v", updated, err)
	}
	if want := "GeoLite2-City/tar.gz,GeoLite2-ASN/tar.gz"; strings.Join(asked, ",") != want {
		t.Errorf("requests = %v", asked)
	}
	if info := u.Target.Info(); info.Provider != "MaxMind GeoLite2" || info.Updated != "2026-10-06" {
		t.Errorf("Info = %+v", info)
	}

	// A wrong key fails with a hint, and the key itself is never in the error (it ends up in logs).
	_, err := mk("wrong-key-abc").Update(context.Background(), true)
	if err == nil || !strings.Contains(err.Error(), "MAXMIND_LICENSE_KEY") {
		t.Fatalf("err = %v", err)
	}
	srv.Close()
	_, err = mk(key).Update(context.Background(), true)
	if err == nil || strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "license_key") {
		t.Errorf("transport error leaks the key: %v", err)
	}
}

func TestNewSource(t *testing.T) {
	if _, ok := NewSource("").(*DBIPSource); !ok {
		t.Error("no key should use DB-IP")
	}
	if _, ok := NewSource("k").(*MaxMindSource); !ok {
		t.Error("a key should use MaxMind")
	}
}

func TestInfoProviders(t *testing.T) {
	for typ, want := range map[string]string{"DBIP-City-Lite": "DB-IP Lite", "GeoLite2-City": "MaxMind GeoLite2", "GeoIP2-City": "GeoIP2-City"} {
		if got := provider(typ); got != want {
			t.Errorf("provider(%q) = %q, want %q", typ, got, want)
		}
	}
	if (Nop{}).Close() != nil || infoFor(Nop{}).Available {
		t.Error("Nop has no database")
	}
}
