# 03 - Quality Gates (PPQA)

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

Three gates must be green before code can advance to the next stage.

## Pre-merge gate (enforced by CI)

The GitHub Actions workflow `.github/workflows/ci.yml` fails the build if any
of the following is red:

- [ ] `gofmt -l .` returns no files
- [ ] `go vet ./...` clean
- [ ] `golangci-lint run` clean (incl. `depguard` for layer rules)
- [ ] `go test -race -count=1 -short ./...` green
- [ ] Integration tests green against a real postgres service container
- [ ] All three binaries (`server`, `migrate`, `generator`) build
- [ ] Docker image builds (proves the Dockerfile is not broken)
- [ ] Code coverage delta does not drop >2% on changed packages
- [ ] At least one approving review

## Pre-release gate (manual, release manager)

- [ ] All pre-merge gates green on the release commit
- [ ] CHANGELOG.md updated with semver rationale
- [ ] Migration files reviewed for backwards compatibility (no destructive
      ALTER without a paired rollback)
- [ ] Load test passed at 2x expected peak for 10 minutes with p99 < 500ms
- [ ] Security scan (govulncheck + trivy image scan) clean
- [ ] Runbook in `docs/runbook.md` reflects current behaviour

## Post-deploy gate (within 30 minutes of deploy)

- [ ] `/healthz` returns 200 from at least one production pod
- [ ] `/readyz` returns 200 from all production pods
- [ ] 5xx rate stays below MA baseline for 10 consecutive minutes
- [ ] p99 latency stays below MA baseline for 10 consecutive minutes
- [ ] No new `level=error layer=errors` log entries unrelated to client input
- [ ] DB pool `MaxOpenConnections` utilisation stays below 80%
- [ ] One canary API call (GET `/api/v1/users?limit=1` with a valid JWT)
      returns 200 with a non-empty `data[]`

If any post-deploy check fails, the release manager rolls back via
`docker compose up -d --build` against the previous image tag.
