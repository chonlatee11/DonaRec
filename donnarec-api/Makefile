# Makefile for donnarec-api
#
# CLI tools (sqlc, migrate) are installed via go install and must be on PATH.
# Run: export PATH="$PATH:$(go env GOPATH)/bin" to add them.
#
# Or invoke via: make GOBIN=$(go env GOPATH)/bin <target>
#
# Tool versions (install with go install):
#   sqlc:    go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
#   migrate: go install -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

GOPATH ?= $(shell go env GOPATH)
GOBIN  ?= $(GOPATH)/bin

.PHONY: help build test test-short test-integration lint vet sqlc-gen migrate-up migrate-down up down clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the API binary
	go build -o bin/donnarec-api ./cmd/server/...

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint (must be installed separately)
	golangci-lint run ./...

test: ## Run all tests (unit + integration; requires Docker for testcontainers)
	go test -v -count=1 ./...

test-short: ## Run only short (unit) tests; no Docker/network required
	go test -v -short -count=1 ./...

test-integration: ## Run integration tests only (requires Docker)
	go test -v -count=1 -run Integration ./... || \
	go test -v -count=1 ./internal/auth/... ./internal/users/...

sqlc-gen: ## Regenerate sqlc type-safe query code
	$(GOBIN)/sqlc generate -f internal/db/sqlc.yaml

migrate-up: ## Apply all pending DB migrations (requires DATABASE_URL env var)
	@[ -n "$$DATABASE_URL" ] || (echo "ERROR: DATABASE_URL not set" && exit 1)
	$(GOBIN)/migrate -path migrations -database "$$DATABASE_URL" up

migrate-down: ## Roll back the last DB migration
	@[ -n "$$DATABASE_URL" ] || (echo "ERROR: DATABASE_URL not set" && exit 1)
	$(GOBIN)/migrate -path migrations -database "$$DATABASE_URL" down 1

migrate-version: ## Show current migration version
	@[ -n "$$DATABASE_URL" ] || (echo "ERROR: DATABASE_URL not set" && exit 1)
	$(GOBIN)/migrate -path migrations -database "$$DATABASE_URL" version

up: ## Start local development stack (PostgreSQL + Keycloak + API)
	docker compose up -d

down: ## Stop local development stack
	docker compose down

up-build: ## Build and start local development stack
	docker compose up -d --build

logs: ## Tail compose logs
	docker compose logs -f

clean: ## Remove build artifacts
	rm -rf bin/
