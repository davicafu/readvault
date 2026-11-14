# ReadVault developer commands. Run `make help` for the list.

SHELL := /bin/bash
ENV_FILE ?= .env
# Compose v2 (`docker compose`, what Colima and current Docker install) when
# present, else the standalone v1 `docker-compose`.
# Old Docker Desktop builds ship a preview `docker compose` without --env-file
# that answers `version` with its help, so only a real version number counts.
COMPOSE_CMD ?= $(shell docker compose version --short 2>/dev/null | grep -Eq '^v?([2-9]|[1-9][0-9])\.' && echo 'docker compose' || echo docker-compose)
COMPOSE := $(COMPOSE_CMD) -f deploy/compose.yaml --env-file $(ENV_FILE)
# Where the database runs: docker (default) starts the compose Postgres;
# external uses your own PostgreSQL 15 or later at DATABASE_URL (from the env
# file) and needs no Docker, for example: make dev DB=external
DB ?= docker
ifeq ($(DB),docker)
DB_READY := db-up
else ifeq ($(DB),external)
DB_READY := db-external
else
$(error DB must be docker or external, not "$(DB)")
endif
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

.PHONY: docker-check
docker-check: ## Check that a Docker engine (Colima or Docker Desktop) is running
	@docker info >/dev/null 2>&1 || { \
		echo 'No Docker engine is running. Start Colima with: colima start'; \
		echo '(or open Docker Desktop). Install Colima with: brew install colima docker docker-compose'; \
		exit 1; \
	}

.PHONY: db-up
db-up: $(ENV_FILE) docker-check ## Start Postgres and wait until it is healthy
	$(COMPOSE) up -d db
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' $$($(COMPOSE) ps -q db))" = "healthy" ]; do sleep 1; done
	@echo "postgres is ready"

.PHONY: db-external
db-external: $(ENV_FILE) ## Check that DATABASE_URL names your own Postgres (DB=external)
	@test -n "$$DATABASE_URL" || { echo 'DB=external needs DATABASE_URL in $(ENV_FILE), pointing at PostgreSQL 15 or later.'; exit 1; }
	@echo "using the database at DATABASE_URL; Docker is not used"

.PHONY: db-down
db-down: ## Stop Postgres (data is kept in the db-data volume)
	$(COMPOSE) down

.PHONY: migrate
migrate: $(DB_READY) ## Apply pending database migrations
	cd $(API_DIR) && go run ./cmd/migrate up

.PHONY: migrate-down
migrate-down: $(DB_READY) ## Roll back the last migration
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
dev: node-check migrate $(WEB_DIR)/node_modules ## Run Postgres (unless DB=external), migrations, the API on :8080 and the web app on :3000
	@trap 'kill 0' EXIT; \
	(cd $(API_DIR) && go run ./cmd/api) & \
	(cd $(WEB_DIR) && npm run dev) & \
	wait

.PHONY: start
start: node-check migrate build ## Build and run the app as in production: the compiled API on :8080 and the built web app on :3000
	@trap 'kill 0' EXIT; \
	$(API_DIR)/bin/api & \
	(cd $(WEB_DIR) && npm start) & \
	wait

.PHONY: build
build: node-check $(WEB_DIR)/node_modules ## Build the API binaries and the production web app
	cd $(API_DIR) && go build -o bin/api ./cmd/api && go build -o bin/migrate ./cmd/migrate
	cd $(WEB_DIR) && npm run build

.PHONY: test
test: ## Run unit tests (no database needed)
	cd $(API_DIR) && go test -race ./...

.PHONY: test-integration
test-integration: $(DB_READY) ## Run integration tests against the compose Postgres (or DATABASE_URL with DB=external)
	cd $(API_DIR) && go test -race -tags=integration -p 1 ./...

.PHONY: lint
lint: node-check $(WEB_DIR)/node_modules ## Check formatting, run static analysis and typecheck the web app
	@cd $(API_DIR) && test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run gofmt -w"; exit 1; }
	cd $(API_DIR) && go vet ./... && go vet -tags=integration ./...
	@if command -v golangci-lint >/dev/null; then cd $(API_DIR) && golangci-lint run ./...; else echo "golangci-lint not installed, skipped"; fi
	cd $(WEB_DIR) && npm run typecheck
