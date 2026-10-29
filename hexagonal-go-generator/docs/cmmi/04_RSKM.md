# 04 - Risk Management (RSKM)

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

## Top-N risk register

The register is reviewed at the monthly engineering meeting. Each risk has an
owner, a likelihood (1-5), an impact (1-5), a risk score (L×I), and a
mitigation.

| ID | Risk | L | I | Score | Owner | Mitigation |
|----|------|---|---|-------|-------|------------|
| R01 | DB pool exhaustion under burst load | 3 | 5 | 15 | SRE | Pool size + slow-query alerts; connection drain on shutdown; rate-limit middleware |
| R02 | JWT secret leak | 2 | 5 | 10 | Sec | Secret in sealed-secrets; 30-day rotation; min 32 bytes enforced at boot |
| R03 | Domain-event handler failure causes stale read model | 4 | 3 | 12 | Backend | Bus dispatch errors counter + alert; future: outbox table for retry |
| R04 | Migration applied out-of-order in blue-green deploy | 2 | 4 | 8 | Backend | schema_migrations ledger; migrations are forward-only; backward-compatible by policy |
| R05 | Audit log tampering | 1 | 5 | 5  | Sec | Logs are append-only in Loki with WORM storage; SHA-256 body digest for mutation audit |
| R06 | Pagination cursor forgery | 2 | 2 | 4  | Backend | Cursor is base64 of (sortValue,id); no security claim; defence is RBAC + rate-limit |
| R07 | Optimistic-concurrency lost update | 3 | 3 | 9  | Backend | `version` column on every aggregate; UPDATE ... WHERE version=:old |
| R08 | Slow query drags down pool | 4 | 4 | 16 | Backend | 100ms slow-query WARN; pgx statement_timeout=15s; per-request HTTP timeout=20s |
| R09 | Process crash mid-shutdown loses in-flight requests | 2 | 4 | 8  | SRE | Graceful drain: SIGTERM → ready=false → srv.Shutdown(ctx, 30s) → db.Close |
| R10 | Env var misconfiguration boots a broken prod | 2 | 5 | 10 | Backend | envconfig fail-fast validation; prod-only rules (debug=false, secret≥32) |

## Risk review cadence

- **Weekly**: top-3 risks reviewed at the engineering standup.
- **Monthly**: full register reviewed; risks re-scored; closed risks archived.
- **Per-incident**: a new risk is always added when a CAR (06) is filed.
