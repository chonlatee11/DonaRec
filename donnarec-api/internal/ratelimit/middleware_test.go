// internal/ratelimit/middleware_test.go — RED tests for the per-IP token-bucket
// gin middleware (06-PLAN-02 <feature> behavior spec): with a limiter configured
// N=2 burst over a long window, the first 2 requests from one IP pass, the 3rd
// aborts with 429, and a different IP is isolated from the first IP's exhausted
// bucket. Black-box test package, mirrors the codebase's `_test` convention.
package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"

	"github.com/donnarec/donnarec-api/internal/ratelimit"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestRouter wires ratelimit.PerIP with burst=2 and a near-zero sustained
// rate (long refill window) — this project's "N=2 burst over a long window"
// spec — so exactly 2 requests per IP pass before the 3rd is rejected, with
// no need to sleep/wait in the test.
func newTestRouter() *gin.Engine {
	router := gin.New()
	router.Use(ratelimit.PerIP(rate.Limit(0.0001), 2))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func doRequest(router *gin.Engine, remoteAddr string) int {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = remoteAddr
	router.ServeHTTP(w, req)
	return w.Code
}

func TestPerIP_TokenBucket(t *testing.T) {
	t.Run("first 2 requests from an IP pass, 3rd is 429", func(t *testing.T) {
		router := newTestRouter()

		assert.Equal(t, http.StatusOK, doRequest(router, "1.2.3.4:1111"), "1st request should pass")
		assert.Equal(t, http.StatusOK, doRequest(router, "1.2.3.4:1111"), "2nd request should pass")
		assert.Equal(t, http.StatusTooManyRequests, doRequest(router, "1.2.3.4:1111"), "3rd request should be rate-limited")
	})

	t.Run("a different IP is isolated from an exhausted IP's bucket", func(t *testing.T) {
		router := newTestRouter()

		doRequest(router, "1.2.3.4:1111")
		doRequest(router, "1.2.3.4:1111")
		exhausted := doRequest(router, "1.2.3.4:1111")
		assert.Equal(t, http.StatusTooManyRequests, exhausted, "IP 1.2.3.4 should be exhausted after 2 requests")

		otherIP := doRequest(router, "5.6.7.8:2222")
		assert.Equal(t, http.StatusOK, otherIP, "a different IP must not be blocked by IP 1.2.3.4's exhausted bucket")
	})
}
