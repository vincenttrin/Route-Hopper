package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/tamtrinh/traffic-visualizer/backend/geo"
	"github.com/tamtrinh/traffic-visualizer/backend/handlers"
	"github.com/tamtrinh/traffic-visualizer/backend/trace"
)

func main() {
	if len(os.Args) > 1 {
		os.Exit(runCommand(os.Args[1:]))
	}

	tracer := trace.NewTracer()
	locator := geo.NewSwappable(openGeo())
	defer locator.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("GEOIP_AUTO_UPDATE") == "true" {
		go newGeoUpdater(locator).Run(ctx)
	}

	api := handlers.NewAPI(tracer, trace.SystemResolver, locator, envInt("MAX_CONCURRENT_TRACES", 4))
	api.AllowPrivate = os.Getenv("ALLOW_PRIVATE_TARGETS") == "true"

	mux := http.NewServeMux()
	// Both trace endpoints draw on one rate limit.
	traces := http.NewServeMux()
	traces.HandleFunc("POST /api/trace", api.Trace)
	traces.HandleFunc("POST /api/trace/stream", api.TraceStream)
	limited := handlers.RateLimit(envInt("RATE_LIMIT_PER_MINUTE", 20), 5, traces)
	mux.Handle("/api/trace", limited)
	mux.Handle("/api/trace/stream", limited)
	mux.HandleFunc("GET /api/geoip", api.GeoInfo)
	mux.HandleFunc("GET /healthz", api.Health)

	srv := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      tracer.Timeout + tracer.ICMPTimeout + 15*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), tracer.Timeout+tracer.ICMPTimeout+20*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Printf("listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func geoPaths() (city, asn string) {
	return env("GEOIP_CITY_DB", "GeoLite2-City.mmdb"), env("GEOIP_ASN_DB", "GeoLite2-ASN.mmdb")
}

func newGeoUpdater(target *geo.Swappable) *geo.Updater {
	city, asn := geoPaths()
	u := geo.NewUpdater(city, asn, geo.NewSource(os.Getenv("MAXMIND_LICENSE_KEY")), target)
	u.Logf = log.Printf
	return u
}

// runCommand runs a subcommand: `fetch-geoip [--force]` downloads the databases
// (what scripts/fetch-geoip.sh runs). It returns the exit code.
func runCommand(args []string) int {
	if args[0] != "fetch-geoip" || len(args) > 2 || (len(args) == 2 && args[1] != "--force") {
		fmt.Fprintln(os.Stderr, "usage: traffic-visualizer-api [fetch-geoip [--force]]")
		return 2
	}
	force := len(args) == 2 || os.Getenv("FORCE") == "1"
	u := newGeoUpdater(nil)
	u.Logf = func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	if !force && u.Present() {
		fmt.Printf("GeoIP databases already present (%s); use --force to refresh\n", filepath.Dir(u.CityPath))
		return 0
	}
	if _, err := u.Update(context.Background(), true); err != nil {
		fmt.Fprintln(os.Stderr, "fetch-geoip:", err)
		return 1
	}
	fmt.Printf("Source: %s. Attribution is shown in the app; review the license terms.\n", u.Source.Name())
	return 0
}

// openGeo opens the MaxMind databases named by GEOIP_CITY_DB and GEOIP_ASN_DB.
// Without a city database the server still runs, but hops carry no coordinates.
func openGeo() geo.Locator {
	cityPath, asnPath := geoPaths()
	if _, err := os.Stat(cityPath); err != nil {
		log.Printf("no GeoIP database loaded, hops will not be placed on the map until one is available: %v (see backend/README.md)", err)
		return geo.Nop{}
	}
	if _, err := os.Stat(asnPath); err != nil {
		log.Printf("network owner lookup disabled: %v", err)
		asnPath = ""
	}
	m, err := geo.OpenMaxMind(cityPath, asnPath)
	if err != nil {
		// Serve without locations rather than not at all; a refresh can replace the bad files.
		log.Printf("geolocation disabled: %v", err)
		return geo.Nop{}
	}
	return m
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}
