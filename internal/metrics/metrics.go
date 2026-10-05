package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// Counters are process-wide, following the same "initialise once, call via
// package functions" rule as the logger.
var (
	requestsTotal atomic.Int64
	deniedTotal   atomic.Int64
	cacheHits     atomic.Int64
	cacheMisses   atomic.Int64
	errorsTotal   atomic.Int64
)

// IncRequest records an update handled for an allowed user.
func IncRequest() { requestsTotal.Add(1) }

// IncDenied records an update from a user who is not on the allow list.
func IncDenied() { deniedTotal.Add(1) }

// IncCacheHit records a schedule site response served from Redis.
func IncCacheHit() { cacheHits.Add(1) }

// IncCacheMiss records a request that had to go to the schedule site.
func IncCacheMiss() { cacheMisses.Add(1) }

// IncErrors records an error-level event. Called by the logging hook, not
// directly.
func IncErrors() { errorsTotal.Add(1) }

// Handler renders the counters in Prometheus text exposition format. Hand-rolled
// because a handful of atomics does not justify the client library's weight.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/plain; version=0.0.4")

		fmt.Fprintf(w, "# TYPE knubaschedulebot_requests_total counter\nknubaschedulebot_requests_total %d\n", requestsTotal.Load())
		fmt.Fprintf(w, "# TYPE knubaschedulebot_denied_total counter\nknubaschedulebot_denied_total %d\n", deniedTotal.Load())
		fmt.Fprintf(w, "# TYPE knubaschedulebot_cache_hits_total counter\nknubaschedulebot_cache_hits_total %d\n", cacheHits.Load())
		fmt.Fprintf(w, "# TYPE knubaschedulebot_cache_misses_total counter\nknubaschedulebot_cache_misses_total %d\n", cacheMisses.Load())
		fmt.Fprintf(w, "# TYPE knubaschedulebot_errors_total counter\nknubaschedulebot_errors_total %d\n", errorsTotal.Load())
	})
}
