# CMMI Level 5 Process Asset Library (PAL)

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

This directory is the **Process Asset Library** for the Hexagonal Go Architecture
project. It contains the eleven documents required for CMMI Level 5 (Optimizing)
process maturity. Each document is intentionally short and prescriptive so it
can actually be followed rather than become shelfware.

| # | Document | CMMI PA | Purpose |
|---|----------|---------|---------|
| 00 | PAL Index | OPF | This index |
| 01 | Organizational Standard Process (OSSP) | OPF, OPD | The canonical lifecycle |
| 02 | Measurement & Analysis (MA) | MA | What we measure and how |
| 03 | Quality Gates (PPQA) | PPQA | Pre-merge / pre-release / post-deploy checks |
| 04 | Risk Management (RSKM) | RSKM | Top-N risk register |
| 05 | Configuration Management (CM) | CM | Branching, versioning, releases |
| 06 | Causal Analysis & Resolution (CAR) | CAR | 5-Whys + Ishikawa templates |
| 07 | Organizational Innovation & Deployment (OID) | OID | How new ideas roll out |
| 08 | Continuous Improvement (Optimizing loop) | OID, OPF | The closed feedback loop |
| 09 | Decision Analysis & Resolution (DAR) | DAR | Evaluated decision matrix |
| 10 | Verification & Validation (VER/VAL) | VER, VAL | Test pyramid policy |

## How to use the PAL

1. Every change to the codebase touches the appropriate PAL doc.
2. Every retrospective produces a CAR entry (06) that feeds Continuous
   Improvement (08).
3. PPQA (03) gates are enforced in `.github/workflows/ci.yml`.
4. MA (02) baselines are exported from `/metrics` and reviewed monthly.

---
