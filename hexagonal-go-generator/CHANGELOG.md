# Changelog

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

All notable changes to this project are documented here.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-10-29

### Added - First pass (20 gaps)
- JWT + API key + RBAC middleware
- Request ID / correlation ID propagation
- Structured JSON logging (12-layer matrix) via zerolog
- Audit log for all mutations (SHA-256 body digest)
- Prometheus /metrics endpoint
- Liveness vs readiness probes
- Standardized error envelope + stable error codes
- Cursor + offset pagination envelope
- HATEOAS links (REST maturity Level 3)
- Unit of Work (transactions with post-commit event flush)
- Specification pattern (And/Or/Not + FieldEq/FieldLike/...)
- Domain Events + Enterprise Bus (in-memory, fan-out)
- Value Objects for IDs (typed UserID / OrderID / ProductID)
- Graceful shutdown with drain (SIGTERM → ready=false → srv.Shutdown → db.Close)
- Env validation (fail-fast via envconfig)
- Dockerfile (multi-stage, distroless) + docker-compose
- Migration runner (embedded SQL, schema_migrations ledger)
- GitHub Actions CI (lint + unit + integration + docker)
- Test factories + integration tests
- Per-method timing logs (repository.Base.Observe)

### Added - Second pass (18 world-class enhancements)
- OpenAPI 3.1 spec generation + Swagger UI + ReDoc
- GraphQL API alongside REST (same service layer) + GraphiQL playground
- OpenTelemetry distributed tracing (W3C traceparent propagation)
- Idempotency-Key middleware (24h dedup, body-digest keyed)
- ETag + conditional requests (RFC 7232, weak ETags, 304 Not Modified)
- Security headers middleware (OWASP A05:2021: HSTS, CSP, X-Frame-Options, etc.)
- Transactional Outbox pattern (migration 000002 + relay worker)
- Resilience: circuit breaker + retry (exp backoff + jitter) + bulkhead
- PII redaction in logs (email/phone/SSN/cc/JWT auto-masked, GDPR Art. 32)
- Composite health subsystem (startup/live/ready/deep, 1s/5s budgets)
- pprof profiling endpoints (feature-flagged)
- Compression middleware (gzip) + body size limit (1 MiB)
- API versioning + Deprecation/Sunset headers (RFC 8594)
- Feature flags package (env-driven, hot-swappable via FEATURES env)
- Background job runner (cron + at-least-once + exponential retry)
- Fuzz tests (Go 1.18+ native: ParseEmail, CursorRoundTrip)
- k6 load tests + SLO/SLI/error-budget doc + Alertmanager rules
- SBOM generation (CycloneDX, EO 14028)

### Added - Third pass (verification + wiring)
- Outbox wired into UoW.Run (events persist atomically with business writes)
- PII redaction auto-wired into zerolog logger (ScrubbingWriter)
- Circuit breaker + retry auto-wired into repository.Base.Execute
- Integration tests for every new feature (GraphQL, OpenAPI, idempotency, ETag, outbox, health, security headers, versioning, body limit, API key)
- Middleware unit tests (auth, rbac, audit, idempotency, etag, security, rate limit, max body size)
- .golangci.yml with depguard layering rules + gosec + revive + 20+ linters
- Observability docker-compose (Prometheus + Grafana + Jaeger + Loki + Alertmanager)
- Prometheus alert_rules.yml + alertmanager.yml + Grafana dashboard
- Smoke-test verification script (exercises every endpoint, 12 categories)
- Helm chart (Deployment, Service, Secret, ServiceAccount, HPA, probes)
- Project hygiene: CHANGELOG, LICENSE, SECURITY, CONTRIBUTING, CODEOWNERS, .dockerignore, .editorconfig, Issue/PR templates

### Security
- No CVEs in shipped dependencies (verified via govulncheck in CI)

[1.0.0]: https://github.com/ori-inc/hexagonal-go/releases/tag/v1.0.0
