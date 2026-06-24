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
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

# Copy binary from build stage
COPY --from=builder /app/bin/donnarec-api /donnarec-api

# Copy i18n locale catalogs (read at runtime from embed or file path)
COPY --from=builder /app/internal/i18n/locales /locales

EXPOSE 8000

ENTRYPOINT ["/donnarec-api"]
