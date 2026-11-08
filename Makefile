# ReadVault developer commands. Run `make help` for the list.

SHELL := /bin/bash
COMPOSE := docker-compose -f deploy/compose.yaml --env-file .env
API_DIR := services/api
WEB_DIR := apps/web

# Load .env when present so DATABASE_URL and friends reach the Go commands.
ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-18s %s\n", $$1, $$2}'

.env:
	@echo ".env not found: copy .env.example to .env and set POSTGRES_PASSWORD" && exit 1

.PHONY: db-up
db-up: .env ## Start Postgres and wait until it is healthy
	$(COMPOSE) up -d db
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' $$($(COMPOSE) ps -q db))" = "healthy" ]; do sleep 1; done
	@echo "postgres is ready"

.PHONY: db-down
db-down: ## Stop Postgres (data is kept in the db-data volume)
	$(COMPOSE) down

.PHONY: migrate
migrate: db-up ## Apply pending database migrations
	cd $(API_DIR) && go run ./cmd/migrate up

.PHONY: migrate-down
migrate-down: db-up ## Roll back the last migration
	cd $(API_DIR) && go run ./cmd/migrate down

.PHONY: web-install
web-install: ## Install the web app dependencies from the lockfile
	cd $(WEB_DIR) && npm ci

$(WEB_DIR)/node_modules:
	cd $(WEB_DIR) && npm ci

.PHONY: dev
dev: migrate $(WEB_DIR)/node_modules ## Run Postgres, migrations, the API on :8080 and the web app on :3000
	@trap 'kill 0' EXIT; \
	(cd $(API_DIR) && go run ./cmd/api) & \
	(cd $(WEB_DIR) && npm run dev) & \
	wait

.PHONY: build
build: $(WEB_DIR)/node_modules ## Build the API binaries and the production web app
	cd $(API_DIR) && go build -o bin/api ./cmd/api && go build -o bin/migrate ./cmd/migrate
	cd $(WEB_DIR) && npm run build

.PHONY: test
test: ## Run unit tests (no database needed)
	cd $(API_DIR) && go test -race ./...

.PHONY: test-integration
test-integration: db-up ## Run integration tests against the compose Postgres
	cd $(API_DIR) && go test -race -tags=integration -p 1 ./...

.PHONY: lint
lint: $(WEB_DIR)/node_modules ## Check formatting, run static analysis and typecheck the web app
	@cd $(API_DIR) && test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run gofmt -w"; exit 1; }
	cd $(API_DIR) && go vet ./... && go vet -tags=integration ./...
	@if command -v golangci-lint >/dev/null; then cd $(API_DIR) && golangci-lint run ./...; else echo "golangci-lint not installed, skipped"; fi
	cd $(WEB_DIR) && npm run typecheck
