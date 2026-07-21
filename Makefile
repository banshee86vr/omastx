SHELL := /bin/bash
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
REGISTRY ?= ghcr.io/banshee86vr
COMPOSE_DEV := docker compose -f deploy/docker-compose.dev.yml
COMPOSE_FULL := docker compose -f deploy/docker-compose.yml

.PHONY: dev dev-local dev-db dev-db-down dev-backend dev-frontend test lint build migrate sqlc docker-build helm-lint helm-template seed

## dev: run the full stack in Docker (rebuilds images - use dev-local for day-to-day coding)
dev:
	OMASTX_DEV=true $(COMPOSE_FULL) up --build

## dev-local: Postgres in Docker + backend (air/go) + Vite on the host - no rebuild per change
dev-local:
	@bash scripts/dev-local.sh

## dev-db: start only Postgres for native dev (detached)
dev-db:
	$(COMPOSE_DEV) up -d --wait
	@echo "Postgres ready at localhost:5432 (DATABASE_URL in deploy/.env.example)"

## dev-db-down: stop the dev Postgres container
dev-db-down:
	$(COMPOSE_DEV) down

## dev-backend: run API on :8484 (OMASTX_DEV=true, optional deploy/.env overrides)
dev-backend:
	@set -a && export OMASTX_DEV=true && \
	export DATABASE_URL="$${DATABASE_URL:-postgres://omastx:$${POSTGRES_PASSWORD:-omastx}@localhost:5432/omastx?sslmode=disable}" && \
	{ test -f deploy/.env && . ./deploy/.env; true; } && \
	export OMASTX_DEV=true && set +a && \
	cd backend && (command -v air >/dev/null && air || go run ./cmd/omastx)

## dev-frontend: Vite dev server on :5173 (proxies /api → :8484)
dev-frontend:
	cd frontend && npm run dev

## test: run backend and frontend tests
test:
	cd backend && go test ./...
	cd frontend && npm test --if-present

## lint: lint backend and frontend
lint:
	cd backend && go vet ./... && test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	cd backend && if command -v golangci-lint >/dev/null; then golangci-lint run; else echo "golangci-lint not installed, skipped"; fi
	cd frontend && npm run lint && npm run typecheck

## build: build backend binary and frontend bundle
build:
	cd backend && go build -o bin/omastx ./cmd/omastx
	cd frontend && npm run build

## migrate: apply migrations against DATABASE_URL (also applied automatically at startup)
migrate:
	cd backend && go run ./cmd/omastx -migrate-only

## seed: load 3 realistic demo clusters for UI review (idempotent)
seed:
	@set -a && export OMASTX_DEV=true && \
	export DATABASE_URL="$${DATABASE_URL:-postgres://omastx:$${POSTGRES_PASSWORD:-omastx}@localhost:5432/omastx?sslmode=disable}" && \
	{ test -f deploy/.env && . ./deploy/.env; true; } && \
	export OMASTX_DEV=true && set +a && \
	cd backend && go run ./cmd/seed

## sqlc: regenerate typed queries from backend/internal/store
sqlc:
	cd backend && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

## docker-build: build both container images locally
docker-build:
	docker build -f deploy/Dockerfile.backend  -t $(REGISTRY)/omastx-backend:$(GIT_SHA) .
	docker build -f deploy/Dockerfile.frontend -t $(REGISTRY)/omastx-frontend:$(GIT_SHA) .

## helm-lint: lint the Helm chart
helm-lint:
	helm lint deploy/chart/omastx --set existingSecret=omastx

## helm-template: render the chart with external-db and test-install values
helm-template:
	helm template omastx deploy/chart/omastx --set existingSecret=omastx >/dev/null
	helm template omastx deploy/chart/omastx --set existingSecret=omastx \
		--set postgres.internal.enabled=true \
		--set ingress.enabled=true --set ingress.host=omastx.example.com >/dev/null
	@echo "helm template OK"
