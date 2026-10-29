// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// webhooks_test.go - unit tests for the webhook dispatcher.
// Run with: go test -short ./internal/infrastructure/webhooks/...
package webhooks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/domain/events"
)

// stubEvent is a minimal events.Event for testing.
type stubEvent struct {
	id, topic string
	t         time.Time
}

func (s stubEvent) Topic() string            { return s.topic }
func (s stubEvent) EventID() string          { return s.id }
func (s stubEvent) OccurredAt() time.Time    { return s.t }

func TestDispatcher_DeliversToMatchingTopic(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify HMAC signature header is present (when secret is set).
		if r.Header.Get("X-Webhook-Signature") == "" {
			t.Error("expected X-Webhook-Signature header")
		}
		// Verify event metadata headers.
		if r.Header.Get("X-Webhook-Event") != "user.created" {
			t.Errorf("expected X-Webhook-Event=user.created, got %q", r.Header.Get("X-Webhook-Event"))
		}
		w.WriteHeader(200)
		received <- "ok"
	}))
	defer srv.Close()

	d := New()
	d.Subscribe(Subscription{URL: srv.URL, Topics: []string{"user.created"}, Secret: "topsecret"})

	_ = d.Dispatch(context.Background(), stubEvent{
		id: "evt-1", topic: "user.created", t: time.Now(),
	})

	select {
	case <-received:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("webhook was not delivered within 3s")
	}
}

func TestDispatcher_SkipsNonMatchingTopic(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	defer srv.Close()

	d := New()
	d.Subscribe(Subscription{URL: srv.URL, Topics: []string{"order.created"}})

	_ = d.Dispatch(context.Background(), stubEvent{
		id: "evt-2", topic: "user.created", t: time.Now(),
	})

	// Give the dispatcher a moment to (not) fire.
	time.Sleep(200 * time.Millisecond)
	if called {
		t.Fatal("webhook should NOT have been called for a non-matching topic")
	}
}

func TestDispatcher_TopicMatches(t *testing.T) {
	cases := []struct {
		subs    []string
		topic   string
		want    bool
	}{
		{nil, "any.topic", true},        // empty = all
		{[]string{"user.created"}, "user.created", true},
		{[]string{"user.created"}, "order.created", false},
		{[]string{"user.created", "order.created"}, "order.created", true},
	}
	for _, c := range cases {
		if got := topicMatches(c.subs, c.topic); got != c.want {
			t.Errorf("topicMatches(%v, %q) = %v, want %v", c.subs, c.topic, got, c.want)
		}
	}
}

// ensure the package compiles against the events.Event interface.
var _ events.Event = stubEvent{}
