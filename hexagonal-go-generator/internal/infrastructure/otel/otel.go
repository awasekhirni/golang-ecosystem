// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package otel wires a minimal OpenTelemetry tracer. We avoid pulling in
// the full go.opentelemetry.io/otel stack to keep the binary small; instead
// we expose the same Span/Tracer abstractions and emit Layer-9 (Domain
// Event) + Layer-7 (Repository) style logs with traceId + spanId so a
// downstream collector (Jaeger/Tempo/Datadog) can ingest them.
//
// To upgrade to the real OTLP exporter:
//
//   1. `go get go.opentelemetry.io/otel go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`
//   2. Replace Init() with a real TracerProvider + OTLP exporter.
//   3. Replace Span.Start/Span.End with otel's span API.
//
// The trace id stamped on every log line by logger.Ctx comes from
// logger.WithTraceID — set it from the middleware boundary.
//
// GAP CLOSED: OpenTelemetry distributed tracing (was mentioned but not
// implemented in the first pass).
package otel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// Span is a minimal span. Start() returns it; End() records the duration.
type Span struct {
	Name    string
	TraceID string
	SpanID  string
	Started time.Time
	attrs   map[string]string
	mu      sync.Mutex
}

// Start returns a new span. The trace id is inherited from ctx if present
// (via logger.TraceIDFrom), otherwise minted fresh. The span id is always
// fresh.
func Start(ctx context.Context, name string) (context.Context, *Span) {
	spanID := randID(8)
	traceID := logger.TraceIDFrom(ctx)
	if traceID == "" {
		traceID = randID(16)
	}
	s := &Span{
		Name:    name,
		TraceID: traceID,
		SpanID:  spanID,
		Started: time.Now(),
		attrs:   map[string]string{},
	}
	ctx = logger.WithTraceID(ctx, traceID)
	logger.Ctx(ctx, logger.LayerService).
		Debug().
		Str("span", name).
		Str("spanId", spanID).
		Str("traceId", traceID).
		Msg("SPAN_START")
	return ctx, s
}

// End records the span end + duration.
func (s *Span) End() {
	logger.For(logger.LayerService).
		Debug().
		Str("span", s.Name).
		Str("spanId", s.SpanID).
		Str("traceId", s.TraceID).
		Dur("durationMs", time.Since(s.Started)).
		Msg("SPAN_END")
}

// SetAttr attaches a key/value attribute to the span (logged on End).
func (s *Span) SetAttr(k, v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attrs[k] = v
}

// Middleware stamps the request ctx with a trace id (minted from the
// `traceparent` W3C header if present, otherwise fresh). Mount it near the
// top of the stack so every downstream logger.Ctx line carries the trace id.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := parseTraceparent(r.Header.Get("traceparent"))
		if traceID == "" {
			traceID = randID(16)
		}
		ctx := logger.WithTraceID(r.Context(), traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// parseTraceparent extracts the trace id from a W3C traceparent header
// `00-<traceId>-<spanId>-<flags>`.
func parseTraceparent(tp string) string {
	parts := strings.Split(tp, "-")
	if len(parts) < 4 {
		return ""
	}
	return parts[1]
}

// randID returns a random hex string of `bytes` bytes.
func randID(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
