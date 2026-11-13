# ReadVault developer commands. Run `make help` for the list.

SHELL := /bin/bash
ENV_FILE ?= .env
COMPOSE := docker-compose -f deploy/compose.yaml --env-file $(ENV_FILE)
API_DIR := services/api
WEB_DIR := apps/web

# Load the env file so DATABASE_URL and friends reach the Go commands. When it
# is missing, make creates it with the rule below and restarts.
-include $(ENV_FILE)
export

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-18s %s\n", $$1, $$2}'

# First run: a local development env file with a random database password.
$(ENV_FILE):
	@pw=$$(LC_ALL=C tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c 32) && umask 077 && printf '%s\n' \
		'# Created by make on first run, for local development only. Not committed.' \
		"POSTGRES_PASSWORD=$$pw" \
		"DATABASE_URL=postgres://readvault:$$pw@localhost:5432/readvault?sslmode=disable" \
		'API_ORIGIN=http://127.0.0.1:8080' > $@
	@echo "created $@ with a random database password"

.PHONY: db-up
db-up: $(ENV_FILE) ## Start Postgres and wait until it is healthy
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

.PHONY: node-check
node-check: ## Check that Node.js 22 or later is first on PATH
	@major=$$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0); \
	if [ "$$major" -lt 22 ]; then \
		echo "Node.js 22 or later is required; found $$(node --version 2>/dev/null || echo none) at $$(command -v node || echo 'no node')."; \
		echo 'Put a newer node first on PATH, for example: export PATH="$$HOME/.local/node/bin:$$PATH"'; \
		exit 1; \
	fi

.PHONY: web-install
web-install: node-check ## Install the web app dependencies from the lockfile
	cd $(WEB_DIR) && npm ci

# Installed on the first run and again whenever the lockfile changes (after a
# pull that adds a dependency); the touch marks the install as current.
$(WEB_DIR)/node_modules: $(WEB_DIR)/package-lock.json | node-check
	cd $(WEB_DIR) && npm ci
	@touch $@

.PHONY: dev
dev: node-check migrate $(WEB_DIR)/node_modules ## Run Postgres, migrations, the API on :8080 and the web app on :3000
	@trap 'kill 0' EXIT; \
	(cd $(API_DIR) && go run ./cmd/api) & \
	(cd $(WEB_DIR) && npm run dev) & \
	wait

.PHONY: build
build: node-check $(WEB_DIR)/node_modules ## Build the API binaries and the production web app
	cd $(API_DIR) && go build -o bin/api ./cmd/api && go build -o bin/migrate ./cmd/migrate
	cd $(WEB_DIR) && npm run build

.PHONY: test
test: ## Run unit tests (no database needed)
	cd $(API_DIR) && go test -race ./...

.PHONY: test-integration
test-integration: db-up ## Run integration tests against the compose Postgres
	cd $(API_DIR) && go test -race -tags=integration -p 1 ./...

.PHONY: lint
lint: node-check $(WEB_DIR)/node_modules ## Check formatting, run static analysis and typecheck the web app
	@cd $(API_DIR) && test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run gofmt -w"; exit 1; }
	cd $(API_DIR) && go vet ./... && go vet -tags=integration ./...
	@if command -v golangci-lint >/dev/null; then cd $(API_DIR) && golangci-lint run ./...; else echo "golangci-lint not installed, skipped"; fi
	cd $(WEB_DIR) && npm run typecheck
