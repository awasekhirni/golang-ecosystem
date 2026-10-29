# Verification & Validation Matrix

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

This document is the **verification contract** for the Hexagonal Go
service. Every feature — first-pass, second-pass, and third-pass — is
listed here with: (a) how to verify it manually, (b) which test covers it,
and (c) which log line / metric proves it's wired.

Use this as your pre-release checklist (ties into `docs/cmmi/03_PPQA.md`).

---

## Table of contents

1. [Build & static analysis](#1-build--static-analysis)
2. [Original 20 gaps](#2-original-20-gaps)
3. [18 world-class additions](#3-18-world-class-additions)
4. [Third-pass wiring + verification](#4-third-pass-wiring--verification)
5. [End-to-end smoke test](#5-end-to-end-smoke-test)
6. [Observability stack](#6-observability-stack)
7. [Production deployment](#7-production-deployment)

---

## 1. Build & static analysis

| Check | Command | Expected |
|-------|---------|----------|
| Build | `go build ./...` | exit 0, no output |
| Vet | `go vet ./...` | exit 0, no output |
| gofmt | `gofmt -l .` | no files listed |
| Lint | `golangci-lint run` | exit 0 (after installing linters) |
| Unit tests | `go test -short -count=1 ./...` | all `ok` |
| Integration tests | `go test -tags=integration -count=1 ./tests/...` (needs DATABASE_URL) | all `ok` |
| Fuzz regression | `go test -run 'FuzzParseEmail\|FuzzCursorRoundTrip' ./tests/` | `ok` |
| Binaries build | `make build` | `bin/server`, `bin/migrate`, `bin/generator` present |
| Docker build | `docker build -t hexagonal-go:ci .` | image built |

### Step-by-step

```bash
cd hexagonal-go

# 1. Static checks
go build ./...
go vet ./...
gofmt -l .

# 2. Install golangci-lint (one-time)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 3. Lint (enforces depguard layering rules)
golangci-lint run ./...

# 4. Tests
go test -short -count=1 ./...                    # unit (fast, no DB)
go test -race -count=1 -tags=integration ./tests/... # integration (needs DB)

# 5. Binaries
make build
ls -lh bin/
```

---

## 2. Original 20 gaps

For each gap: how to verify, which test covers it, the log/metric proof.

### #1 JWT + API key + RBAC

- **Verify**: `curl -s localhost:8080/api/v1/users` → 401; with `Authorization: Bearer <jwt>` → 200; with `X-API-Key: dev-key-1` → 200.
- **Test**: `TestIntegration_UnauthorizedWithoutToken`, `TestIntegration_APIKeyAuth`, `TestJWTAuth_*`, `TestRequire_*`.
- **Log**: `AUTH_OK_JWT`, `AUTH_OK_APIKEY`, `RBAC_FORBIDDEN`, `RBAC_ANONYMOUS`.

### #2 Request ID

- **Verify**: `curl -sI localhost:8080/healthz/live | grep -i x-request-id`.
- **Test**: `TestRequestID_MintsWhenAbsent`, `TestRequestID_PreservesIncoming`.
- **Log**: every line carries `requestId`.

### #3 Structured JSON logging

- **Verify**: boot with `LOG_FORMAT=json` and inspect any log line — it's valid JSON with `level`, `layer`, `time`, `message`.
- **Test**: implicit — every middleware test asserts log output shape.

### #4 Audit log for mutations

- **Verify**: `curl -X POST .../api/v1/users ...` then grep logs for `AUDIT_MUTATION` with a `bodyDigest` field.
- **Log**: `AUDIT_MUTATION`.
- **Metric**: `audit_events_total{action=...}`.

### #5 Prometheus /metrics

- **Verify**: `curl -s localhost:8080/metrics | grep http_requests_total`.
- **Test**: `TestIntegration_OpenAPI_Served` (server boots → /metrics is mounted).

### #6 Liveness vs readiness

- **Verify**: `curl localhost:8080/healthz/live` → `{"status":"alive"}`; `curl localhost:8080/healthz/ready` → `{"status":"up","checks":[...]}`.
- **Test**: `TestIntegration_Health_AllProbes`.

### #7 Standardized error envelope

- **Verify**: `curl -s localhost:8080/api/v1/users` (no token) → `{"error":{"code":"UNAUTHORIZED",...}}`.
- **Test**: `TestIntegration_UnauthorizedWithoutToken`.

### #8 Cursor + offset pagination

- **Verify**: `curl -s 'localhost:8080/api/v1/users?limit=5&offset=0' | jq .meta.page`.
- **Test**: `FuzzCursorRoundTrip`, `FuzzPaginationParse`.

### #9 HATEOAS links

- **Verify**: `curl -s 'localhost:8080/api/v1/users?limit=5' | jq .links` — `self`, `next`, `create` present.

### #10 Unit of Work

- **Verify**: any mutation produces a `UOW_COMMITTED` log line with `events=N`.
- **Test**: `TestIntegration_Outbox_ReceivesEvents` (UoW writes event to outbox inside tx).

### #11 Specification pattern

- **Verify**: code review — `internal/domain/specification/spec.go` + `repository.Base.Render`.
- **Test**: `FuzzCursorRoundTrip` exercises the package indirectly.

### #12 Domain Events

- **Verify**: POST a user → grep logs for `DOMAIN_EVENT_PUBLISHED` topic=`user.created`.
- **Test**: `TestIntegration_Outbox_ReceivesEvents`.

### #13 Value Objects for IDs

- **Verify**: `FuzzParseEmail` (fuzzer hammers `valueobject.ParseEmail`).
- **Test**: `TestUserBuilder_Defaults` (asserts `UserID` non-zero).

### #14 Graceful shutdown with drain

- **Verify**: `kill -TERM <pid>` → logs show `SIGNAL_RECEIVED → DRAIN_START → JOBS_DRAINED → OUTBOX_RELAY_STOPPED → DRAIN_COMPLETE`, exit 0.

### #15 Env validation (fail fast)

- **Verify**: `JWT_SECRET=short APP_ENV=production go run ./cmd/server` → process exits with a validation error.

### #16 Dockerfile + docker-compose

- **Verify**: `docker compose up -d --build` → `curl localhost:8080/healthz/live` → 200.

### #17 Migration runner

- **Verify**: `go run ./cmd/migrate -direction=up` → logs show `MIGRATION_APPLIED` per file.
- **Test**: integration tests boot the server which runs migrations automatically.

### #18 CI/CD (GitHub Actions)

- **Verify**: open a PR → `.github/workflows/ci.yml` runs lint + unit + integration + docker jobs.

### #19 Test factories + integration tests

- **Verify**: `tests/factory.go` exists; `go test -tags=integration ./tests/...` passes.

### #20 Per-method logging with timings

- **Verify**: any repository call → `REPOSITORY_CALL` log with `durationMs`, `rowsAffected`, `op`, `entity`.
- **Metric**: `db_query_duration_seconds`.

---

## 3. 18 world-class additions

### A1 OpenAPI 3.1 + Swagger UI + ReDoc

- **Verify**: `curl localhost:8080/openapi.json | jq .openapi` → `3.1.0`; browse `/docs/swagger`, `/docs/redoc`.
- **Test**: `TestIntegration_OpenAPI_Served`, `TestIntegration_DocsUI_Served`.

### A2 GraphQL API

- **Verify**: `curl localhost:8080/graphql/schema` → SDL; POST a mutation to `/graphql`.
- **Test**: `TestIntegration_GraphQL_CreateAndQuery`, `TestIntegration_GraphQL_SchemaServed`.

### A3 OpenTelemetry tracing

- **Verify**: send `traceparent: 00-<32hex>-<16hex>-01` header → grep logs for that traceId.
- **Test**: implicit (middleware stamps ctx).

### A4 Idempotency-Key

- **Verify**: POST twice with same `Idempotency-Key` + body → same response, single DB row.
- **Test**: `TestIntegration_Idempotency_Dedup`, `TestIdempotency_Dedup`, `TestIdempotency_NoKey_PassesThrough`.

### A5 ETag + conditional requests

- **Verify**: GET → capture `ETag` → GET with `If-None-Match: <etag>` → 304.
- **Test**: `TestIntegration_ETag_304`, `TestETag_304_OnMatch`.

### A6 Security headers

- **Verify**: `curl -sI localhost:8080/healthz/live` → all OWASP headers present.
- **Test**: `TestIntegration_SecurityHeaders_Present`, `TestSecurityHeaders_AllPresent`, `TestSecurityHeaders_HSTS_InProd`.

### A7 Transactional Outbox

- **Verify**: POST a user → `SELECT count(*) FROM outbox WHERE topic='user.created'` → ≥1.
- **Test**: `TestIntegration_Outbox_ReceivesEvents`.
- **Wiring proof**: `internal/domain/uow/uow.go` `Run()` calls `outbox.AppendTx` inside the tx before commit (third-pass fix).

### A8 Resilience (circuit breaker + retry + bulkhead)

- **Verify**: code review — `repository.Base.Execute` wraps DB calls with breaker + retry.
- **Wiring proof**: `internal/domain/repository/base.go` `Execute()` method (third-pass fix).

### A9 PII redaction

- **Verify**: log a line containing `alice@example.com` → output contains `[REDACTED]` instead.
- **Wiring proof**: `internal/logger/logger.go` `Init()` wraps stdout in `redaction.NewWriter` (third-pass fix).

### A10 Composite health

- **Verify**: `/healthz/startup`, `/healthz/live`, `/healthz/ready`, `/healthz/deep` all return 200.
- **Test**: `TestIntegration_Health_AllProbes`.

### A11 pprof

- **Verify**: `FEATURES=+use_pprof go run ./cmd/server` → `curl localhost:8080/debug/pprof/` → profile index.

### A12 Compression + body size limit

- **Verify**: `curl -sH 'Accept-Encoding: gzip' -o /dev/null -w '%{size_download}' localhost:8080/api/v1/users?limit=100` → small; POST 2 MiB → 413.
- **Test**: `TestIntegration_MaxBodySize_413`, `TestMaxBodySize_Reject`.

### A13 API versioning + Deprecation/Sunset

- **Verify**: `curl -sI localhost:8080/api/v1/users | grep api-version` → `v1`.
- **Test**: `TestIntegration_Versioning_Header`.

### A14 Feature flags

- **Verify**: `FEATURES=-use_graphql go run ./cmd/server` → `/graphql` returns 404.

### A15 Background job runner

- **Verify**: boot → logs show `JOB_REGISTERED`, `JOB_STARTED`, `JOB_RUN_START`, `JOB_RUN_END`.
- **Test**: implicit (the heartbeat job runs every 30s).

### A16 Fuzz tests

- **Verify**: `go test -fuzz=FuzzParseEmail -fuzztime=10s ./tests/` → no crash.
- **Test**: `FuzzParseEmail`, `FuzzCursorRoundTrip`, `FuzzPaginationParse`.

### A17 k6 load tests + SLO/SLI

- **Verify**: `k6 run scripts/k6/users_load.js` → prints p99 + error_rate + rps.
- **Doc**: `docs/architecture/slo_sli.md`.

### A18 SBOM

- **Verify**: `./scripts/sbom/sbom.sh > sbom.json` → valid CycloneDX JSON.

---

## 4. Third-pass wiring + verification

These are the fixes that turned "feature exists" into "feature is wired":

| Fix | Before | After | Proof |
|-----|--------|-------|-------|
| Outbox in UoW | events dispatched post-commit but NOT persisted | events persisted to outbox table inside the same tx, then dispatched | `TestIntegration_Outbox_ReceivesEvents` asserts `outbox` row count > 0 |
| PII redaction auto-wired | `redaction.Scrub()` existed but nobody called it | `logger.Init` wraps stdout in `redaction.NewWriter` — every log line auto-scrubbed | log a line with an email, observe `[REDACTED]` |
| Resilience in repos | `resilience.CircuitBreaker` existed but unused | `repository.Base.Execute` wraps DB calls with breaker + retry | `repository.go` calls `r.Execute(ctx, "get_by_id", fn)` |
| Integration test coverage | only User CRUD tested | 10 new integration tests cover GraphQL, OpenAPI, idempotency, ETag, outbox, health, security headers, versioning, body limit, API key | `go test -tags=integration ./tests/...` |
| Middleware unit tests | none | 12 unit tests cover Recover, RequestID, JWTAuth, RBAC, Idempotency, ETag, SecurityHeaders, MaxBodySize, RateLimit | `go test ./internal/middleware/...` |
| Linter config | missing | `.golangci.yml` with depguard layering + gosec + revive + 20+ linters | `golangci-lint run` |
| Observability stack | missing | `deploy/observability/docker-compose.observability.yml` + Prometheus + Grafana + Jaeger + Loki + Alertmanager + dashboard | `docker compose -f deploy/observability/docker-compose.observability.yml up -d` |
| Alert rules | missing | `deploy/prometheus/alert_rules.yml` with 6 SLO-derived alerts | load in Prometheus |
| Smoke test | missing | `scripts/smoke_test.sh` exercises 12 categories of endpoints | `./scripts/smoke_test.sh` |
| Helm chart | missing | `deploy/helm/` with Deployment, Service, Secret, SA, HPA, probes | `helm template deploy/helm/` |
| Project hygiene | missing | CHANGELOG, LICENSE, SECURITY, CONTRIBUTING, CODEOWNERS, .dockerignore, .editorconfig, Issue/PR templates | `ls -la` at repo root |

---

## 5. End-to-end smoke test

The smoke test at `scripts/smoke_test.sh` exercises every endpoint and
asserts the right status code. It's the single command that proves the
whole stack is wired.

### Step-by-step

```bash
# 1. Boot the server (terminal 1)
cd hexagonal-go
cp .env.example .env       # fill DATABASE_URL + JWT_SECRET
make migrate-up
make run

# 2. Run the smoke test (terminal 2)
BASE_URL=http://localhost:8080 ./scripts/smoke_test.sh
```

Expected output (abridged):

```
=== Hexagonal Go smoke test against http://localhost:8080 ===

[1/12] Health probes
  ✓ GET /healthz/live (200)
  ✓ GET /healthz/ready (200)
  ...
[7/12] REST CRUD round-trip
  ✓ POST /api/v1/users -> 201 (201)
  ✓ GET /api/v1/users/{id} -> 200 (200)
  ✓ PATCH /api/v1/users/{id}/status -> 200 (200)
  ✓ DELETE /api/v1/users/{id} -> 204 (204)
...
=== Summary: 38 passed, 0 failed ===
```

If any check fails, the script exits non-zero — wire it into CI as a
post-deploy gate.

---

## 6. Observability stack

### Step-by-step

```bash
# 1. Boot the app + the observability stack
docker compose up -d --build                                  # app + postgres
docker compose -f deploy/observability/docker-compose.observability.yml up -d

# 2. Open the dashboards
open http://localhost:3000    # Grafana (admin/admin)
open http://localhost:9090    # Prometheus
open http://localhost:9093    # Alertmanager
open http://localhost:16686   # Jaeger
open http://localhost:3100    # Loki (API only)

# 3. Generate some traffic
TOKEN=$(curl -s localhost:8080/auth/demo-token | jq -r .token)
for i in $(seq 1 100); do
  curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/users?limit=1 > /dev/null
done

# 4. Verify metrics are flowing
curl -s localhost:9090/api/v1/query?query=http_requests_total | jq

# 5. Verify the Grafana dashboard auto-loaded
#    Dashboards → hexagonal-go → "Hexagonal Go - Operations"
```

---

## 7. Production deployment

### Kubernetes via Helm

```bash
# 1. Build + push the image
docker build -t your-registry/hexagonal-go:1.0.0 .
docker push your-registry/hexagonal-go:1.0.0

# 2. Create a values override
cat > prod.yaml <<EOF
image:
  repository: your-registry/hexagonal-go
  tag: "1.0.0"
replicaCount: 3
secrets:
  databaseUrl: "postgres://..."     # use a sealed-secret in prod
  jwtSecret: "<32-byte-secret>"
  apiKeys: "prod-key-1,prod-key-2"
config:
  app:
    env: production
    logLevel: info
    logFormat: json
EOF

# 3. Install
helm install hexgo deploy/helm/ -f prod.yaml

# 4. Verify
kubectl get pods -l app.kubernetes.io/instance=hexgo
kubectl describe hpa hexgo
```

### Post-deploy gate (per `docs/cmmi/03_PPQA.md`)

Within 30 minutes of deploy, run the smoke test against the production URL:

```bash
BASE_URL=https://api.example.com ./scripts/smoke_test.sh
```

All 38 checks must pass. If any fail, roll back via `helm rollback hexgo`.
