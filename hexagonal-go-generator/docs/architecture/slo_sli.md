# SLOs, SLIs, and Error Budgets

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

This document defines the Service-Level Objectives for the Hexagonal Go
service. It is the operational contract between engineering and the rest
of the business.

## SLIs (indicators)

| SLI | Source query (Prometheus) | Window |
|-----|---------------------------|--------|
| Availability | `1 - (sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])))` | 5m rolling |
| Latency p99 | `histogram_quantile(0.99, sum by (route, le) (rate(http_request_duration_seconds_bucket[5m])))` | 5m rolling |
| Throughput | `sum(rate(http_requests_total[5m]))` | 5m rolling |
| DB pool saturation | `db_connections_in_use / db_connections_max` | 1m rolling |
| Bus dispatch error rate | `sum(rate(bus_dispatch_errors_total[5m]))` | 5m rolling |

## SLOs (objectives)

| SLO | Target | Window |
|-----|--------|--------|
| Availability (non-5xx) | 99.9% | 30 days |
| Latency p99 < 500ms | 99% | 30 days |
| Bus dispatch error rate < 0.1% | 99.9% | 30 days |

## Error budget

The 30-day error budget for 99.9% availability is:

  30 days × 86,400 s/day × (1 − 0.999) = **2,592 s ≈ 43.2 minutes**

Engineering may spend this budget on:
- Feature rollouts that briefly impact availability.
- Planned maintenance.
- Risky migrations.

If the budget is exhausted, all non-reliability feature work is frozen
until the budget recovers (see Continuous Improvement loop in 08).

## Alerting rules

Alertmanager rules derived from the SLOs:

```yaml
groups:
  - name: hexagonal-go
    rules:
      - alert: HighErrorRate
        expr: |
          sum(rate(http_requests_total{status=~"5.."}[5m]))
            / sum(rate(http_requests_total[5m])) > 0.01
        for: 5m
        labels: { severity: page }
        annotations:
          summary: "5xx rate > 1% for 5 minutes"

      - alert: HighLatencyP99
        expr: |
          histogram_quantile(0.99,
            sum by (route, le) (rate(http_request_duration_seconds_bucket[5m]))
          ) > 0.5
        for: 10m
        labels: { severity: page }
        annotations:
          summary: "p99 latency > 500ms for 10 minutes"

      - alert: BusDispatchErrors
        expr: rate(bus_dispatch_errors_total[5m]) > 0
        for: 5m
        labels: { severity: ticket }
        annotations:
          summary: "Enterprise bus handler failures"

      - alert: DBPoolSaturation
        expr: |
          pgxpool_active_connections / pgxpool_max_connections > 0.8
        for: 5m
        labels: { severity: page }
        annotations:
          summary: "DB pool >80% saturated"
```

## Validation harness

The k6 script at `scripts/k6/users_load.js` validates the latency and
error-rate SLOs in CI. It ramps 0 → 100 VUs over 60s and fails the run
if p99 > 500ms or error rate > 1%.
