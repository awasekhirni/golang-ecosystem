# Security Policy

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

## Supported versions

Only the latest minor release receives security updates. Users should
always run the latest patch.

## Reporting a vulnerability

Email security@ori-inc.ca with:
- A description of the issue and its impact
- Steps to reproduce (PoC, screenshots, or traces)
- Affected versions, if known

We acknowledge within 48 hours and target a fix within 30 days. Critical
issues get a hotfix release within 7 days. Please do not file public
GitHub issues for security reports.

## Security measures implemented

This codebase ships with the following controls (see README §3 and §3a):

- **AuthN**: JWT (HS256) + static API keys; either one authenticates.
- **AuthZ**: RBAC middleware with role-set intersection.
- **Transport**: HSTS + TLS in production (enforced at the ingress layer).
- **Headers**: OWASP A05:2021 security headers on every response.
- **Body**: 1 MiB body size limit (413 on overflow).
- **Rate limit**: per-IP token bucket (default 50 rps, 100 burst).
- **PII**: auto-redaction of email/phone/SSN/cc/JWT in every log line.
- **Audit**: SHA-256 body digest for every mutation (POST/PUT/PATCH/DELETE).
- **Supply chain**: CycloneDX SBOM generated on every release
  (`scripts/sbom/sbom.sh`).
- **Secrets**: env validation at boot; production refuses to start if
  `JWT_SECRET` is < 32 bytes.

## Dependency scanning

CI runs `govulncheck` on every push and PR. The Docker image is scanned
with `trivy` before any release tag is pushed.

## Threat model summary

| Threat | Control |
|--------|---------|
| Stolen JWT | 15m access TTL + refresh rotation (out of scope of reference impl) |
| Replay of mutation | Idempotency-Key middleware (24h dedup) |
| Lost domain event | Transactional outbox + relay worker |
| Cascading DB failure | Circuit breaker in repository.Base.Execute |
| Log leak of PII | ScrubbingWriter auto-redacts every log line |
| DoS via large body | MaxBodySize middleware (1 MiB) |
| DoS via slowloris | Per-request HTTP timeout (20s write) |
| Privilege escalation | RBAC middleware on every authenticated route |
