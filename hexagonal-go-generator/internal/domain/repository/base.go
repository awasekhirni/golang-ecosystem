// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package repository provides the Enterprise Repository base that every
// per-aggregate repository embeds. It centralizes:
//
//   - timing + metrics for every CRUD call (Layer-7 Repository logging)
//   - slow-query auto-WARN (>100ms)
//   - Specification -> SQL rendering (so concrete repos don't repeat it)
//   - pagination helpers (offset + cursor)
//   - rows-affected logging
//
// Concrete repos embed Base and add typed methods. This is the "enterprise
// repository pattern" the brief asks for: one consistent contract across
// every aggregate, with cross-cutting concerns factored out.
//
// GAP CLOSED: Enterprise repository pattern
//             #19 Per-method logging with timings (Repository layer)
package repository

import (
        "context"
        "fmt"
        "strings"
        "time"

        "github.com/jmoiron/sqlx"
        "github.com/ori-inc/hexagonal-go/internal/domain/specification"
        "github.com/ori-inc/hexagonal-go/internal/logger"
        "github.com/ori-inc/hexagonal-go/internal/resilience"
        "github.com/ori-inc/hexagonal-go/pkg/metrics"
)

// Base is embedded by every concrete repository.
type Base struct {
        DB      *sqlx.DB
        Entity  string // e.g. "user" - used in logs and metrics labels
        Breaker *resilience.CircuitBreaker
}

// NewBase returns a Base bound to a pool and an entity label.
// A circuit breaker is auto-created so every Observe-wrapped call is
// protected against a cascading DB failure.
func NewBase(db *sqlx.DB, entity string) Base {
        return Base{
                DB:      db,
                Entity:  entity,
                Breaker: resilience.NewCircuitBreaker("db."+entity, 5, 30*time.Second),
        }
}

// Observe records metrics + a Layer-7 log line, and WARNs on slow query.
// Repositories call `defer r.Observe(ctx, "get_by_id", start, rows, err)`.
//
// Exported because concrete repositories live in a different package and
// must call it via the embedded Base.
func (b Base) Observe(ctx context.Context, op string, start time.Time, rowsAffected int64, err error) {
        elapsed := time.Since(start)
        metrics.ObserveDB(op, b.Entity, start)

        evt := logger.Ctx(ctx, logger.LayerRepository).
                Info().
                Str("entity", b.Entity).
                Str("op", op).
                Int64("rowsAffected", rowsAffected).
                Dur("durationMs", elapsed)
        if err != nil {
                evt = evt.Err(err)
        }
        evt.Msg("REPOSITORY_CALL")

        if elapsed > 100*time.Millisecond {
                logger.Ctx(ctx, logger.LayerDatabase).
                        Warn().
                        Str("entity", b.Entity).
                        Str("op", op).
                        Dur("durationMs", elapsed).
                        Msg("SLOW_QUERY")
        }
}

// Execute wraps a DB closure with the circuit breaker + retry policy. Use it
// for any call that hits the network (DB or upstream) so a cascading failure
// trips the breaker instead of starving the goroutine pool.
//
//      err := r.Execute(ctx, "get_by_id", func(ctx context.Context) error {
//          return r.DB.GetContext(ctx, &m, "SELECT ...", id)
//      })
func (b Base) Execute(ctx context.Context, op string, fn func(context.Context) error) error {
        if b.Breaker == nil {
                return fn(ctx)
        }
        policy := resilience.DefaultRetryPolicy()
        policy.MaxAttempts = 2 // DB calls rarely benefit from more
        policy.IsRetryable = func(err error) bool {
                if err == nil {
                        return false
                }
                return !isNonRetryable(err)
        }
        return b.Breaker.Execute(ctx, func(ctx context.Context) error {
                return resilience.Retry(ctx, b.Entity+"."+op, policy, fn)
        })
}

// isNonRetryable returns true for errors that should NOT be retried (e.g.
// unique constraint, foreign key, not found). Conservative: we only treat
// obvious non-retryables as such; everything else gets retried.
func isNonRetryable(err error) bool {
        if err == nil {
                return true
        }
        msg := err.Error()
        for _, marker := range []string{"unique constraint", "foreign key", "no rows", "duplicate key"} {
                if strings.Contains(msg, marker) {
                        return true
                }
        }
        return false
}

// Render turns a Specification into a parameterized SQL WHERE clause suitable
// for sqlx. `?` placeholders are converted to Postgres `$N`.
func (b Base) Render(base string, spec specification.Spec[any]) (string, []any, error) {
        if spec == nil {
                return base, nil, nil
        }
        q := spec.ToQuery()
        sql, args := specification.RenderQuery(base, q)
        return sql, args, nil
}

// OffsetPage runs `SELECT ... ORDER BY $orderCol $orderDir LIMIT $limit OFFSET $offset`.
// `base` must NOT contain LIMIT/OFFSET - the helper appends them.
// `countBase` is run with the same WHERE to compute the total.
func (b Base) OffsetPage(ctx context.Context, base, countBase, orderCol string,
        orderDesc bool, offset, limit int, spec specification.Spec[any],
        dest any) (total int64, err error) {

        start := time.Now()
        defer func() { b.Observe(ctx, "list_offset", start, 0, err) }()

        where, args, err := b.Render(base, spec)
        if err != nil {
                return 0, err
        }

        // Count
        countSQL := countBase
        if where != base && len(args) > 0 {
                countSQL, _, _ = b.Render(countBase, spec)
        }
        if err := b.DB.GetContext(ctx, &total, countSQL, args...); err != nil {
                return 0, fmt.Errorf("count: %w", err)
        }

        // Page
        dir := "ASC"
        if orderDesc {
                dir = "DESC"
        }
        pageSQL := where + fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d",
                orderCol, dir, len(args)+1, len(args)+2)
        pageArgs := append(args, limit, offset)
        if err := b.DB.SelectContext(ctx, dest, pageSQL, pageArgs...); err != nil {
                return 0, fmt.Errorf("select: %w", err)
        }
        return total, nil
}

// CursorPage runs a cursor-style query: WHERE ($cursorCol $cmp $cursorValue)
// AND (id $cmp $lastID) ORDER BY $cursorCol, id LIMIT $limit+1.
// The extra row lets the caller decide hasNext.
//
// Concrete repos usually wrap this with their own type-safe signature.
func (b Base) CursorPage(ctx context.Context, base, orderCol, cursorValue, lastID string,
        limit int, dest any) (err error) {
        start := time.Now()
        defer func() { b.Observe(ctx, "list_cursor", start, 0, err) }()

        sql := base + fmt.Sprintf(" ORDER BY %s ASC, id ASC LIMIT $1", orderCol)
        return b.DB.SelectContext(ctx, dest, sql, limit+1)
}
