# Architecture

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

## Layered hexagonal (ports & adapters)

```
                      ┌───────────────────────────────────┐
                      │            HTTP client            │
                      └───────────────┬───────────────────┘
                                      │
              ┌───────────────────────▼───────────────────────┐
              │  Middleware: Recover, RequestID, HTTPLogger,   │
              │  CORS, RateLimit, Timeout, APIKey, JWT, RBAC,  │
              │  Audit                                          │
              └───────────────────────┬───────────────────────┘
                                      │
              ┌───────────────────────▼───────────────────────┐
              │  API layer:                                    │
              │   controllers/  - thin HTTP handlers          │
              │   dtos/         - wire format                 │
              │   mappers/      - entity <-> dto              │
              │   validations/  - field-level rules           │
              └───────────────────────┬───────────────────────┘
                                      │ (calls)
              ┌───────────────────────▼───────────────────────┐
              │  Application layer:                            │
              │   services/   - orchestration + UoW + events   │
              └───────────────────────┬───────────────────────┘
                                      │ (uses ports)
              ┌───────────────────────▼───────────────────────┐
              │  Domain layer (pure, no I/O):                  │
              │   entities/      - aggregate roots + invariants│
              │   valueobject/   - typed IDs, Email, Slug      │
              │   specification/ - query DSL                   │
              │   uow/           - transactional boundary      │
              │   events/        - domain event primitives     │
              │   bus/           - enterprise bus (in-memory)  │
              │   repository/    - enterprise repository base  │
              └───────────────────────▲───────────────────────┘
                                      │ (adapters implement ports)
              ┌───────────────────────┴───────────────────────┐
              │  Infrastructure layer:                         │
              │   repositories/  - PostgreSQL adapters         │
              │   auth/          - JWT issuer                  │
              │   database/      - pgx pool + migration runner │
              └───────────────────────────────────────────────┘
```

## Cross-cutting concerns

| Concern | Owner | Where |
|---------|-------|-------|
| Request ID | middleware.RequestID | stamped on ctx, X-Request-Id header |
| Structured logging | logger package | 12-layer matrix, zerolog |
| Error envelope | httpx package | every 4xx/5xx body |
| Pagination | httpx package | offset + cursor; HATEOAS links |
| Metrics | pkg/metrics | Prometheus `/metrics` |
| Auth | middleware.JWTAuth + APIKeyAuth | Bearer or X-API-Key |
| RBAC | middleware.Require | role-set intersection |
| Audit | middleware.Audit | SHA-256 body digest, mutations only |
| Transactions | uow package | Run(ctx, fn) with post-commit event flush |
| Domain events | events + bus packages | topic-based fan-out, goroutine-per-handler |

## Hexagonal rules

1. **The domain never imports net/http, sql, or chi.** It expresses intent
   via interfaces (ports); infrastructure provides concrete adapters.
2. **Controllers are thin.** Parse → validate → call service → write envelope.
3. **Services own orchestration**, never SQL. They use the UoW for
   multi-step mutations and the bus for event dispatch.
4. **Repositories own SQL**, never business rules. They embed
   `repository.Base` to inherit timing/metrics/slow-query logging.
5. **Every mutation goes through Audit.** POST/PUT/PATCH/DELETE get a
   single audit log line with a body digest.
6. **Every error is an `httpx.AppError`.** Controllers never format error
   JSON; they hand the error to `httpx.WriteError`.
7. **Every paginated response carries HATEOAS links.** `self`, `next`,
   `prev`, `create` at minimum.
8. **Every aggregate has typed value-object IDs** so compile-time prevents
   passing a `UserID` where an `OrderID` is expected.
