# 12-Layer Logging Matrix

Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
Author: Awase Khirni Syed

Every log line carries a `layer` field whose value is one of the twelve
identifiers below. A query like `level=error layer=repository` instantly
isolates the failing slice of the architecture without reading the message
text. The `requestId` field (stamped by `middleware.RequestID`) lets a
single user-reported id grep the entire fan-out of a request across all
twelve layers.

| # | Layer | What gets logged | Level | Context attached | Source file |
|---|-------|------------------|-------|------------------|-------------|
| 1 | `http` | method, path, status, duration, bytes | info (warn ≥400, error ≥500) | requestId, userId, ip, userAgent | `internal/middleware/middleware.go` (HTTPLogger) |
| 2 | `middleware` | auth outcome, rate-limit hits, cors, rbac, audit | debug/info/warn | requestId, userId | `internal/middleware/*.go` |
| 3 | `controller` | action, params, response status | info | requestId, userId, action | `internal/api/controllers/user/controller.go` |
| 4 | `validation` | field-level errors, value coercion | warn | requestId, field, value | `internal/api/validations/user/validation.go` |
| 5 | `mapper` | entity↔DTO conversion trace | debug | requestId, mapperName | `internal/api/mappers/user/mapper.go`, `internal/infrastructure/repositories/user/repository.go` |
| 6 | `service` | business operation enter/exit, timing | info/debug | requestId, userId, operation | `internal/application/services/user/service.go` |
| 7 | `repository` | CRUD with rows-affected, SQL timing | info/debug | requestId, entity, op, rowsAffected, durationMs | `internal/domain/repository/base.go` (Observe) |
| 8 | `database` | pool lifecycle, slow queries (>100ms), connection | info/warn | requestId, poolStats | `internal/database/postgres.go`, `internal/domain/repository/base.go` |
| 9 | `domain_event` | publish/subscribe, handler success/failure | debug/info | requestId, topic, eventId | `internal/domain/events/event.go` |
| 10 | `enterprise_bus` | dispatch fan-out, handler errors | debug/error | requestId, topic, handler | `internal/domain/bus/bus.go` |
| 11 | `errors` | stable code, stack, cause, traceId | error | requestId, code, stack | `internal/httpx/envelope.go` (WriteError) |
| 12 | `lifecycle` | boot, shutdown, signals, drain | info/warn | signal, inFlightRequests | `internal/server/server.go`, `internal/database/postgres.go` |

## How to query

| Question | Query (Loki / kubectl logs / Datadog) |
|----------|----------------------------------------|
| Why did request X fail? | `requestId="X" and level>=warn` |
| What was the slowest SQL call in the last hour? | `layer="repository" and durationMs>100ms` |
| Are domain events being dispatched? | `layer="enterprise_bus" and msg="BUS_HANDLER_OK"` |
| Did the bus drop any events? | `layer="enterprise_bus" and msg="BUS_HANDLER_ERROR"` |
| What mutations did user Y make? | `layer="middleware" and audit="true" and userId="Y"` |
| Did the process drain cleanly on shutdown? | `layer="lifecycle" and msg="DRAIN_COMPLETE"` |
| Did any validation reject a request? | `layer="validation" and msg="VALIDATION_FAILED"` |
