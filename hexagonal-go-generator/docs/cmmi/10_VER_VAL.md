# 10 - Verification & Validation (VER / VAL)

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

## Test pyramid

```
                    ┌──────────┐
                    │   e2e    │   1-2 tests: full HTTP->DB->bus round trip
                    ├──────────┤
                    │ integra- │   ~10 tests: real postgres, real bus
                    │  tion    │   go test -tags=integration
                    ├──────────┤
                    │  unit    │   ~50+ tests: pure go, ms-fast
                    └──────────┘
```

## What each layer must cover

### Unit tests (`go test -short`)

- Aggregate invariants (state machines, value object validation).
- Mapper round-trips.
- Validation rules.
- Specification combinators (And/Or/Not).
- Pagination cursor encode/decode.
- Error envelope code→status mapping.

Target: ≥85% coverage on `internal/domain/*` and `internal/httpx/*`.

### Integration tests (`go test -tags=integration`)

- Repository Create/Get/List/Update/SoftDelete round-trips against a real
  postgres.
- Optimistic-concurrency: stale `version` rejected.
- Service dispatches domain events through the bus.
- Migration runner applies forward-only cleanly from empty DB.
- Generator introspects a fixture schema and emits parseable Go.

Target: one happy-path + one failure-path per aggregate per operation.

### End-to-end tests (manual or CI nightly)

- Boot server, hit `/healthz`, `/readyz`, `/metrics`.
- Auth flow: `/auth/demo-token` → use token to POST `/api/v1/users` → GET
  list → PATCH status → DELETE → confirm 404 on re-GET.

## Validation (VAL) — does the right thing?

VAL is owned by the product owner, not engineering. It is expressed as:

- Acceptance criteria on every issue (Given/When/Then).
- The PPQA post-deploy gate (03) is the production-side VAL.
- A monthly "user-facing API audit" reviews the OpenAPI/DTO drift.

## Verification (VER) — built the right way?

VER is owned by engineering. It is expressed as:

- All tests green at the appropriate layer.
- `go vet`, `golangci-lint`, `gofmt` clean.
- Layering rules from OSSP (01) enforced by `depguard`.
- Coverage delta does not drop >2% on changed packages.
