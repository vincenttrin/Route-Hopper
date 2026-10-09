package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/tamtrinh/traffic-visualizer/backend/geo"
	"github.com/tamtrinh/traffic-visualizer/backend/handlers"
	"github.com/tamtrinh/traffic-visualizer/backend/trace"
)

func main() {
	tracer := trace.NewTracer()
	locator := openGeo()
	defer locator.Close()

	api := handlers.NewAPI(tracer, trace.SystemResolver, geo.Cached(locator), envInt("MAX_CONCURRENT_TRACES", 4))
	api.AllowPrivate = os.Getenv("ALLOW_PRIVATE_TARGETS") == "true"

	mux := http.NewServeMux()
	mux.Handle("POST /api/trace", handlers.RateLimit(envInt("RATE_LIMIT_PER_MINUTE", 20), 5, http.HandlerFunc(api.Trace)))
	mux.HandleFunc("GET /healthz", handlers.Health)

	srv := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      tracer.Timeout + tracer.ICMPTimeout + 15*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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

// openGeo opens the MaxMind databases named by GEOIP_CITY_DB and GEOIP_ASN_DB.
// Without a city database the server still runs, but hops carry no coordinates.
func openGeo() geo.Locator {
	cityPath := env("GEOIP_CITY_DB", "GeoLite2-City.mmdb")
	if _, err := os.Stat(cityPath); err != nil {
		log.Printf("geolocation disabled: %v (see backend/README.md)", err)
		return geo.Nop{}
	}
	asnPath := env("GEOIP_ASN_DB", "GeoLite2-ASN.mmdb")
	if _, err := os.Stat(asnPath); err != nil {
		log.Printf("network owner lookup disabled: %v", err)
		asnPath = ""
	}
	m, err := geo.OpenMaxMind(cityPath, asnPath)
	if err != nil {
		log.Fatalf("geolocation: %v", err)
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
