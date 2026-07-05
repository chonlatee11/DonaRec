// Package testutil provides shared test infrastructure for donnarec-api integration tests.
package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// chromeCDPPort is the Chrome DevTools Protocol port exposed by the
// chromedp/headless-shell base image (verified in 04-02 Task 1 build:
// `docker inspect` shows ExposedPorts 9222/tcp, 9223/tcp).
const chromeCDPPort = "9222/tcp"

// StartChrome starts the chrome sidecar (docker/chrome.Dockerfile:
// chromedp/headless-shell:stable + fonts-thai-tlwg, Phase 4 D-58) via
// testcontainers-go and returns a CDP WebSocket URL usable by
// chromedp.NewRemoteAllocator (internal/pdf, 04-03), plus a cleanup func.
//
// Builds from the same docker/chrome.Dockerfile used by docker-compose.yml's
// `chrome` service (Context: repo root relative to this package, mirroring
// SetupTestPostgres's "../../migrations" relative-path convention) so tests
// exercise the identical Thai-font-equipped image, not a bare upstream one.
//
// Mirrors SetupTestPostgres/NewKeycloakTestServer's shape: t.Helper(),
// require.NoError guards, t.Cleanup-registered teardown.
//
// Skips the test (t.Skip) when Docker is unavailable, since building this
// sidecar's own image (rather than pulling a pre-built module image like
// testcontainers-go/modules/postgres) is a heavier local-dev precondition
// than the existing Postgres/Keycloak test helpers assume.
//
// Usage in test files:
//
//	wsURL, cleanup := testutil.StartChrome(t)
//	defer cleanup()
//	allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
func StartChrome(t *testing.T) (wsURL string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	if !dockerAvailable(ctx) {
		t.Skip("testutil.StartChrome: Docker not available, skipping chrome sidecar test")
		return "", func() {}
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			// Repo root (donnarec-api/) relative to this package's directory,
			// same relative-path convention as SetupTestPostgres's
			// "file://../../migrations".
			Context:    "../..",
			Dockerfile: "docker/chrome.Dockerfile",
		},
		ExposedPorts: []string{chromeCDPPort},
		WaitingFor: wait.ForHTTP("/json/version").
			WithPort(chromeCDPPort).
			WithStartupTimeout(90 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start chrome sidecar container")

	// sync.Once guards against a double-Terminate warning: callers may invoke
	// the returned cleanup func explicitly (e.g. `defer cleanup()`) AND it is
	// also registered via t.Cleanup below, so both paths are safe to call.
	var once sync.Once
	cleanupFn := func() {
		once.Do(func() {
			if err := container.Terminate(ctx); err != nil {
				t.Logf("warning: failed to terminate chrome container: %v", err)
			}
		})
	}
	t.Cleanup(cleanupFn)

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get chrome container host")

	mappedPort, err := container.MappedPort(ctx, chromeCDPPort)
	require.NoError(t, err, "failed to get chrome container mapped CDP port")

	// Resolve the actual browser-level CDP WebSocket endpoint (not just "the
	// HTTP port is open") by querying /json/version — this is exactly what
	// chromedp.NewRemoteAllocator expects as its target URL, and mirrors how
	// production wiring (internal/pdf, 04-03) will connect to the
	// docker-compose `chrome` service.
	wsURL, err = resolveBrowserWSURL(ctx, host, mappedPort.Port())
	require.NoError(t, err, "failed to resolve chrome CDP websocket URL")

	return wsURL, cleanupFn
}

// resolveBrowserWSURL queries the Chrome DevTools /json/version HTTP endpoint
// and returns the advertised webSocketDebuggerUrl (the browser-level CDP
// endpoint chromedp.NewRemoteAllocator dials).
func resolveBrowserWSURL(ctx context.Context, host, port string) (string, error) {
	versionURL := fmt.Sprintf("http://%s:%s/json/version", host, port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", fmt.Errorf("testutil: build /json/version request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("testutil: GET /json/version: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("testutil: decode /json/version response: %w", err)
	}
	if payload.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("testutil: /json/version returned empty webSocketDebuggerUrl")
	}

	return payload.WebSocketDebuggerURL, nil
}

// dockerAvailable does a fast, side-effect-free check that a Docker daemon is
// reachable, so StartChrome can skip cleanly on machines/CI runners without
// Docker rather than failing with a confusing testcontainers error.
func dockerAvailable(ctx context.Context) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return exec.CommandContext(checkCtx, "docker", "info").Run() == nil
}
