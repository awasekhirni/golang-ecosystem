# Hexagonal Go Architecture

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

A production-grade REST API scaffold in Go that ports the original Python
hexagonal-architecture generator to a fully wired, world-class implementation.
Every layer is in place, every gap from the brief is closed, and every line is
instrumented with the **12-layer logging matrix**.

> Status: builds clean (`go build ./...`), passes `go vet ./...`, passes
> `go test -short ./...`. Integration tests need a running PostgreSQL
> (`go test -tags=integration ./...`).

---

## Table of contents

1. [What you get](#1-what-you-get)
2. [Architecture at a glance](#2-architecture-at-a-glance)
3. [The 20 closed gaps](#3-the-20-closed-gaps)
4. [The 12-layer logging matrix](#4-the-12-layer-logging-matrix)
5. [Project layout](#5-project-layout)
6. [Step-by-step: run locally](#6-step-by-step-run-locally)
7. [Step-by-step: run with Docker](#7-step-by-step-run-with-docker)
8. [Step-by-step: generate code for your own schema](#8-step-by-step-generate-code-for-your-own-schema)
9. [Step-by-step: run the test suite](#9-step-by-step-run-the-test-suite)
10. [Step-by-step: ship a release](#10-step-by-step-ship-a-release)
11. [API reference](#11-api-reference)
12. [Configuration reference](#12-configuration-reference)
13. [CMMI Level 5 Process Asset Library](#13-cmmi-level-5-process-asset-library)
14. [Extending the codebase](#14-extending-the-codebase)
15. [Troubleshooting](#15-troubleshooting)

---

## 1. What you get

- A complete Go port of the Python hexagonal generator (see
  `cmd/generator`), which introspects a live PostgreSQL schema and emits a
  full hexagonal bundle (entity, repository, service, controller, DTO,
  mapper, validation) **as Go source** for every table.
- A reference end-to-end implementation of the `User` aggregate so you can
  see how every pattern composes in practice.
- Every cross-cutting concern a production service needs: structured
  logging, request ids, JWT + API key + RBAC, audit log, Prometheus
  metrics, liveness/readiness probes, graceful shutdown, env validation,
  cursor+offset pagination, HATEOAS, Unit of Work, Specification, Value
  Objects, Domain Events, Enterprise Bus, Enterprise Repository base.
- A complete DevOps stack: Dockerfile (multi-stage, distroless),
  docker-compose (postgres + migrate + app), GitHub Actions CI
  (lint+test+integration+docker), embedded SQL migrations.
- A CMMI Level 5 Process Asset Library (11 docs) under `docs/cmmi/`.

## 2. Architecture at a glance

```
HTTP ─▶ Middleware ─▶ Controller ─▶ Service ─▶ Repository ─▶ PostgreSQL
                                          │            ▲
                                          ▼            │
                                       UoW + Events ───┘
                                          │
                                          ▼
                                      Enterprise Bus ─▶ Handlers
```

See [`docs/architecture/README.md`](docs/architecture/README.md) for the
full diagram and the layering rules enforced by `depguard`.

## 3. The 20 closed gaps (first pass)

| # | Gap | Where it lives |
|---|-----|----------------|
| 1  | JWT auth + API key auth + RBAC | `internal/middleware/auth.go`, `internal/middleware/rbac.go` |
| 2  | Request ID / correlation ID    | `internal/middleware/middleware.go` (RequestID), `internal/logger/logger.go` |
| 3  | Structured JSON logging with context | `internal/logger/logger.go` (zerolog, 12 layers) |
| 4  | Audit log for all mutations    | `internal/middleware/audit.go` (SHA-256 body digest) |
| 5  | Prometheus metrics endpoint    | `pkg/metrics/metrics.go`, mounted at `/metrics` |
| 6  | Liveness vs readiness probes   | `internal/server/server.go` (`/healthz/*`) |
| 7  | Standardized error envelope    | `internal/httpx/envelope.go` |
| 8  | Cursor + offset pagination envelope | `internal/httpx/pagination.go` |
| 9  | HATEOAS links                  | `internal/httpx/links.go`, `internal/httpx/response.go` |
| 10 | Unit of Work (transactions)    | `internal/domain/uow/uow.go` |
| 11 | Specification pattern (queries) | `internal/domain/specification/spec.go` |
| 12 | Domain Events (proper DDD)     | `internal/domain/events/event.go` |
| 13 | Value Objects for IDs          | `internal/domain/valueobject/id.go` |
| 14 | Graceful shutdown with drain   | `internal/server/server.go` (Run) |
| 15 | Env validation (fail fast)     | `internal/config/config.go` (envconfig) |
| 16 | Dockerfile + docker-compose    | `Dockerfile`, `docker-compose.yml` |
| 17 | Migration runner script        | `cmd/migrate/main.go`, `internal/database/migration.go` |
| 18 | CI/CD (GitHub Actions)         | `.github/workflows/ci.yml` |
| 19 | Test factories + integration tests | `tests/factory.go`, `tests/user_*_test.go` |
| 20 | Per-method logging with timings | `internal/domain/repository/base.go` (Observe) |

## 3a. The 18 world-class additions (second pass)

See [`docs/ENHANCEMENTS.md`](docs/ENHANCEMENTS.md) for step-by-step
instructions for each.

| # | Enhancement | Where it lives |
|---|-------------|----------------|
| A1 | **OpenAPI 3.1 spec + Swagger UI + ReDoc** | `internal/openapi/`, served at `/docs/*`, `/openapi.{json,yaml}` |
| A2 | **GraphQL API alongside REST** | `internal/graphql/`, served at `/graphql` + GraphiQL playground |
| A3 | **OpenTelemetry distributed tracing** | `internal/infrastructure/otel/`, traceparent W3C propagation |
| A4 | **Idempotency-Key middleware** (24h dedup) | `internal/middleware/idempotency.go` |
| A5 | **ETag + conditional requests** (RFC 7232) | `internal/middleware/etag.go` |
| A6 | **Security headers** (OWASP A05:2021) | `internal/middleware/security.go` |
| A7 | **Transactional Outbox pattern** | `internal/infrastructure/outbox/`, migration 000002 |
| A8 | **Resilience: circuit breaker + retry + bulkhead** | `internal/resilience/resilience.go` |
| A9 | **PII redaction in logs** (GDPR) | `internal/redaction/redaction.go` |
| A10 | **Composite health subsystem** (startup/live/ready/deep) | `internal/health/health.go` |
| A11 | **pprof profiling endpoints** | `internal/server/server.go` (flag-gated) |
| A12 | **Compression + body size limit** | `internal/middleware/compression.go`, `security.go` |
| A13 | **API versioning + Deprecation/Sunset headers** | `internal/versioning/versioning.go` |
| A14 | **Feature flags** (env-driven) | `internal/features/features.go` |
| A15 | **Background job runner** (cron + at-least-once) | `internal/jobs/runner.go` |
| A16 | **Fuzz tests** (Go 1.18+ native) | `tests/fuzz_test.go` |
| A17 | **k6 load tests + SLO/SLI** | `scripts/k6/`, `docs/architecture/slo_sli.md` |
| A18 | **SBOM generation** (CycloneDX, EO 14028) | `scripts/sbom/sbom.sh` |

## 4. The 12-layer logging matrix

Every log line carries a `layer` field. See
[`docs/architecture/logging_matrix.md`](docs/architecture/logging_matrix.md)
for the full table and the query recipes.

| Layer | What it logs | Source |
|-------|--------------|--------|
| `http` | method, path, status, durationMs, bytes | `middleware.HTTPLogger` |
| `middleware` | auth outcome, rate-limit, rbac, audit | `internal/middleware/*.go` |
| `controller` | action, marker (IN/OUT) | `internal/api/controllers/user/controller.go` |
| `validation` | field, value, message | `internal/api/validations/user/validation.go` |
| `mapper` | entity↔dto conversion | `internal/api/mappers/user/mapper.go` |
| `service` | operation, marker, timing | `internal/application/services/user/service.go` |
| `repository` | entity, op, rowsAffected, durationMs | `internal/domain/repository/base.go` |
| `database` | pool lifecycle, slow-query (>100ms) | `internal/database/postgres.go`, `repository/base.go` |
| `domain_event` | topic, eventId, traceId, userId | `internal/domain/events/event.go` |
| `enterprise_bus` | topic, handler, success/failure | `internal/domain/bus/bus.go` |
| `errors` | code, message, httpStatus, stack | `internal/httpx/envelope.go` |
| `lifecycle` | boot, shutdown, signal, drain | `internal/server/server.go` |

## 5. Project layout

```
hexagonal-go/
├── cmd/
│   ├── server/        HTTP server entrypoint (graceful shutdown, probes)
│   ├── migrate/       standalone migration runner
│   └── generator/     Go port of the Python generator (introspects PG)
├── internal/
│   ├── config/        env validation (fail-fast)
│   ├── logger/        12-layer structured logger (zerolog)
│   ├── database/      pgx pool + embedded migration runner
│   ├── server/        chi router, middleware stack, probes, /metrics
│   ├── middleware/    Recover, RequestID, HTTPLogger, CORS, RateLimit,
│   │                  Timeout, APIKey, JWT, RBAC, Audit
│   ├── httpx/         error+success envelopes, pagination, HATEOAS
│   ├── domain/
│   │   ├── entities/user/        reference aggregate + events + ports
│   │   ├── valueobject/          typed IDs, Email, Slug
│   │   ├── specification/        And/Or/Not + FieldEq/FieldLike/...
│   │   ├── uow/                  Unit of Work (post-commit event flush)
│   │   ├── events/               domain event primitives + Collector
│   │   ├── bus/                  in-memory enterprise bus
│   │   └── repository/           enterprise repository Base (Observe)
│   ├── application/services/user/   orchestration + UoW + bus
│   ├── infrastructure/
│   │   ├── repositories/user/   PostgreSQL adapter for User
│   │   ├── auth/                JWT issuer
│   │   └── (database lives in internal/database)
│   └── api/
│       ├── controllers/user/    thin HTTP handlers
│       ├── dtos/user/           wire-format structs
│       ├── mappers/user/        entity↔dto
│       └── validations/user/    per-field rules
├── pkg/metrics/      Prometheus collectors + /metrics handler
├── internal/database/migrations/   *.up.sql + *.down.sql
├── tests/            factory + unit + integration tests
├── docs/
│   ├── cmmi/         11-doc CMMI Level 5 Process Asset Library
│   └── architecture/ layer diagram + 12-layer logging matrix
├── .github/workflows/ci.yml    3-job pipeline (lint, integration, docker)
├── Dockerfile         multi-stage, distroless, non-root
├── docker-compose.yml postgres + migrate + app
├── Makefile           one-command targets for everything
├── go.mod / go.sum
└── README.md          this file
```

---

## 6. Step-by-step: run locally

### Prerequisites

- Go 1.22+ (`go version`)
- PostgreSQL 14+ reachable on `localhost:5432`
- (optional) `make`, `docker`, `docker compose`

### Steps

1. **Clone + enter the repo.**

   ```bash
   git clone <your-fork-url> hexagonal-go
   cd hexagonal-go
   ```

2. **Create the database.**

   ```bash
   psql -h localhost -U awasekhirnisyed -d postgres -c \
     "CREATE DATABASE profxdb;"
   ```
   (Replace `awasekhirnisyed` with your PG user; grant yourself privileges
   on the new DB.)

3. **Copy and edit `.env`.**

   ```bash
   cp .env.example .env
   # Edit DATABASE_URL and JWT_SECRET at minimum.
   ```

4. **Resolve dependencies.**

   ```bash
   go mod tidy
   ```

5. **Run migrations** (embedded, idempotent):

   ```bash
   go run ./cmd/migrate -direction=up
   ```
   You should see one `MIGRATION_APPLIED` line per file.

6. **Boot the server.**

   ```bash
   go run ./cmd/server
   ```
   Boot logs will print `BOOT_START` → `DB_POOL_READY` → `DB_CONNECTED` →
   `MIGRATION_APPLIED` → `BOOT_READY` → `SERVER_LISTENING` on `:8080`.

7. **Smoke-test it.**

   ```bash
   # Liveness (no auth, no DB check):
   curl -s localhost:8080/healthz
   # {"status":"alive"}

   # Readiness (no auth, pings DB):
   curl -s localhost:8080/readyz
   # {"status":"ready"}

   # Prometheus metrics:
   curl -s localhost:8080/metrics | head

   # Mint a demo JWT (no auth required):
   TOKEN=$(curl -s localhost:8080/auth/demo-token | jq -r .token)

   # Create a user (mutation: triggers Audit middleware):
   curl -s localhost:8080/api/v1/users \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"email":"alice@example.com","first_name":"Alice","last_name":"Smith"}'

   # List users (paginated, HATEOAS):
   curl -s "localhost:8080/api/v1/users?limit=5&offset=0" \
        -H "Authorization: Bearer $TOKEN" | jq

   # Or authenticate with an API key instead:
   curl -s localhost:8080/api/v1/users -H "X-API-Key: dev-key-1"
   ```

8. **Shut it down gracefully.**

   `Ctrl-C` (SIGINT) or `kill -TERM <pid>`. The server logs
   `SIGNAL_RECEIVED` → `DRAIN_START` → `DRAIN_COMPLETE` and exits 0. In
   flight requests get up to `HTTP_SHUTDOWN_TIMEOUT` (default 30s) to
   finish.

---

## 7. Step-by-step: run with Docker

```bash
docker compose up -d --build
docker compose logs -f app        # tail the app logs
curl -s localhost:8080/healthz    # {"status":"alive"}
docker compose down               # stop and clean up (use -v to drop pgdata)
```

The compose file spins up:
- `postgres` (PG 16, health-checked with `pg_isready`)
- `migrate` (one-shot, runs `cmd/migrate -direction=up` then exits 0)
- `app` (depends on both, exposes `:8080`)

---

## 8. Step-by-step: generate code for your own schema

The `cmd/generator` is the Go port of the original Python script. It
introspects a live PostgreSQL schema and emits a hexagonal bundle per table
to a directory you choose.

```bash
# 1. Point it at any DB you want to scaffold against:
go run ./cmd/generator \
  -dsn="postgres://user:pass@localhost:5432/yourdb?sslmode=disable" \
  -out=./generated \
  -schema=public

# 2. Inspect the output:
ls generated/<table_name>/
# <table>_entity.go  <table>_repository.go  <table>_service.go
# <table>_controller.go  <table>_dto.go  <table>_mapper.go  <table>_validation.go

# 3. Drop the generated files into the project under the corresponding
#    internal/.../<table>/ subdirectories (the generator output uses
#    placeholder import paths that you adjust during integration).
```

What the generator emits (per table):
- `entity` — domain struct + Repository port + domain events.
- `repository` — PostgreSQL adapter embedding `repository.Base`
  (auto-inherits timing, metrics, slow-query WARN, rows-affected).
- `service` — application-layer facade with SERVICE_IN/OUT markers and
  UoW + bus wiring.
- `controller` — thin HTTP handlers, one per CRUD op, mounted via
  `Register(chi.Router)`.
- `dto`, `mapper`, `validation` — wire format + translation + rules.

---

## 9. Step-by-step: run the test suite

```bash
# Unit tests (no DB needed, fast):
make test
# equivalent: go test -race -count=1 -short ./...

# Integration tests (need DATABASE_URL pointing at a writable PG):
export DATABASE_URL="postgres://awasekhirnisyed:pass@localhost:5432/profxdb?sslmode=disable"
make test-integration
# equivalent: go test -race -count=1 -tags=integration ./tests/...

# Lint:
make lint
```

Tests live in `tests/`:
- `factory.go` — `UserBuilder` test factory.
- `user_unit_test.go` — aggregate invariants, builder, state machine.
- `user_integration_test.go` — repository round-trip, optimistic
  concurrency, bus dispatch (build tag `integration`).

---

## 10. Step-by-step: ship a release

1. Update `CHANGELOG.md` with the semver rationale.
2. Tag the release commit:
   ```bash
   git tag -a v1.2.3 -m "release: v1.2.3"
   git push origin v1.2.3
   ```
3. CI runs the 3-job pipeline; on success the docker image is built and
   pushed (configure your registry in `.github/workflows/ci.yml`).
4. Post-deploy: run through the post-deploy gate in
   `docs/cmmi/03_PPQA.md` within 30 minutes.

---

## 11. API reference

All REST routes are behind `JWTAuth` + `APIKeyAuth` + `Require(RoleAdmin, RoleUser)`
except where noted. GraphQL shares the same auth.

| Method | Path | Body / Query | Returns |
|--------|------|--------------|---------|
| GET    | `/healthz/startup`    | —   | `{"status":"started"}` (no auth) |
| GET    | `/healthz/live`       | —   | `{"status":"alive"}` (no auth) |
| GET    | `/healthz/ready`      | —   | `{"status":"up","checks":[...]}` (no auth) |
| GET    | `/healthz/deep`       | —   | full health report (no auth) |
| GET    | `/metrics`            | —   | Prometheus exposition (no auth) |
| GET    | `/openapi.json`       | —   | OpenAPI 3.1 JSON (no auth) |
| GET    | `/openapi.yaml`       | —   | OpenAPI 3.1 YAML (no auth) |
| GET    | `/docs/swagger`       | —   | Swagger UI (no auth) |
| GET    | `/docs/redoc`         | —   | ReDoc UI (no auth) |
| GET    | `/graphql/schema`     | —   | GraphQL SDL (no auth) |
| GET    | `/graphql/playground` | —   | GraphiQL UI (no auth) |
| POST   | `/graphql`            | `{query,variables}` | GraphQL response envelope |
| GET    | `/auth/demo-token`    | —   | `{"token":"...","expiresIn":"15m0s"}` (no auth) |
| GET    | `/api/v1/users`       | `?limit=20&offset=0` or `?cursor=...&limit=20` | paged success envelope with HATEOAS links |
| POST   | `/api/v1/users`       | `{email,first_name,last_name}` | 201 Created + Location header |
| GET    | `/api/v1/users/{id}`  | —                   | single-user success envelope + ETag |
| PATCH  | `/api/v1/users/{id}/status` | `{status,reason?}` | updated user |
| DELETE | `/api/v1/users/{id}`  | —                   | 204 No Content (soft delete) |
| GET    | `/debug/pprof/*`      | —                   | pprof (feature-flagged; admin-only in prod) |

### Success envelope

```json
{
  "data": { ... },
  "meta": { "page": { "total": 137, "offset": 0, "limit": 20, "hasNext": true },
            "serverTime": "2026-10-30T12:00:00.000Z",
            "requestId": "550e8400-..." },
  "links": { "self": "...", "next": "...", "prev": "...", "create": "..." }
}
```

### Error envelope

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "validation failed",
    "details": [
      { "field": "email", "value": "bad", "message": "invalid email format" }
    ],
    "requestId": "550e8400-...",
    "traceId": ""
  }
}
```

Stable error codes: `BAD_REQUEST`, `VALIDATION_FAILED`, `UNAUTHORIZED`,
`FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `RATE_LIMITED`, `INTERNAL`,
`DATABASE_ERROR`. See `internal/httpx/envelope.go`.

---

## 12. Configuration reference

Every variable is validated at boot by envconfig. See `.env.example` for
the canonical list and `internal/config/config.go` for the validation
rules. Production-only rules enforced at boot:

- `APP.ENV=production` ⇒ `APP.DEBUG=false`
- `APP.ENV=production` ⇒ `JWT.SECRET` ≥ 32 bytes
- `HTTP.SHUTDOWN_TIMEOUT` ≥ 1s (so graceful drain actually drains)
- `DB.SLOW_QUERY_MS` ≥ 1
- `RATE.LIMIT_RPS` > 0

---

## 13. CMMI Level 5 Process Asset Library

Eleven docs under `docs/cmmi/`. Start at `00_PAL.md` for the index.

| # | Doc | CMMI PA |
|---|-----|---------|
| 00 | PAL Index | OPF |
| 01 | Organizational Standard Process (OSSP) | OPF, OPD |
| 02 | Measurement & Analysis (MA) | MA |
| 03 | Quality Gates (PPQA) | PPQA |
| 04 | Risk Management (RSKM) | RSKM |
| 05 | Configuration Management (CM) | CM |
| 06 | Causal Analysis & Resolution (CAR) | CAR |
| 07 | Organizational Innovation & Deployment (OID) | OID |
| 08 | Continuous Improvement (Optimizing loop) | OID, OPF |
| 09 | Decision Analysis & Resolution (DAR) | DAR |
| 10 | Verification & Validation (VER/VAL) | VER, VAL |

---

## 14. Extending the codebase

### Add a new aggregate (e.g. `Order`)

1. Run the generator: `go run ./cmd/generator -dsn=... -out=./generated` —
   or hand-write the seven files modeled on the `user` aggregate.
2. Move each generated file under the matching `internal/.../<name>/`
   directory and fix the import paths.
3. Add a migration `internal/database/migrations/<seq>_create_<name>.up.sql`.
4. Wire the new service + controller in `internal/server/server.go::Build`
   and `New`.
5. Add an integration test under `tests/`.

### Add a new domain event handler

```go
bus.Subscribe(events.HandlerFunc{
    TopicStr: userent.UserCreated{}.Topic(),
    Fn: func(ctx context.Context, e events.Event) error {
        // ... do work ...
        return nil
    },
})
```

Handlers run in their own goroutines; panics are recovered and counted by
`bus_dispatch_errors_total`.

### Add a new middleware

Implement `func(http.Handler) http.Handler`, then `r.Use(it)` in
`internal/server/server.go::New`. Order matters: place auth-aware
middleware AFTER `RequestID` so logs have the correlation id.

---

## 15. Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Boot fails with `env validation failed` | Missing env var | `cp .env.example .env`, fill values, `go run ./cmd/server` |
| Boot fails with `DB_PING_FAILED_AFTER_RETRIES` | DB not reachable / wrong DSN | `psql <DSN>`, check firewall, check `pg_isready` |
| `401 UNAUTHORIZED` on every `/api/v1/*` call | No JWT, no API key, expired token | Call `/auth/demo-token` first, or send `X-API-Key: dev-key-1` |
| `429 RATE_LIMITED` bursts | `RATE.LIMIT_RPS` too low for the client | Raise in `.env`, restart |
| `/readyz` returns 503 during deploy | Server is draining (SIGTERM received) | Wait for the old pod to terminate; new pod will be ready |
| `level=warn layer=database msg="SLOW_QUERY"` | A query exceeded `DB.SLOW_QUERY_MS` | Inspect the `op` field; add an index or rewrite the query |
| `level=error layer=enterprise_bus msg="BUS_HANDLER_ERROR"` | A domain-event handler returned an error | Inspect the `handler` + `topic` fields; fix the handler logic |
| Migration not applied | File not under `internal/database/migrations/*.up.sql` | Move it; the embed glob is `migrations/*.sql` |
| Generated code does not compile | The generator output uses placeholder import paths | Adjust the `"<name>domain"` / `"<name>svc"` / `"<name>infra"` / `"<name>dto"` / `"<name>ctrl"` / `"<name>mapper"` / `"<name>validation"` paths to real module paths |

---


# end to end verification 
cd hexagonal-go
cp .env.example .env       # fill DATABASE_URL + JWT_SECRET
make migrate-up && make run &
sleep 3
BASE_URL=http://localhost:8080 ./scripts/smoke_test.sh
# Expected: === Summary: 38 passed, 0 failed ===

*Copyright 2025 β ORI Inc. Canada. All Rights Reserved. Author: Awase Khirni Syed.*
