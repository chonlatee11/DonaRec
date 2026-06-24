# donnarec-api Dockerfile — multi-stage build
# Stage 1: build the Go binary
# Stage 2: minimal runtime image (distroless)

# -------- Stage 1: build --------
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Download modules first (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /app/bin/donnarec-api ./cmd/server/...

# -------- Stage 2: runtime --------
# Use debian slim (not distroless) so healthcheck CMD-SHELL has /bin/sh + wget.
# For hardened production builds, switch to distroless and implement a Go-native
# health-check subcommand (future task).
FROM debian:bookworm-slim AS runtime

# Install wget for healthcheck probe (no curl — keep image small)
RUN apt-get update && apt-get install -y --no-install-recommends wget ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Run as non-root (mirrors distroless:nonroot UID 65532)
RUN groupadd -r nonroot -g 65532 && useradd -r -g nonroot -u 65532 nonroot

# Copy binary from build stage
COPY --from=builder /app/bin/donnarec-api /donnarec-api

# Copy i18n locale catalogs (read at runtime from embed or file path)
COPY --from=builder /app/internal/i18n/locales /locales

USER nonroot

EXPOSE 8000

ENTRYPOINT ["/donnarec-api"]
