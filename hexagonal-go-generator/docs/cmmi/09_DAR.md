# 09 - Decision Analysis & Resolution (DAR)

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

## When to use DAR

DAR is mandatory for any decision where:
- The choice is irreversible (e.g. picking a database engine).
- The choice affects ≥3 PAL docs.
- The choice has a risk score ≥12 in the register (04).
- Engineering lead or architect requests it.

## Template

```
Decision: <one-sentence question, framed as a question>

Criteria (each weighted 1-5):
  C1: <criterion> - weight W1
  C2: <criterion> - weight W2
  ...

Alternatives:
  A1: <option>
  A2: <option>
  A3: <option>

Score matrix (1-5 per cell; multiply by weight):

           | C1 (w=W1) | C2 (w=W2) | ... | Total |
  ---------|-----------|-----------|-----|-------|
  A1       |     x     |     y     | ... |  Σ    |
  A2       |     x     |     y     | ... |  Σ    |
  A3       |     x     |     y     | ... |  Σ    |

Selected: A? because <one paragraph rationale>

Risks of selected alternative: <list, with link to RSKM (04) entries>

Rollback: <how do we undo this if it goes wrong?>

Owner: @<person>   Date: YYYY-MM-DD   Status: proposed|accepted|superseded
```

## Worked example (referenced by other PAL docs)

Decision: "Which PostgreSQL driver for the reference impl?"
- C1: Observability hooks (pgx native slow-query, prepared statements) — w5
- C2: Mature sqlx-compatible interface — w3
- C3: Active maintenance — w4

| | pgx/v5 | lib/pq | pgx+sqlx |
|---|---|---|---|
| C1 (w5) | 5 | 3 | 5 |
| C2 (w3) | 3 | 5 | 5 |
| C3 (w4) | 5 | 2 | 5 |
| Total | 50 | 34 | 60 |

Selected: pgx + sqlx (registered as `pgx` driver via `_ "github.com/jackc/pgx/v5/stdlib"`).
Rollback: swap driver string in `database.New()` — no code changes elsewhere.
