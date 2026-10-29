// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package uow implements the Unit of Work pattern (DDD tactical) wired to
// the transactional outbox pattern for reliable event delivery.
//
// A Unit of Work bundles several repository mutations into a single
// transactional boundary: either all succeed and commit, or any failure
// rolls everything back. Events collected during the unit are persisted to
// the `outbox` table IN THE SAME TX as the business write (so they survive
// a crash between commit and bus dispatch), then dispatched to the
// enterprise bus AFTER commit. A background relay worker re-dispatches any
// rows the in-process dispatch missed (e.g. process died right after
// commit).
//
// At-least-once delivery: consumers MUST be idempotent (eventId dedup).
//
// Usage from a service:
//
//	err := uow.Run(ctx, func(tx uow.Tx) error {
//	    if err := tx.Users().Create(ctx, u); err != nil { return err }
//	    tx.Events().Append(UserCreated{ID: u.ID}) // persisted to outbox, dispatched post-commit
//	    return nil
//	})
//
// GAP CLOSED: #10 Unit of Work (transactions)
//             #12 Domain Events (proper DDD) — events flushed post-commit
//             Outbox pattern wired end-to-end (not just documented)
package uow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/ori-inc/hexagonal-go/internal/domain/events"
	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// OutboxWriter is the contract the UoW uses to persist events inside the tx.
// The concrete implementation lives in internal/infrastructure/outbox; we
// depend on the interface here so the domain layer stays free of SQL.
type OutboxWriter interface {
	AppendTx(ctx context.Context, tx *sqlx.Tx, e events.Event) error
}

// Tx is the in-flight transaction handed to the callback. It exposes the
// underlying *sqlx.Tx plus per-aggregate repositories bound to that tx.
type Tx interface {
	// SQLTx returns the underlying sqlx transaction. Repositories that need
	// raw access use this; everything else goes through the typed accessors.
	SQLTx() *sqlx.Tx

	// Events returns the in-memory event collector. Events appended here are
	// (a) persisted to the outbox table inside the tx, AND (b) dispatched to
	// the enterprise bus after commit. This dual-write is what makes
	// delivery reliable: if (b) fails, the outbox relay will retry.
	Events() *events.Collector
}

// RepositoryFactory builds a per-aggregate repository bound to a Tx. Services
// pass one in at Run time so the same UoW can be wired against many domains.
type RepositoryFactory func(tx *sqlx.Tx) Tx

// UoW owns a *sqlx.DB, a factory, a bus, and an optional outbox writer.
type UoW struct {
	db      *sqlx.DB
	factory RepositoryFactory
	bus     events.Dispatcher
	outbox  OutboxWriter // nil = in-process only (no persistence)
}

// New constructs a UoW with in-process event dispatch only (no outbox
// persistence). Use NewWithOutbox for production-grade delivery.
func New(db *sqlx.DB, factory RepositoryFactory, bus events.Dispatcher) *UoW {
	return &UoW{db: db, factory: factory, bus: bus}
}

// NewWithOutbox constructs a UoW that persists events to the outbox table
// inside the same tx as the business write, then dispatches post-commit.
// This is the production-grade constructor.
func NewWithOutbox(db *sqlx.DB, factory RepositoryFactory, bus events.Dispatcher, ob OutboxWriter) *UoW {
	return &UoW{db: db, factory: factory, bus: bus, outbox: ob}
}

// Run executes fn inside a single DB transaction. On fn error or panic the
// tx rolls back; on success it commits and then dispatches every event
// collected during fn via the enterprise bus.
//
// If an outbox writer is configured, events are persisted to the outbox
// table BEFORE commit (so they ride the same tx) — this is the key
// invariant that makes delivery reliable.
func (u *UoW) Run(ctx context.Context, fn func(tx Tx) error) (err error) {
	L := logger.Ctx(ctx, logger.LayerService)

	tx, err := u.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("uow begin: %w", err)
	}

	// Ensure rollback on panic or error. committed flag prevents a double
	// rollback when commit already succeeded.
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				L.Error().Err(rbErr).Msg("UOW_ROLLBACK_FAILED")
			} else {
				L.Info().Msg("UOW_ROLLED_BACK")
			}
		}
	}()

	t := u.factory(tx)
	if err = fn(t); err != nil {
		return err
	}

	// --- OUTBOX PERSISTENCE (inside the tx) --------------------------------
	// Persist every collected event to the outbox table so delivery survives
	// a crash between commit and bus dispatch. This is the heart of the
	// transactional outbox pattern.
	collected := t.Events().Drain()
	if u.outbox != nil {
		for _, e := range collected {
			if err := u.outbox.AppendTx(ctx, tx, e); err != nil {
				return fmt.Errorf("uow outbox append %s: %w", e.Topic(), err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("uow commit: %w", err)
	}
	committed = true
	L.Info().Int("events", len(collected)).Msg("UOW_COMMITTED")

	// --- POST-COMMIT DISPATCH (best-effort) --------------------------------
	// The relay worker will retry any events we fail to dispatch here.
	for _, e := range collected {
		if dErr := u.bus.Dispatch(ctx, e); dErr != nil {
			logger.Ctx(ctx, logger.LayerEnterpriseBus).
				Error().Err(dErr).
				Str("topic", e.Topic()).
				Str("eventId", e.EventID()).
				Msg("UOW_EVENT_DISPATCH_FAILED_RELAY_WILL_RETRY")
		} else {
			logger.Ctx(ctx, logger.LayerEnterpriseBus).
				Debug().
				Str("topic", e.Topic()).
				Str("eventId", e.EventID()).
				Msg("UOW_EVENT_DISPATCHED")
		}
	}
	return nil
}

// concreteTx is the trivial Tx implementation that just carries the *sqlx.Tx
// and an event collector. The factory is responsible for attaching typed
// repository accessors; if it doesn't, callers fall back to SQLTx().
type concreteTx struct {
	sqlTx  *sqlx.Tx
	events *events.Collector
}

func (c *concreteTx) SQLTx() *sqlx.Tx          { return c.sqlTx }
func (c *concreteTx) Events() *events.Collector { return c.events }

// NewConcreteTx is the helper a typical RepositoryFactory uses.
func NewConcreteTx(sqlTx *sqlx.Tx) Tx {
	return &concreteTx{sqlTx: sqlTx, events: events.NewCollector()}
}

// jsonMarshal is a tiny helper kept local so we don't pull encoding/json into
// every caller's import list. Used by outbox adapters that serialize event
// payloads.
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

var _ = jsonMarshal // keep the import live for future use
