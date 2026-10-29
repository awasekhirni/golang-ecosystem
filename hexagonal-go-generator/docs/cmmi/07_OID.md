# 07 - Organizational Innovation & Deployment (OID)

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

## What counts as an innovation

Any change to one of these is an "innovation" requiring OID treatment:

- A PAL document (00-10).
- A new tactical pattern (e.g. introducing the Outbox pattern).
- A new tool (e.g. switching from Loki to Datadog).
- A new aggregate (e.g. adding `Order`).
- A new non-trivial library dependency.

## Deployment lifecycle

1. **Proposal** (1 paragraph) in `docs/innovations/<short-name>.md`.
2. **DAR evaluation** using the template in 09. Result: adopt / pilot / reject.
3. **Pilot** on one aggregate or one environment. Pilot must declare success
   metrics in advance and run for a fixed window (≥2 weeks).
4. **Retrospective** comparing pilot metrics to baseline. Filed as a CAR
   candidate if pilot failed.
5. **Rollout plan** with sequenced PRs; each PR sized for review.
6. **PAL update** — the relevant PAL doc is patched in the same PR as the
   rollout so the doc is never stale.
7. **Training** — a 15-minute walkthrough at the next engineering meeting.

## Anti-patterns explicitly forbidden

- "Big-bang" refactors across all aggregates in one PR.
- PAL docs updated after the rollout (the doc must lead, not follow).
- Innovations deployed without a success metric.
- Innovations deployed without a rollback plan.
