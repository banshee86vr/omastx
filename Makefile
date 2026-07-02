SHELL := /bin/bash
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
REGISTRY ?= ghcr.io/banshee86vr

.PHONY: dev test lint build migrate sqlc docker-build helm-lint helm-template seed

## dev: run the full local stack (postgres + backend + frontend) with docker compose
dev:
	docker compose -f deploy/docker-compose.yml up --build

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
