# ReadVault developer commands. Run `make help` for the list.

SHELL := /bin/bash
COMPOSE := docker-compose -f deploy/compose.yaml --env-file .env
API_DIR := services/api
WEB_DIR := services/web

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

.PHONY: dev
dev: db-up ## Run Postgres and the API (and the web app when present)
	@if [ -d $(WEB_DIR) ]; then \
		trap 'kill 0' EXIT; \
		(cd $(API_DIR) && go run ./cmd/api) & \
		(cd $(WEB_DIR) && npm run dev) & \
		wait; \
	else \
		cd $(API_DIR) && go run ./cmd/api; \
	fi

.PHONY: test
test: ## Run unit tests (no database needed)
	cd $(API_DIR) && go test -race ./...
	@if [ -d $(WEB_DIR) ]; then cd $(WEB_DIR) && npm test; fi

.PHONY: test-integration
test-integration: db-up ## Run integration tests against the compose Postgres
	cd $(API_DIR) && go test -race -tags=integration -p 1 ./...

.PHONY: lint
lint: ## Check formatting and run static analysis
	@cd $(API_DIR) && test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run gofmt -w"; exit 1; }
	cd $(API_DIR) && go vet ./... && go vet -tags=integration ./...
	@if command -v golangci-lint >/dev/null; then cd $(API_DIR) && golangci-lint run ./...; else echo "golangci-lint not installed, skipped"; fi
	@if [ -d $(WEB_DIR) ]; then cd $(WEB_DIR) && npm run lint && npm run typecheck; fi
