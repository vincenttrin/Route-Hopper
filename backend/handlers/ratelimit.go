package handlers

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type visitor struct {
	limiter *rate.Limiter
	seen    time.Time
}

// RateLimit allows each client address a burst of requests that refills at
// perMinute. Clients are identified by the connection's remote address, so
// behind a reverse proxy the limit applies to the proxy unless it is replaced
// by one that sets RemoteAddr from a trusted header.
func RateLimit(perMinute, burst int, next http.Handler) http.Handler {
	var mu sync.Mutex
	visitors := map[string]*visitor{}
	var lastSweep time.Time

	allow := func(key string) bool {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		if now.Sub(lastSweep) > time.Minute {
			for k, v := range visitors {
				if now.Sub(v.seen) > 10*time.Minute {
					delete(visitors, k)
				}
			}
			lastSweep = now
		}
		v, ok := visitors[key]
		if !ok {
			v = &visitor{limiter: rate.NewLimiter(rate.Limit(float64(perMinute)/60), burst)}
			visitors[key] = v
		}
		v.seen = now
		return v.limiter.Allow()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !allow(host) {
			w.Header().Set("Retry-After", "10")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
