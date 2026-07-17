# Deferred Items — Phase 06

Out-of-scope discoveries logged during execution of quick task 260717-spx (fix
public-endpoint security flags), per the SCOPE BOUNDARY rule (only auto-fix
issues directly caused by the current task's changes).

## 2026-07-17 — TestE2E_MakerCheckerIssuancePipeline: chrome sidecar container build fails

**Symptom:** `go test ./...` at `donnarec-api` fails one test —
`cmd/server.TestE2E_MakerCheckerIssuancePipeline` — with:

```
create container: build image: NotFound: content digest
sha256:d96f262a7537d8f4687e03c6a3c0d7ef942a88de44a4dbf2fc4f919fcb3b5bd9: not found
Messages: failed to start chrome sidecar container
```

**Root cause (environmental, not code):** `testutil.StartChrome`
(`internal/testutil/chrome.go:74`) builds `docker/chrome.Dockerfile` via
testcontainers `FromDockerfile` on every run. A locally cached image
(`donnarec-api-chrome:latest`, 13 days old) exists, but the build step still
checks the upstream base-image (`chromedp/headless-shell:stable`) manifest
against the registry and fails to resolve a content digest — likely a stale
local layer cache entry or restricted registry egress in this environment.
`docker images` confirms the base image and prior build output are present
locally; this is a registry-resolution/build-cache issue, not a code defect.

**Why out of scope:** This task (260717-spx) touches only
`internal/config/config.go`, `cmd/server/main.go`,
`cmd/server/e2e_test.go`/`e2e_public_test.go` (trusted-proxies +
multipart body-limit fixes). `TestE2E_MakerCheckerIssuancePipeline` exercises
the Admin settings PDF-preview path via a real Chrome sidecar and has no
dependency on the rate-limit/captcha/trusted-proxy seam this task fixes.
`TestPublicDonationE2E` (the test suite this task's verification requires)
passes fully, including under `-race` (6/6 subtests).

**Recommended follow-up:** `docker pull chromedp/headless-shell:stable`
(force a fresh manifest pull) or `docker builder prune` to clear the stale
layer-cache entry, then re-run `go test ./cmd/server/... -run
TestE2E_MakerCheckerIssuancePipeline`. Not fixed in this task per the
SCOPE BOUNDARY rule (pre-existing, unrelated to this task's changes).
