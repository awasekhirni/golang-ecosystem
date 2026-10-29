# 06 - Causal Analysis & Resolution (CAR)

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

## When to file a CAR

- Any Sev-1 or Sev-2 incident.
- Any SPC Out-of-Control signal from the MA plan (02).
- Any risk that hits score ≥15 in the register (04).
- Any repeated bug in the same aggregate within 30 days.

## 5-Whys template

```
Problem statement: <one-sentence description, observed effect>

Why 1: <root cause layer 1>
Why 2: <root cause layer 2>
Why 3: <root cause layer 3>
Why 4: <root cause layer 4>
Why 5: <deepest actionable cause>

Root cause: <one-sentence summary of Why 5>

Action items (each must have an owner + due date):
1. <action> - @owner - YYYY-MM-DD
2. <action> - @owner - YYYY-MM-DD
3. <action> - @owner - YYYY-MM-DD

Verification: how will we know this won't recur? -> metric / test / gate.
```

## Ishikawa (fishbone) categories

For complex incidents, branch the 5-Whys across the 6 Ms:

- **Method** (process): was the OSSP (01) followed?
- **Machine** (infra): DB, network, container runtime.
- **Material** (data): was the input schema valid? migration state?
- **Man** (people): was the reviewer qualified? runbook followed?
- **Measurement** (observability): did MA (02) catch it? why not?
- **Milieu** (environment): env vars, configs, secrets.

## Outputs

Each CAR produces:
1. One or more action items in the tracker.
2. A patch to a PAL doc (often 03 PPQA or 02 MA) — process changes are
   first-class artefacts.
3. An entry in the Continuous Improvement log (08).
