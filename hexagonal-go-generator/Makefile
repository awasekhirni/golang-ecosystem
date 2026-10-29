# =============================================================================
# Makefile - Hexagonal Go Architecture
# Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
# Author: Awase Khirni Syed
# =============================================================================

APP_NAME   := hexagonal-go
BIN_DIR    := bin
PKG        := github.com/ori-inc/hexagonal-go

GO         := go
GOFLAGS    := -trimpath -ldflags "-s -w"

.PHONY: all tidy build run test test-integration lint vet fmt migrate-up migrate-down generate docker-up docker-down docker-build ci clean help

all: tidy build

## ---------- Development ----------
tidy:
	$(GO) mod tidy

run:
	$(GO) run ./cmd/server

build: tidy
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/server   ./cmd/server
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/migrate  ./cmd/migrate
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/generate ./cmd/generator
	@echo "Build complete -> $(BIN_DIR)/"

## ---------- Quality gates ----------
fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

test:
	$(GO) test -race -count=1 -short ./...

test-integration:
	$(GO) test -race -count=1 -tags=integration ./tests/...

## ---------- Database ----------
migrate-up:
	$(GO) run ./cmd/migrate -direction=up

migrate-down:
	$(GO) run ./cmd/migrate -direction=down

migrate-new:
	@read -p "Migration name: " name; \
	ts=$$(date +%Y%m%d%H%M%S); \
	mkdir -p migrations; \
	touch migrations/$${ts}_$${name}.up.sql; \
	touch migrations/$${ts}_$${name}.down.sql; \
	echo "Created migrations/$${ts}_$${name}.{up,down}.sql"

## ---------- Code generation (mirrors the Python script) ----------
generate:
	$(GO) run ./cmd/generator

## ---------- Containerization ----------
docker-build:
	docker build -t $(APP_NAME):latest .

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down -v

## ---------- CI ----------
ci: fmt lint test test-integration
	@echo "CI pipeline complete"

clean:
	rm -rf $(BIN_DIR)
	$(GO) clean -testcache

help:
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*##"}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
