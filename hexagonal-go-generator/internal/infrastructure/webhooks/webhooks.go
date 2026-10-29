// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package webhooks implements an HTTP fan-out dispatcher for domain events.
// External systems register a webhook URL for one or more topics; when an
// event is published, the dispatcher POSTs the event payload to every
// matching URL with exponential-backoff retry.
//
// Delivery is at-least-once; receivers MUST be idempotent (eventId dedup).
// The dispatcher signs every payload with an HMAC-SHA256 of the body using
// a shared secret so receivers can verify authenticity.
//
// This is the external counterpart to the in-process Enterprise Bus: the
// bus is for internal handlers, webhooks are for external systems.
//
// GAP CLOSED: Webhook dispatcher for domain events (external event fan-out)
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/domain/events"
	"github.com/ori-inc/hexagonal-go/internal/logger"
	"github.com/ori-inc/hexagonal-go/internal/resilience"
)

// Subscription pairs a URL with the topics it wants.
type Subscription struct {
	URL    string
	Topics []string // empty = all topics
	Secret string   // shared secret for HMAC signing; empty = unsigned
}

// Dispatcher fans events out to registered webhooks.
type Dispatcher struct {
	mu      sync.RWMutex
	subs    []Subscription
	client  *http.Client
	breaker *resilience.CircuitBreaker
}

// New constructs a dispatcher with a 10s HTTP client timeout + a circuit
// breaker so a single flaky endpoint can't stall the publisher.
func New() *Dispatcher {
	return &Dispatcher{
		client:  &http.Client{Timeout: 10 * time.Second},
		breaker: resilience.NewCircuitBreaker("webhooks", 5, 30*time.Second),
	}
}

// Subscribe registers a webhook.
func (d *Dispatcher) Subscribe(s Subscription) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.subs = append(d.subs, s)
	logger.For(logger.LayerEnterpriseBus).
		Info().
		Str("url", s.URL).
		Strs("topics", s.Topics).
		Msg("WEBHOOK_SUBSCRIBED")
}

// Dispatch implements events.Dispatcher — but it fans out to HTTP webhooks
// instead of in-process handlers. We satisfy the same interface so the UoW
// can dispatch to either (or both) transparently.
func (d *Dispatcher) Dispatch(ctx context.Context, e events.Event) error {
	d.mu.RLock()
	subs := make([]Subscription, len(d.subs))
	copy(subs, d.subs)
	d.mu.RUnlock()

	matched := 0
	for _, s := range subs {
		if !topicMatches(s.Topics, e.Topic()) {
			continue
		}
		matched++
		// Each webhook fires in its own goroutine so one slow endpoint
		// doesn't block the others.
		go d.deliverWithRetry(ctx, s, e)
	}
	logger.Ctx(ctx, logger.LayerEnterpriseBus).
		Info().
		Str("eventId", e.EventID()).
		Str("topic", e.Topic()).
		Int("webhooks", matched).
		Msg("WEBHOOK_FANOUT")
	return nil
}

// topicMatches returns true if the subscription wants the given topic.
// Empty topics list means "all topics".
func topicMatches(topics []string, t string) bool {
	if len(topics) == 0 {
		return true
	}
	for _, tt := range topics {
		if tt == t {
			return true
		}
	}
	return false
}

// deliverWithRetry POSTs the event with exponential backoff (3 attempts).
func (d *Dispatcher) deliverWithRetry(ctx context.Context, s Subscription, e events.Event) {
	policy := resilience.DefaultRetryPolicy()
	policy.MaxAttempts = 3
	policy.IsRetryable = func(err error) bool { return err != nil }

	payload, err := json.Marshal(map[string]any{
		"event_id":    e.EventID(),
		"topic":       e.Topic(),
		"occurred_at": e.OccurredAt(),
		"payload":     e,
	})
	if err != nil {
		logger.For(logger.LayerEnterpriseBus).
			Error().Err(err).Str("eventId", e.EventID()).Msg("WEBHOOK_MARSHAL_FAILED")
		return
	}

	_ = d.breaker.Execute(ctx, func(ctx context.Context) error {
		return resilience.Retry(ctx, "webhook."+s.URL, policy, func(ctx context.Context) error {
			return d.deliverOnce(ctx, s, e, payload)
		})
	})
}

// deliverOnce makes a single HTTP POST with HMAC signature.
func (d *Dispatcher) deliverOnce(ctx context.Context, s Subscription, e events.Event, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", e.Topic())
	req.Header.Set("X-Webhook-Id", e.EventID())

	// HMAC-SHA256 signature so the receiver can verify authenticity.
	if s.Secret != "" {
		mac := hmac.New(sha256.New, []byte(s.Secret))
		mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", sig)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		logger.For(logger.LayerEnterpriseBus).
			Warn().Err(err).
			Str("url", s.URL).
			Str("eventId", e.EventID()).
			Msg("WEBHOOK_DELIVERY_RETRY")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logger.For(logger.LayerEnterpriseBus).
			Info().
			Str("url", s.URL).
			Str("eventId", e.EventID()).
			Int("status", resp.StatusCode).
			Msg("WEBHOOK_DELIVERED")
		return nil
	}
	logger.For(logger.LayerEnterpriseBus).
		Warn().
		Str("url", s.URL).
		Str("eventId", e.EventID()).
		Int("status", resp.StatusCode).
		Msg("WEBHOOK_NON_2XX")
	return errWebhookFailed
}

var errWebhookFailed = &webhookErr{"webhook returned non-2xx"}

type webhookErr struct{ msg string }

func (e *webhookErr) Error() string { return e.msg }
