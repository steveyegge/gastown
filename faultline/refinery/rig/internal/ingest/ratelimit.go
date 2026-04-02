package ingest

import (
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiter provides per-project token bucket rate limiting.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[int64]*rate.Limiter
	rate     rate.Limit
	burst    int
	log      *slog.Logger
}

// NewRateLimiter creates a rate limiter with the given per-project events/second limit.
func NewRateLimiter(eventsPerSecond float64, log *slog.Logger) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[int64]*rate.Limiter),
		rate:     rate.Limit(eventsPerSecond),
		burst:    int(eventsPerSecond), // burst = 1 second of tokens
		log:      log,
	}
}

func (rl *RateLimiter) limiterFor(projectID int64) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	lim, ok := rl.limiters[projectID]
	if !ok {
		lim = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[projectID] = lim
	}
	return lim
}

// Wrap returns middleware that enforces per-project rate limits.
func (rl *RateLimiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := pathProjectID(r)
		if projectID == 0 {
			next(w, r)
			return
		}

		lim := rl.limiterFor(projectID)
		if !lim.Allow() {
			rl.log.Warn("rate limited", "project", projectID)
			retryAfter := retryAfterSeconds(lim)
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.Header().Set("X-Sentry-Rate-Limits", formatRateLimitHeader(retryAfter))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"detail": fmt.Sprintf("rate limit exceeded for project %d", projectID),
			})
			return
		}
		next(w, r)
	}
}

// retryAfterSeconds returns the number of whole seconds until the next token is available.
func retryAfterSeconds(lim *rate.Limiter) int {
	r := lim.Reserve()
	delay := r.Delay()
	r.Cancel()
	secs := int(math.Ceil(delay.Seconds()))
	if secs < 1 {
		secs = 1
	}
	return secs
}

// formatRateLimitHeader returns the X-Sentry-Rate-Limits value.
// Format: "<retry_after>:<categories>:<scope>"
// See https://develop.sentry.dev/sdk/rate-limiting/
func formatRateLimitHeader(retryAfter int) string {
	return fmt.Sprintf("%d::project", retryAfter)
}
