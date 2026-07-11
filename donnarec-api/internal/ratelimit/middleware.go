// Package ratelimit implements a per-IP token-bucket rate limiter as a gin
// middleware (D-83, T-06-05) — the first defensive layer on the public
// donation route group. It is deliberately in-memory (golang.org/x/time/rate)
// rather than a DB-backed counter: a DB write on every single public request
// (including rejected/bot traffic) would add write load Postgres doesn't need
// to carry (06-RESEARCH "Don't Hand-Roll").
//
// Placement: PerIP is registered BEFORE CAPTCHA verification and before any
// DB/storage call, so a flood is rejected as cheaply as possible (T-06-05).
package ratelimit

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// cleanupInterval is how often the background goroutine sweeps the visitor
// map for stale entries.
const cleanupInterval = 1 * time.Minute

// staleAfter is how long a visitor may go unseen before its limiter is
// evicted — bounds long-run memory growth from one-off/bot IPs that never
// return.
const staleAfter = 10 * time.Minute

// visitor tracks a single IP's token-bucket limiter and last-seen time (used
// by the cleanup sweep to decide eviction).
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter holds one token-bucket rate.Limiter per client IP, guarded by
// a mutex, with a background goroutine evicting stale visitors.
type IPRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     rate.Limit
	burst    int
}

// NewIPRateLimiter constructs an IPRateLimiter allowing r requests/sec
// (sustained) with a burst of b per IP, and starts its background cleanup
// goroutine (runs for the lifetime of the process — mirrors the outbox
// worker's existing unbounded background-goroutine lifecycle).
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	l := &IPRateLimiter{
		visitors: make(map[string]*visitor),
		rate:     r,
		burst:    b,
	}
	go l.cleanupVisitors()
	return l
}

// getVisitor returns the rate.Limiter for ip, creating one (starting with a
// full burst) on first sight, and refreshes its lastSeen timestamp.
func (l *IPRateLimiter) getVisitor(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	v, exists := l.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(l.rate, l.burst)
		l.visitors[ip] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}
	v.lastSeen = time.Now()
	return v.limiter
}

// cleanupVisitors periodically evicts visitors unseen for longer than
// staleAfter. Extracted as its own method (REFACTOR) so the eviction sweep
// is independently readable/testable from the request-path hot code
// (getVisitor/PerIP).
func (l *IPRateLimiter) cleanupVisitors() {
	for {
		time.Sleep(cleanupInterval)
		l.mu.Lock()
		for ip, v := range l.visitors {
			if time.Since(v.lastSeen) > staleAfter {
				delete(l.visitors, ip)
			}
		}
		l.mu.Unlock()
	}
}

// PerIP returns a gin.HandlerFunc enforcing a per-client-IP token bucket
// (r requests/sec sustained, burst b). On exhaustion it aborts with 429
// ({"error": "rate_limited"}) before any downstream middleware/handler runs.
//
// IP resolution reuses gin's c.ClientIP() (X-Forwarded-For-aware) — this
// package deliberately does not re-implement proxy-header parsing
// (06-RESEARCH "Don't Hand-Roll").
func PerIP(r rate.Limit, b int) gin.HandlerFunc {
	limiter := NewIPRateLimiter(r, b)
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.getVisitor(ip).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate_limited",
			})
			return
		}
		c.Next()
	}
}
