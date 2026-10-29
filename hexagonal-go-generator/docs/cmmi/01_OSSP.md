# 01 - Organizational Standard Process (OSSP)

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

## Lifecycle stages

1. **Intake** — issue created in the tracker, tagged with `domain:<aggregate>`.
2. **Design** — short design note in `docs/decisions/` referencing the affected
   layer (entity / repo / service / controller / middleware). Output: ADR
   using the DAR template (09).
3. **Implementation** — code follows the hexagonal layering rules in
   `docs/architecture.md`. No business logic in controllers or repositories.
4. **Self-review** — author self-checks against the PPQA pre-merge gate (03).
5. **Peer review** — one reviewer; CI must pass; PPQA pre-merge gate green.
6. **Merge** — squash-merge to `develop`; tagged build produced.
7. **Release** — see CM (05) for the release cadence.
8. **Post-deploy** — PPQA post-deploy gate within 30 minutes.
9. **Retrospective** — every two weeks; outputs feed CAR (06) and OID (07).

## Hexagonal layering rules (mandatory)

| Layer | Allowed to import | Forbidden to import |
|-------|-------------------|---------------------|
| `domain/entities/*` | `domain/valueobject`, `domain/events`, `domain/specification` | anything else |
| `domain/*` (tactical) | stdlib only | any package |
| `application/services/*` | `domain/*`, `httpx` (for typed errors only) | `database`, `net/http`, chi |
| `infrastructure/repositories/*` | `domain/*`, `database` | `api/*` |
| `api/controllers/*` | `application/services/*`, `api/dtos`, `api/mappers`, `api/validations`, `httpx`, `net/http`, chi | `infrastructure/*`, `database` |
| `middleware/*`, `server/*` | everything | n/a |

These rules are enforced by `golangci-lint` with `depguard` (configured in
`.golangci.yml`).
