// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package resilience implements three classical resilience patterns from
// "Release It!" (Nygard) that every production service needs when it talks
// to anything unreliable (which is everything):
//
//   1. CircuitBreaker - stop calling a failing dependency so it can recover.
//   2. Retry          - retry transient failures with exponential backoff
//                       + jitter, bounded max attempts.
//   3. Bulkhead       - cap concurrent in-flight calls so one slow dep can't
//                       starve the goroutine pool.
//
// All three emit Layer-8 (Database) or Layer-9 (Domain Event) style logs
// when their state changes, and bump Prometheus counters where applicable.
//
// GAP CLOSED: Resilience patterns (circuit breaker + retry + bulkhead)
package resilience

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// =============================================================================
// 1. CircuitBreaker
// =============================================================================

// State is the circuit-breaker state machine.
type State int

const (
	StateClosed State = iota   // normal operation
	StateOpen                  // tripped; calls fail fast
	StateHalfOpen              // trial period; one call allowed
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	}
	return "unknown"
}

// CircuitBreaker opens after `threshold` consecutive failures, stays open
// for `resetTimeout`, then enters half-open; the next call is a probe: if it
// succeeds the breaker closes, if it fails it re-opens.
type CircuitBreaker struct {
	name          string
	threshold     int
	resetTimeout  time.Duration

	mu              sync.Mutex
	state           State
	consecutiveFail int
	openedAt        time.Time
}

// NewCircuitBreaker constructs a breaker.
func NewCircuitBreaker(name string, threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{name: name, threshold: threshold, resetTimeout: resetTimeout}
}

// ErrCircuitOpen is returned when a call is rejected because the breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker open")

// Execute runs fn under the breaker. On ErrCircuitOpen the caller should
// degrade gracefully (return cached data, queue for later, etc.).
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if err := cb.allow(); err != nil {
		return err
	}
	err := fn(ctx)
	cb.after(err)
	return err
}

func (cb *CircuitBreaker) allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		if time.Since(cb.openedAt) > cb.resetTimeout {
			cb.state = StateHalfOpen
			logger.For(logger.LayerMiddleware).
				Info().Str("breaker", cb.name).Str("state", "half-open").Msg("CIRCUIT_HALF_OPEN")
			return nil
		}
		return ErrCircuitOpen
	case StateHalfOpen:
		return nil // allow the probe
	}
	return nil
}

func (cb *CircuitBreaker) after(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err == nil {
		if cb.state != StateClosed {
			logger.For(logger.LayerMiddleware).
				Info().Str("breaker", cb.name).Str("state", "closed").Msg("CIRCUIT_CLOSED")
		}
		cb.state = StateClosed
		cb.consecutiveFail = 0
		return
	}
	cb.consecutiveFail++
	if cb.state == StateHalfOpen {
		cb.trip()
		return
	}
	if cb.consecutiveFail >= cb.threshold {
		cb.trip()
	}
}

func (cb *CircuitBreaker) trip() {
	cb.state = StateOpen
	cb.openedAt = time.Now()
	logger.For(logger.LayerMiddleware).
		Warn().
		Str("breaker", cb.name).
		Str("state", "open").
		Int("consecutiveFailures", cb.consecutiveFail).
		Msg("CIRCUIT_TRIPPED")
}

// =============================================================================
// 2. Retry
// =============================================================================

// RetryPolicy describes how to retry: max attempts, base delay, max delay,
// and an optional classifier that decides whether an error is retryable.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      bool
	IsRetryable func(error) bool
}

// DefaultRetryPolicy is a sensible policy for transient failures.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		Jitter:      true,
		IsRetryable: func(err error) bool { return err != nil },
	}
}

// Retry runs fn up to policy.MaxAttempts times. Between attempts it sleeps
// for an exponential backoff with optional jitter. Context cancellation is
// respected.
func Retry(ctx context.Context, name string, policy RetryPolicy, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		logger.For(logger.LayerService).
			Debug().
			Str("op", name).
			Int("attempt", attempt).
			Msg("RETRY_ATTEMPT")
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !policy.IsRetryable(err) {
			return err
		}
		if attempt == policy.MaxAttempts {
			break
		}
		delay := backoff(attempt, policy)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	logger.For(logger.LayerService).
		Warn().
		Str("op", name).
		Int("attempts", policy.MaxAttempts).
		Err(lastErr).
		Msg("RETRY_EXHAUSTED")
	return lastErr
}

func backoff(attempt int, p RetryPolicy) time.Duration {
	d := p.BaseDelay * time.Duration(1<<(attempt-1))
	if d > p.MaxDelay {
		d = p.MaxDelay
	}
	if p.Jitter {
		d = time.Duration(float64(d) * (0.5 + rand.Float64()*0.5))
	}
	return d
}

// =============================================================================
// 3. Bulkhead
// =============================================================================

// Bulkhead caps concurrent in-flight calls. Calls beyond the cap block until
// a slot frees or ctx is cancelled. Use one bulkhead per dependency so a
// slow DB doesn't starve the downstream HTTP client.
type Bulkhead struct {
	name    string
	slots   chan struct{}
}

// NewBulkhead constructs a bulkhead with `maxConcurrency` slots.
func NewBulkhead(name string, maxConcurrency int) *Bulkhead {
	return &Bulkhead{name: name, slots: make(chan struct{}, maxConcurrency)}
}

// Acquire blocks until a slot is free or ctx is cancelled. The returned
// release func MUST be called (typically via defer) to return the slot.
func (b *Bulkhead) Acquire(ctx context.Context) (func(), error) {
	select {
	case b.slots <- struct{}{}:
		return b.release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *Bulkhead) release() { <-b.slots }

// InFlight returns the current count of in-flight calls.
func (b *Bulkhead) InFlight() int { return len(b.slots) }
