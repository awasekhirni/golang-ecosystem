# Enhancements - World-Class Additions

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

This document lists every enhancement added in the second pass (on top of
the 20 original gaps) with **step-by-step instructions** for using each one.

## Table of contents

1. [OpenAPI 3.1 spec + Swagger UI + ReDoc](#1-openapi-31-spec--swagger-ui--redoc)
2. [GraphQL API alongside REST](#2-graphql-api-alongside-rest)
3. [OpenTelemetry distributed tracing](#3-opentelemetry-distributed-tracing)
4. [Idempotency-Key middleware](#4-idempotency-key-middleware)
5. [ETag + conditional requests](#5-etag--conditional-requests)
6. [Security headers middleware (OWASP A05:2021)](#6-security-headers-middleware-owasp-a052021)
7. [Transactional Outbox pattern](#7-transactional-outbox-pattern)
8. [Resilience: circuit breaker + retry + bulkhead](#8-resilience-circuit-breaker--retry--bulkhead)
9. [PII redaction in logs](#9-pii-redaction-in-logs)
10. [Composite health subsystem](#10-composite-health-subsystem)
11. [pprof profiling endpoints](#11-pprof-profiling-endpoints)
12. [Compression + body size limit](#12-compression--body-size-limit)
13. [API versioning + Deprecation/Sunset headers](#13-api-versioning--deprecationsunset-headers)
14. [Feature flags](#14-feature-flags)
15. [Background job runner](#15-background-job-runner)
16. [Fuzz tests](#16-fuzz-tests)
17. [k6 load tests + SLO/SLI](#17-k6-load-tests--slosli)
18. [SBOM generation](#18-sbom-generation)

---

## 1. OpenAPI 3.1 spec + Swagger UI + ReDoc

**What**: every REST route is described in a programmatically-built OpenAPI
3.1 document. Two interactive UIs (Swagger UI for try-it-out, ReDoc for
readable reference) and the raw spec are served.

**Files**: `internal/openapi/spec.go`, `internal/openapi/user_ops.go`.

### Step-by-step

1. Boot the server (`go run ./cmd/server`).
2. Open `http://localhost:8080/docs/swagger` - interactive Swagger UI.
3. Open `http://localhost:8080/docs/redoc` - readable ReDoc reference.
4. Fetch the raw spec:
   ```bash
   curl -s localhost:8080/openapi.json | jq .info
   curl -s localhost:8080/openapi.yaml
   ```
5. Generate a client SDK from the spec:
   ```bash
   npx @openapitools/openapi-generator-cli generate \
     -i http://localhost:8080/openapi.json \
     -g go -o ./generated-client
   ```

### Adding a new operation

Call `openapi.RegisterUserOps(b)` from `server.buildOpenAPI()`. For a new
aggregate, write a `Register<Order>Ops(b)` function in
`internal/openapi/<name>_ops.go` following the same pattern, then add a
call in `buildOpenAPI()`.

---

## 2. GraphQL API alongside REST

**What**: the same `usersvc.Service` that powers REST is exposed via
GraphQL at `/graphql`. The schema mirrors the REST resource model.

**Files**: `internal/graphql/handler.go`, `internal/graphql/playground.go`.

### Step-by-step

1. Boot the server.
2. Open `http://localhost:8080/graphql/playground` - GraphiQL UI.
3. Fetch the SDL:
   ```bash
   curl -s localhost:8080/graphql/schema
   ```
4. Mint a JWT (no auth required):
   ```bash
   TOKEN=$(curl -s localhost:8080/auth/demo-token | jq -r .token)
   ```
5. Run a mutation:
   ```bash
   curl -s localhost:8080/graphql -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"query":"mutation { createUser(input: {email: \"bob@example.com\", firstName: \"Bob\", lastName: \"Lee\"}) { id email fullName status } }"}'
   ```
6. Run a query:
   ```bash
   curl -s localhost:8080/graphql -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"query":"{ users(limit: 10) { totalCount edges { node { id email } } } }"}'
   ```
7. Toggle GraphQL off via env: `FEATURES=-use_graphql`.

### Adding a new aggregate to GraphQL

Add a resolver method in `internal/graphql/handler.go` (`resolveOrders`,
etc.) and a case in `execute()`. The SDL in `Schema()` should be extended
to match.

---

## 3. OpenTelemetry distributed tracing

**What**: every request gets a trace id (from the W3C `traceparent` header
or freshly minted). The id is stamped on the request context so every
downstream `logger.Ctx()` line carries it. `otel.Start()` produces spans
for service-layer operations.

**Files**: `internal/infrastructure/otel/otel.go`.

### Step-by-step

1. Boot the server.
2. Send a request with a propagated traceparent:
   ```bash
   curl -s localhost:8080/api/v1/users \
     -H "Authorization: Bearer $TOKEN" \
     -H 'traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01'
   ```
3. Grep the logs for the trace id:
   ```bash
   # the trace id from the traceparent above is 0af7651916cd43dd8448eb211c80319c
   ```
4. To produce explicit spans inside a service:
   ```go
   ctx, span := otel.Start(ctx, "user.ChangeStatus")
   defer span.End()
   span.SetAttr("user_id", id.String())
   ```
5. To upgrade to a real OTLP exporter, see the upgrade notes at the top of
   `internal/infrastructure/otel/otel.go`.

---

## 4. Idempotency-Key middleware

**What**: clients that send `Idempotency-Key: <uuid>` on a POST/PUT/PATCH
get deduplicated for 24h. Replays return the cached response verbatim
(status, headers, body).

**Files**: `internal/middleware/idempotency.go`.

### Step-by-step

1. Boot the server.
2. Mint a token, then create a user with an idempotency key:
   ```bash
   TOKEN=$(curl -s localhost:8080/auth/demo-token | jq -r .token)
   KEY=$(uuidgen)
   curl -s localhost:8080/api/v1/users \
     -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -H "Idempotency-Key: $KEY" \
     -d '{"email":"carol@example.com","first_name":"Carol","last_name":"Ng"}'
   ```
3. Retry the exact same request with the same key:
   ```bash
   curl -s localhost:8080/api/v1/users \
     -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -H "Idempotency-Key: $KEY" \
     -d '{"email":"carol@example.com","first_name":"Carol","last_name":"Ng"}'
   ```
   You'll get the SAME response (same `id`, same `Location`) and only one
   row exists in the DB.
4. Change the body but keep the key — the cache key includes a body digest,
   so the new body is treated as a fresh request (and likely returns 409
   Conflict because the email is taken).
5. For multi-instance deployments, replace `memStore` with a Redis-backed
   `IdempotencyStore` (the interface is identical).

---

## 5. ETag + conditional requests

**What**: GET responses get a weak ETag (SHA-256 prefix). Clients that send
`If-None-Match: <etag>` get a 304 Not Modified when the resource is
unchanged.

**Files**: `internal/middleware/etag.go`.

### Step-by-step

1. Boot the server, fetch a user, capture the ETag:
   ```bash
   TOKEN=$(curl -s localhost:8080/auth/demo-token | jq -r .token)
   ETAG=$(curl -sI localhost:8080/api/v1/users/<id> \
     -H "Authorization: Bearer $TOKEN" | grep -i etag | awk '{print $2}' | tr -d '\r\n')
   ```
2. Re-fetch with `If-None-Match`:
   ```bash
   curl -i localhost:8080/api/v1/users/<id> \
     -H "Authorization: Bearer $TOKEN" \
     -H "If-None-Match: $ETAG"
   # HTTP/1.1 304 Not Modified
   ```
3. Mutate the user (PATCH status), then re-fetch with the old ETag — you'll
   get 200 with a new ETag.

---

## 6. Security headers middleware (OWASP A05:2021)

**What**: every response gets `X-Frame-Options: DENY`,
`X-Content-Type-Options: nosniff`, `Referrer-Policy`,
`Permissions-Policy`, `Content-Security-Policy`, `Cache-Control: no-store`,
`Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy`. In
production, `Strict-Transport-Security` (HSTS) is added.

**Files**: `internal/middleware/security.go`.

### Step-by-step

1. Boot the server.
2. Inspect the headers:
   ```bash
   curl -sI localhost:8080/healthz
   ```
3. Confirm every response carries the security headers regardless of path.
4. Toggle off via `FEATURES=-use_security_headers`.

---

## 7. Transactional Outbox pattern

**What**: domain events that MUST not be lost are written into an `outbox`
table inside the same DB transaction as the business write. A relay worker
polls the table every 2s and dispatches pending events to the enterprise
bus. At-least-once delivery; consumers dedupe by `event_id`.

**Files**: `internal/infrastructure/outbox/outbox.go`,
`internal/database/migrations/000002_create_outbox.up.sql`.

### Step-by-step

1. Boot the server — the outbox table is created by the migration runner.
2. From inside a UoW, append an event:
   ```go
   err := uow.Run(ctx, func(tx uow.Tx) error {
       if err := repo.Create(ctx, u); err != nil { return err }
       return outbox.New().Append(ctx, tx.SQLTx(), userent.UserCreated{
           Base: events.NewBase(), UserID: u.ID.String(), Email: u.Email.String(),
       })
   })
   ```
3. Watch the relay worker pick it up:
   ```bash
   # logs:
   # OUTBOX_APPENDED  eventId=... topic=user.created
   # OUTBOX_DISPATCHED eventId=... topic=user.created
   ```
4. Inspect the table:
   ```bash
   psql $DATABASE_URL -c "SELECT id, event_id, topic, processed_at FROM outbox ORDER BY id DESC LIMIT 10;"
   ```
5. Toggle off via `FEATURES=-use_outbox_relay`.

---

## 8. Resilience: circuit breaker + retry + bulkhead

**What**: three classical resilience patterns for talking to unreliable
dependencies.

**Files**: `internal/resilience/resilience.go`.

### Step-by-step

1. Wrap a flaky upstream call in a circuit breaker:
   ```go
   cb := resilience.NewCircuitBreaker("upstream-api", 5, 30*time.Second)
   err := cb.Execute(ctx, func(ctx context.Context) error {
       return callUpstream(ctx)
   })
   if errors.Is(err, resilience.ErrCircuitOpen) {
       // degrade gracefully: return cached value
   }
   ```
2. Retry a transient failure:
   ```go
   err := resilience.Retry(ctx, "send_email", resilience.DefaultRetryPolicy(),
       func(ctx context.Context) error { return sendEmail(ctx) })
   ```
3. Cap concurrency to a downstream:
   ```go
   bulk := resilience.NewBulkhead("db", 50)
   release, err := bulk.Acquire(ctx)
   if err != nil { return err }
   defer release()
   // ... do the work ...
   ```
4. Watch state transitions in the logs:
   ```bash
   # CIRCUIT_TRIPPED  breaker=upstream-api state=open consecutiveFailures=5
   # CIRCUIT_HALF_OPEN breaker=upstream-api state=half-open
   # CIRCUIT_CLOSED   breaker=upstream-api state=closed
   ```

---

## 9. PII redaction in logs

**What**: a regex-based scrubber rewrites emails, phones, SSNs, credit
cards, and JWTs to `[REDACTED]` before they hit logs.

**Files**: `internal/redaction/redaction.go`.

### Step-by-step

1. Use the scrubber when logging potentially-PII payloads:
   ```go
   import "github.com/ori-inc/hexagonal-go/internal/redaction"

   log.Info().
       Str("body", redaction.Scrub(string(body))).
       Msg("incoming_request")
   ```
2. Verify:
   ```bash
   # input:  {"email":"alice@example.com","ssn":"123-45-6789"}
   # logged: body="[REDACTED],[REDACTED]" (roughly)
   ```
3. Add a custom pattern by appending to `redaction.Patterns` at boot:
   ```go
   redaction.Patterns = append(redaction.Patterns,
       regexp.MustCompile(`PATIENT-\d+`))
   ```

---

## 10. Composite health subsystem

**What**: four probes mapped to the Kubernetes probe taxonomy.

| Endpoint | Probe type | Checks deps? | When to use |
|----------|-----------|--------------|-------------|
| `/healthz/startup` | startup | no | K8s `startupProbe` - gates liveness/readiness until first 200 |
| `/healthz/live` | liveness | no | K8s `livenessProbe` - restart pod on failure |
| `/healthz/ready` | readiness | yes (1s budget) | K8s `readinessProbe` - remove from service on failure |
| `/healthz/deep` | deep | yes (5s budget) | dashboards - full report always 200 |

**Files**: `internal/health/health.go`.

### Step-by-step

1. Boot the server.
2. Curl each probe:
   ```bash
   curl -s localhost:8080/healthz/startup  # {"status":"started"}
   curl -s localhost:8080/healthz/live     # {"status":"alive"}
   curl -s localhost:8080/healthz/ready    # {"status":"up","checks":[...]}
   curl -s localhost:8080/healthz/deep     # full report with per-check latency
   ```
3. Register a new checker during boot:
   ```go
   c.Health.Register("redis", func(ctx context.Context) error {
       return rdb.Ping(ctx).Err()
   })
   ```

---

## 11. pprof profiling endpoints

**What**: Go's built-in pprof endpoints, mounted at `/debug/pprof/*` behind
the feature flag `use_pprof` (off by default).

**Files**: `internal/server/server.go` (the `pprof` import + routes).

### Step-by-step

1. Enable the flag and boot:
   ```bash
   FEATURES=+use_pprof go run ./cmd/server
   ```
2. Capture a CPU profile:
   ```bash
   go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
   ```
3. Inspect goroutines:
   ```bash
   curl -s localhost:8080/debug/pprof/goroutine?debug=2 | head -50
   ```
4. In production, gate `/debug/pprof/*` behind RBAC (admin-only) — see
   OSSP (01) for the layering rule.

---

## 12. Compression + body size limit

**What**: gzip compression for clients that send `Accept-Encoding: gzip`,
plus a 1 MiB body size limit on every request (413 on overflow).

**Files**: `internal/middleware/compression.go`, `internal/middleware/security.go` (`MaxBodySize`).

### Step-by-step

1. Boot the server.
2. Verify compression:
   ```bash
   curl -sH 'Accept-Encoding: gzip' -o /dev/null -w '%{size_download}\n' \
     localhost:8080/api/v1/users?limit=100 -H "Authorization: Bearer $TOKEN"
   # small number = compressed
   ```
3. Verify body limit:
   ```bash
   head -c 2000000 /dev/urandom | base64 > /tmp/big.json
   curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/api/v1/users \
     -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     --data-binary @/tmp/big.json
   # 413
   ```

---

## 13. API versioning + Deprecation/Sunset headers

**What**: path-based versioning (`/api/v1/...`). Every response carries
`API-Version: v1`. When a version is deprecated, `Deprecation: <date>` is
added; when it's scheduled for removal, `Sunset: <date>` is added too
(RFC 8594).

**Files**: `internal/versioning/versioning.go`.

### Step-by-step

1. Boot the server.
2. Inspect the response headers:
   ```bash
   curl -sI localhost:8080/api/v1/users -H "Authorization: Bearer $TOKEN" | grep -i 'api-version'
   # api-version: v1
   ```
3. To deprecate v1 (do this when v2 ships):
   ```go
   policy := versioning.Policy{
       Deprecated: map[string]time.Time{"v1": time.Now()},
       Sunset:     map[string]time.Time{"v1": time.Now().Add(180 * 24 * time.Hour)},
   }
   // pass to versioning.Middleware(policy)
   ```
4. Client libraries that respect `Deprecation`/`Sunset` will surface
   warnings automatically.

---

## 14. Feature flags

**What**: env-driven flags (hot-swappable via `FEATURES=+flag,-flag`) gate
every optional subsystem. Defaults are tuned for dev (everything on except
pprof).

**Files**: `internal/features/features.go`.

### Step-by-step

1. Boot with everything default:
   ```bash
   go run ./cmd/server
   ```
2. Boot with GraphQL and OpenAPI UI off:
   ```bash
   FEATURES=-use_graphql,-use_openapi_ui go run ./cmd/server
   ```
3. Boot with pprof on in prod:
   ```bash
   APP_ENV=production FEATURES=+use_pprof go run ./cmd/server
   ```
4. Use a flag in code:
   ```go
   if features.On(features.FlagCompression) {
       r.Use(middleware.Compress())
   }
   ```
5. Available flags: `use_graphql`, `use_outbox_relay`, `use_openapi_ui`,
   `use_compression`, `use_otel_tracing`, `use_idempotency`,
   `use_security_headers`, `use_etag`, `use_pprof`.

---

## 15. Background job runner

**What**: a tiny cron-style runner with at-least-once delivery and
exponential-backoff retry. Jobs run in their own goroutines; panics are
recovered and logged.

**Files**: `internal/jobs/runner.go`.

### Step-by-step

1. Register a job during boot (already wired in `server.Build`):
   ```go
   runner.Register(jobs.Job{
       Name: "cleanup_soft_deleted_users",
       Schedule: 6 * time.Hour,
       MaxRetries: 3,
       Run: func(ctx context.Context) error {
           return cleanupService.PurgeOlderThan(ctx, 30*24*time.Hour)
       },
   })
   ```
2. Watch the logs:
   ```bash
   # JOB_REGISTERED job=cleanup_soft_deleted_users schedule=6h0m0s
   # JOB_STARTED job=cleanup_soft_deleted_users
   # JOB_RUN_START job=cleanup_soft_deleted_users
   # JOB_RUN_END job=cleanup_soft_deleted_users durationMs=123
   ```
3. On SIGTERM the runner drains (`JOBS_DRAINED`) before the DB pool closes.

---

## 16. Fuzz tests

**What**: Go 1.18+ native fuzz tests for value objects and the pagination
cursor round-trip.

**Files**: `tests/fuzz_test.go`.

### Step-by-step

1. Run the corpus once (regression mode):
   ```bash
   go test -run FuzzParseEmail ./tests/
   go test -run FuzzCursorRoundTrip ./tests/
   ```
2. Run a fuzzer for 30s to discover new crashes:
   ```bash
   go test -fuzz=FuzzParseEmail -fuzztime=30s ./tests/
   ```
3. If a crash is found, the failing input is saved under
   `testdata/fuzz/<TestName>/<hash>` and becomes a permanent regression.

---

## 17. k6 load tests + SLO/SLI

**What**: a k6 script that ramps 0 → 100 VUs over 60s and validates the
p99 < 500ms + error-rate < 1% SLOs. SLO/SLI definitions and Alertmanager
rules live in `docs/architecture/slo_sli.md`.

**Files**: `scripts/k6/users_load.js`, `docs/architecture/slo_sli.md`.

### Step-by-step

1. Install k6: `brew install k6` (macOS) or see
   https://k6.io/docs/getting-started/installation/.
2. Boot the server.
3. Mint a token:
   ```bash
   TOKEN=$(curl -s localhost:8080/auth/demo-token | jq -r .token)
   ```
4. Run the load test:
   ```bash
   k6 run --vus 50 --duration 60s \
     -e BASE_URL=http://localhost:8080 \
     -e TOKEN=$TOKEN \
     scripts/k6/users_load.js
   ```
5. The script prints a JSON summary at the end:
   ```json
   {
     "p99_ms": 142,
     "error_rate": 0.002,
     "rps": 187.3,
     "passes": true
   }
   ```
6. Wire it into CI by adding a k6 step to `.github/workflows/ci.yml`.

---

## 18. SBOM generation

**What**: a CycloneDX Software Bill of Materials for the Go module.
Required by EO 14028 and SOC2 supply-chain audits.

**Files**: `scripts/sbom/sbom.sh`.

### Step-by-step

1. Install cyclonedx-gomod:
   ```bash
   go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
   ```
2. Generate the SBOM:
   ```bash
   ./scripts/sbom/sbom.sh > sbom.json
   ```
3. Validate it:
   ```bash
   cyclonedx validate --input-file sbom.json --input-format json \
     --input-version v1.5 --schema-type application
   ```
4. Upload `sbom.json` to your artifact store on every release (add a step
   to `.github/workflows/ci.yml`).
