# 08 - Continuous Improvement (the Optimizing loop)

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

CMMI Level 5 ("Optimizing") is distinguished from Level 4 ("Quantitatively
Managed") by an explicit, measured, closed feedback loop. This document is
that loop.

## The loop

```
   ┌────────────────────────────────────────────────────────────┐
   │                                                            ▼
 MA (02) ──► SPC signal ──► CAR (06) ──► action items ──► OID (07) rollout
   ▲                                                            │
   │                                                            ▼
   └───────────────── metrics re-observed ◄── PPQA (03) gates ◄┘
```

1. **MA** continuously samples metrics from `/metrics`.
2. **SPC** detects Out-of-Control points and opens a CAR candidate.
3. **CAR** performs 5-Whys + Ishikawa, produces action items.
4. **OID** rolls the action items out as PAL patches + code.
5. **PPQA** gates each rollout at pre-merge / pre-release / post-deploy.
6. **MA** re-samples; if the metric improves, the loop closes. If not, a new
   CAR is filed.

## Cadence

| Activity | Frequency | Owner |
|----------|-----------|-------|
| SPC sampling | continuous | Prometheus + Alertmanager |
| CAR triage | weekly | Engineering lead |
| OID rollout review | per-PR | Reviewer |
| PPQA post-deploy gate | per-deploy | Release manager |
| PAL doc audit | monthly | Engineering lead |
| SPC baseline recompute | monthly | SRE |
| Full PAL review | quarterly | Eng + SRE + Sec |

## Improvement log

Every closed CAR adds one row to `docs/improvements/log.md` (created on
first use) with: date, CAR id, metric improved, before/after values. This
log is the audit trail that CMMI appraisers review.
