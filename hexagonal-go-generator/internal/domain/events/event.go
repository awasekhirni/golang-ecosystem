// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package events implements Domain Events (DDD tactical pattern).
//
// An Event is something that happened in the past tense in the domain
// (UserCreated, OrderShipped, PaymentRefunded). Producers don't know who
// consumes events; consumers subscribe by topic.
//
// Layer-9 logging (Domain Events) stamps eventId + topic + traceId + userId.
//
// GAP CLOSED: #12 Domain Events (proper DDD)
package events

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/ori-inc/hexagonal-go/internal/logger"
	"github.com/ori-inc/hexagonal-go/pkg/metrics"
)

// Event is the marker interface every domain event implements.
type Event interface {
	Topic() string
	EventID() string
	OccurredAt() time.Time
}

// Base is embedded into concrete events to provide id+timestamp plumbing.
type Base struct {
	ID   string    `json:"eventId"`
	Time time.Time `json:"occurredAt"`
}

func (b Base) EventID() string        { return b.ID }
func (b Base) OccurredAt() time.Time  { return b.Time }

// NewBase returns a Base with a fresh id and now-UTC timestamp.
func NewBase() Base {
	return Base{ID: uuid.NewString(), Time: time.Now().UTC()}
}

// Handler is a typed consumer of events on a given topic.
type Handler interface {
	Topic() string
	Handle(ctx context.Context, e Event) error
}

// HandlerFunc is the function adapter for Handler.
type HandlerFunc struct {
	TopicStr string
	Fn       func(ctx context.Context, e Event) error
}

func (h HandlerFunc) Topic() string                              { return h.TopicStr }
func (h HandlerFunc) Handle(ctx context.Context, e Event) error { return h.Fn(ctx, e) }

// Dispatcher is the interface the enterprise bus implements. We split it out
// so the UoW can be wired against a fake dispatcher in tests.
type Dispatcher interface {
	Dispatch(ctx context.Context, e Event) error
	Subscribe(h Handler)
}

// Collector accumulates events inside a Unit of Work without dispatching
// them. The UoW drains the collector and dispatches each event only after
// the tx commits successfully.
type Collector struct {
	events []Event
}

func NewCollector() *Collector           { return &Collector{} }
func (c *Collector) Append(e Event)      { c.events = append(c.events, e) }
func (c *Collector) Drain() []Event      { out := c.events; c.events = nil; return out }

// Publish is a convenience: it bumps the metrics counter and logs at the
// Domain Events layer. Real dispatch goes through the bus.
func Publish(ctx context.Context, bus Dispatcher, e Event) {
	L := logger.Ctx(ctx, logger.LayerDomainEvent)
	metrics.DomainEventsPublished.WithLabelValues(e.Topic()).Inc()
	L.Info().
		Str("topic", e.Topic()).
		Str("eventId", e.EventID()).
		Str("traceId", logger.TraceIDFrom(ctx)).
		Str("userId", logger.UserIDFrom(ctx)).
		Msg("DOMAIN_EVENT_PUBLISHED")
	if bus != nil {
		if err := bus.Dispatch(ctx, e); err != nil {
			L.Error().Err(err).Msg("DOMAIN_EVENT_DISPATCH_FAILED")
		}
	}
}
