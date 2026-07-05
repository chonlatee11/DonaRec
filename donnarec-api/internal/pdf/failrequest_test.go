// Package pdf — WR-02 regression coverage (04-REVIEW.md): a failed
// fetch.FailRequest call (e.g. the paused request was already handled, or the
// CDP context was mid-teardown) must be logged, never silently swallowed —
// previously `_ = chromedp.Run(...)` threw the error away entirely, so a
// stuck paused request had zero diagnostic signal until renderTimeout (30s).
package pdf

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/fetch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestFailRequestAndLog_LogsErrorInsteadOfSwallowing proves failRequestAndLog
// logs (rather than discards) a FailRequest error. context.Background() has
// no cdp executor bound, so fetch.FailRequest(...).Do(ctx) deterministically
// returns cdp.ErrInvalidContext — standing in for any real-world FailRequest
// failure (race with an already-handled request, context mid-teardown, etc)
// without needing a live Chromium instance.
func TestFailRequestAndLog_LogsErrorInsteadOfSwallowing(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	failRequestAndLog(context.Background(), logger, fetch.RequestID("test-request-id-123"))

	entries := observed.All()
	require.Len(t, entries, 1, "a FailRequest error must produce exactly one logged entry, not be swallowed")
	assert.Contains(t, entries[0].Message, "fail")
	assert.Equal(t, "test-request-id-123", entries[0].ContextMap()["request_id"])
}
