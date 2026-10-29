// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package bus is the Enterprise Bus - an in-process pub/sub that fans out a
// single domain event to every registered handler for the topic. Each handler
// is invoked in its own goroutine; a panic or error in one handler does NOT
// stop the others, and is logged at the Enterprise Bus layer with the
// handler name so it can be retried or alerted on.
//
// For multi-process fan-out (Kafka, NATS, SQS, RabbitMQ) replace
// InMemoryBus with a thin adapter that publishes to the broker; the
// Dispatcher interface stays identical.
//
// Layer-10 logging (Enterprise Bus) stamps topic, handler, success/failure.
//
// GAP CLOSED: Enterprise bus pattern (and supports #12 Domain Events)
package bus

import (
	"context"
	"sync"

	"github.com/ori-inc/hexagonal-go/internal/domain/events"
	"github.com/ori-inc/hexagonal-go/internal/logger"
	"github.com/ori-inc/hexagonal-go/pkg/metrics"
)

// InMemoryBus is the default in-process bus. It is goroutine-safe and uses
// one goroutine per handler invocation, so slow handlers don't block the
// publisher. Use a bounded worker pool if you need backpressure.
type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]events.Handler
}

// New returns an empty bus.
func New() *InMemoryBus {
	return &InMemoryBus{handlers: map[string][]events.Handler{}}
}

// Subscribe registers a handler for a topic. Multiple handlers per topic are
// allowed; each gets every event.
func (b *InMemoryBus) Subscribe(h events.Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	topic := h.Topic()
	b.handlers[topic] = append(b.handlers[topic], h)
	logger.For(logger.LayerEnterpriseBus).
		Info().Str("topic", topic).Msg("BUS_HANDLER_SUBSCRIBED")
}

// Dispatch fans out e to every handler registered for e.Topic(). Errors in
// individual handlers are logged and counted but never propagated to the
// publisher (the event already happened; we cannot un-happen it).
func (b *InMemoryBus) Dispatch(ctx context.Context, e events.Event) error {
	L := logger.Ctx(ctx, logger.LayerEnterpriseBus)

	b.mu.RLock()
	hs := b.handlers[e.Topic()]
	b.mu.RUnlock()

	if len(hs) == 0 {
		L.Debug().Str("topic", e.Topic()).Str("eventId", e.EventID()).Msg("BUS_NO_HANDLERS")
		return nil
	}

	L.Info().
		Str("topic", e.Topic()).
		Str("eventId", e.EventID()).
		Int("handlerCount", len(hs)).
		Msg("BUS_DISPATCH_FANOUT")

	var wg sync.WaitGroup
	for _, h := range hs {
		h := h // capture for goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					metrics.BusDispatchErrors.WithLabelValues(e.Topic(), nameOf(h)).Inc()
					logger.Ctx(ctx, logger.LayerEnterpriseBus).
						Error().
						Interface("panic", rec).
						Str("topic", e.Topic()).
						Str("handler", nameOf(h)).
						Msg("BUS_HANDLER_PANIC")
				}
			}()
			if err := h.Handle(ctx, e); err != nil {
				metrics.BusDispatchErrors.WithLabelValues(e.Topic(), nameOf(h)).Inc()
				logger.Ctx(ctx, logger.LayerEnterpriseBus).
					Error().
					Err(err).
					Str("topic", e.Topic()).
					Str("handler", nameOf(h)).
					Str("eventId", e.EventID()).
					Msg("BUS_HANDLER_ERROR")
				return
			}
			logger.Ctx(ctx, logger.LayerEnterpriseBus).
				Debug().
				Str("topic", e.Topic()).
				Str("handler", nameOf(h)).
				Str("eventId", e.EventID()).
				Msg("BUS_HANDLER_OK")
		}()
	}
	wg.Wait()
	return nil
}

// nameOf returns a human-readable handler name. Concrete handlers should
// implement a `String()` method for nicer logs.
func nameOf(h events.Handler) string {
	if s, ok := h.(interface{ String() string }); ok {
		return s.String()
	}
	return "anonymous"
}
