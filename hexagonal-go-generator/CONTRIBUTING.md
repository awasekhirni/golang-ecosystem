# Contributing

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

Thank you for considering a contribution! This document is the short
version; the long version lives in the CMMI PAL (`docs/cmmi/`).

## Quick start

```bash
git clone <repo>
cd hexagonal-go
cp .env.example .env       # fill DATABASE_URL + JWT_SECRET
go mod tidy
make migrate-up            # apply migrations
make run                   # boot the server
make test                  # unit tests (no DB needed)
make test-integration      # integration tests (needs DATABASE_URL)
make lint                  # golangci-lint
```

## Before you open a PR

1. **Layering**: respect the hexagonal rules in `docs/cmmi/01_OSSP.md`.
   The `depguard` linter enforces them automatically.
2. **Tests**: add or update tests for every behavior change. Unit tests
   must pass `make test`; integration tests must pass `make test-integration`.
3. **Logging**: every new code path should emit at least one structured
   log line tagged with the right `layer` (see
   `docs/architecture/logging_matrix.md`).
4. **Migrations**: schema changes go in paired `*.up.sql` / `*.down.sql`
   files under `internal/database/migrations/`. Forward-only compatible.
5. **Docs**: if you add a feature, update `README.md` and the relevant
   CMMI PAL doc in the same PR.
6. **Commit messages**: conventional commits (`feat:`, `fix:`, `chore:`,
   `docs:`, `refactor:`, `test:`).

## PR checklist

- [ ] `gofmt -l .` returns nothing
- [ ] `go vet ./...` clean
- [ ] `golangci-lint run` clean
- [ ] `make test` green
- [ ] `make test-integration` green (if you touched DB or service code)
- [ ] CHANGELOG.md updated
- [ ] At least one approving review

## Branching

Trunk-based: short-lived `feat/*`, `fix/*`, `chore/*` branches merged to
`develop` via PR. Hotfixes branch from `main` and merge to both `main`
and `develop`. See `docs/cmmi/05_CM.md` for details.
