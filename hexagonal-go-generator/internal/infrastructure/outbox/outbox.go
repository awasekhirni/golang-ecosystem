// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package outbox implements the transactional outbox pattern.
//
// Problem: if a service writes to the DB and then publishes to a message
// bus, a crash between the two means a lost event. The outbox pattern fixes
// this by writing the event into an `outbox` table IN THE SAME TX as the
// business write, then a relay worker polls the table and publishes pending
// events to the bus. At-least-once delivery; consumers must be idempotent
// (the eventId makes that trivial).
//
// Schema (auto-created by the migration runner):
//
//   CREATE TABLE outbox (
//     id           BIGSERIAL PRIMARY KEY,
//     event_id     UUID        NOT NULL UNIQUE,
//     topic        TEXT        NOT NULL,
//     payload      JSONB       NOT NULL,
//     created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
//     processed_at TIMESTAMPTZ
//   );
//
// GAP CLOSED: Outbox pattern (reliable event delivery, no lost events)
package outbox

import (
        "context"
        "encoding/json"
        "time"

        "github.com/jmoiron/sqlx"
        "github.com/ori-inc/hexagonal-go/internal/domain/events"
        "github.com/ori-inc/hexagonal-go/internal/logger"
)

// Outbox writes events into the outbox table. The same *sqlx.Tx used for the
// business write MUST be passed so the insert is atomic with it.
type Outbox struct{}

// New returns the outbox writer.
func New() *Outbox { return &Outbox{} }

// Append inserts e into the outbox table using tx. This is the method that
// satisfies uow.OutboxWriter.
func (o *Outbox) AppendTx(ctx context.Context, tx *sqlx.Tx, e events.Event) error {
        return o.Append(ctx, tx, e)
}

// Append inserts e into the outbox table using tx.
func (o *Outbox) Append(ctx context.Context, tx *sqlx.Tx, e events.Event) error {
        payload, err := json.Marshal(e)
        if err != nil {
                return err
        }
        _, err = tx.ExecContext(ctx,
                `INSERT INTO outbox (event_id, topic, payload) VALUES ($1, $2, $3)`,
                e.EventID(), e.Topic(), payload)
        if err != nil {
                return err
        }
        logger.Ctx(ctx, logger.LayerDomainEvent).
                Info().
                Str("eventId", e.EventID()).
                Str("topic", e.Topic()).
                Msg("OUTBOX_APPENDED")
        return nil
}

// EnsureSchema creates the outbox table if missing. Idempotent.
func EnsureSchema(ctx context.Context, db *sqlx.DB) error {
        _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     UUID        NOT NULL UNIQUE,
    topic        TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_outbox_unprocessed ON outbox(id) WHERE processed_at IS NULL;`)
        return err
}

// row is the on-disk shape of one outbox row.
type row struct {
        ID        int64           `db:"id"`
        EventID   string          `db:"event_id"`
        Topic     string          `db:"topic"`
        Payload   json.RawMessage `db:"payload"`
        CreatedAt time.Time       `db:"created_at"`
}

// Relay polls the outbox table and dispatches pending events to the bus.
// It marks each row processed_at=now() after a successful dispatch. Failed
// dispatches leave the row pending so the next poll retries them.
type Relay struct {
        db       *sqlx.DB
        bus      events.Dispatcher
        interval time.Duration
        batch    int
        stop     chan struct{}
}

// NewRelay constructs a relay that polls every `interval`, taking up to
// `batch` rows per pass.
func NewRelay(db *sqlx.DB, bus events.Dispatcher, interval time.Duration, batch int) *Relay {
        if batch <= 0 {
                batch = 100
        }
        return &Relay{db: db, bus: bus, interval: interval, batch: batch, stop: make(chan struct{})}
}

// Start runs the relay in a background goroutine until Stop is called.
func (r *Relay) Start(ctx context.Context) {
        go func() {
                t := time.NewTicker(r.interval)
                defer t.Stop()
                logger.For(logger.LayerLifecycle).
                        Info().
                        Dur("interval", r.interval).
                        Int("batch", r.batch).
                        Msg("OUTBOX_RELAY_STARTED")
                for {
                        select {
                        case <-r.stop:
                                logger.For(logger.LayerLifecycle).Info().Msg("OUTBOX_RELAY_STOPPED")
                                return
                        case <-ctx.Done():
                                return
                        case <-t.C:
                                _ = r.drain(ctx)
                        }
                }
        }()
}

// Stop signals the relay to exit.
func (r *Relay) Stop() { close(r.stop) }

func (r *Relay) drain(ctx context.Context) error {
        var rows []row
        err := r.db.SelectContext(ctx, &rows,
                `SELECT id, event_id, topic, payload, created_at
                   FROM outbox
                  WHERE processed_at IS NULL
                  ORDER BY id ASC
                  LIMIT $1 FOR UPDATE SKIP LOCKED`, r.batch)
        if err != nil {
                logger.For(logger.LayerEnterpriseBus).
                        Error().Err(err).Msg("OUTBOX_DRAIN_QUERY_FAILED")
                return err
        }
        for _, row := range rows {
                // We don't know the concrete event type at this layer; reconstruct a
                // generic envelope and dispatch. Real consumers receive the topic +
                // payload and deserialize themselves.
                evt := genericEvent{
                        id: row.EventID, topic: row.Topic, payload: row.Payload,
                        time: row.CreatedAt,
                }
                if err := r.bus.Dispatch(ctx, evt); err != nil {
                        logger.For(logger.LayerEnterpriseBus).
                                Error().
                                Err(err).
                                Str("eventId", row.EventID).
                                Str("topic", row.Topic).
                                Msg("OUTBOX_DISPATCH_FAILED")
                        continue // leave pending; retry next pass
                }
                _, _ = r.db.ExecContext(ctx,
                        `UPDATE outbox SET processed_at = now() WHERE id = $1`, row.ID)
                logger.For(logger.LayerEnterpriseBus).
                        Info().
                        Str("eventId", row.EventID).
                        Str("topic", row.Topic).
                        Msg("OUTBOX_DISPATCHED")
        }
        return nil
}

// genericEvent is a minimal events.Event implementation used when
// reconstructing from the outbox JSONB column.
type genericEvent struct {
        id     string
        topic  string
        payload json.RawMessage
        time   time.Time
}

func (g genericEvent) Topic() string             { return g.topic }
func (g genericEvent) EventID() string           { return g.id }
func (g genericEvent) OccurredAt() time.Time     { return g.time }
