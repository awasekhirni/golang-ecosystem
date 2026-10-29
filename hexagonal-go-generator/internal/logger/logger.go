// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package logger implements the 12-layer structured logging matrix described in
// the architecture document. Every layer (HTTP, Middleware, Controller,
// Validation, Mapper, Service, Repository, Database, Domain Events, Enterprise
// Bus, Errors, Lifecycle) gets its own typed logger that automatically stamps
// the right context fields (requestId, userId, action, durationMs, etc.).
//
// Output is structured JSON via zerolog so it can be ingested by ELK / Loki /
// Datadog / CloudWatch without parsing. A console prettifier is available for
// local development via LOG_FORMAT=console.
//
// GAP CLOSED: #3 Structured JSON logging with context
//             #2 Request ID / correlation ID (via WithRequestID)
//             #19 Per-method logging with timings (durationMs fields)
package logger

import (
        "context"
        "io"
        "os"
        "strings"
        "time"

        "github.com/ori-inc/hexagonal-go/internal/redaction"
        "github.com/rs/zerolog"
        "github.com/rs/zerolog/log"
)

// Layer is the 12-layer enum. Stamping the layer on every record lets a
// downstream query like `level=error layer=repository` instantly pin the
// failing slice of the architecture.
type Layer string

const (
        LayerHTTP         Layer = "http"
        LayerMiddleware   Layer = "middleware"
        LayerController   Layer = "controller"
        LayerValidation   Layer = "validation"
        LayerMapper       Layer = "mapper"
        LayerService      Layer = "service"
        LayerRepository   Layer = "repository"
        LayerDatabase     Layer = "database"
        LayerDomainEvent  Layer = "domain_event"
        LayerEnterpriseBus Layer = "enterprise_bus"
        LayerErrors       Layer = "errors"
        LayerLifecycle    Layer = "lifecycle"
)

// ctxKey is unexported so callers must use the typed helpers below.
type ctxKey string

const (
        ctxKeyRequestID ctxKey = "request_id"
        ctxKeyUserID    ctxKey = "user_id"
        ctxKeyTraceID   ctxKey = "trace_id"
)

// Init wires zerolog's global logger using app config.
// level: trace|debug|info|warn|error ; format: json|console
//
// Every log line is piped through the redaction writer so PII (email,
// phone, SSN, credit-card, JWT) is auto-masked before hitting disk.
func Init(level, format string) {
        zerolog.TimeFieldFormat = time.RFC3339Nano

        lvl, err := zerolog.ParseLevel(strings.ToLower(level))
        if err != nil || lvl == zerolog.NoLevel {
                lvl = zerolog.InfoLevel
        }
        zerolog.SetGlobalLevel(lvl)

        // The redaction writer wraps the final destination so every line is
        // scrubbed in-flight. This is the GDPR Art. 32 control.
        var dst io.Writer = os.Stdout
        dst = redaction.NewWriter(dst)

        if strings.EqualFold(format, "console") {
                log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: dst, TimeFormat: time.RFC3339}).
                        With().Timestamp().Caller().Logger()
        } else {
                log.Logger = zerolog.New(dst).
                        With().Timestamp().Caller().Logger()
        }
}

// For returns a layer-stamped sub-logger. Use it once at the top of each
// package, e.g. `var L = logger.For(logger.LayerRepository)`.
func For(layer Layer) *zerolog.Logger {
        l := log.Logger.With().Str("layer", string(layer)).Logger()
        return &l
}

// --- Context plumbing -------------------------------------------------------

// WithRequestID returns a new context carrying the request id.
// Used by the request-id middleware so every downstream log line inherits it.
func WithRequestID(ctx context.Context, id string) context.Context {
        return context.WithValue(ctx, ctxKeyRequestID, id)
}

// WithUserID returns a new context carrying the authenticated user id.
func WithUserID(ctx context.Context, id string) context.Context {
        return context.WithValue(ctx, ctxKeyUserID, id)
}

// WithTraceID returns a new context carrying an OpenTelemetry trace id.
func WithTraceID(ctx context.Context, id string) context.Context {
        return context.WithValue(ctx, ctxKeyTraceID, id)
}

// RequestIDFrom extracts the request id from a context (empty if absent).
func RequestIDFrom(ctx context.Context) string {
        if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
                return v
        }
        return ""
}

// UserIDFrom extracts the user id from a context (empty if absent).
func UserIDFrom(ctx context.Context) string {
        if v, ok := ctx.Value(ctxKeyUserID).(string); ok {
                return v
        }
        return ""
}

// TraceIDFrom extracts the trace id from a context (empty if absent).
func TraceIDFrom(ctx context.Context) string {
        if v, ok := ctx.Value(ctxKeyTraceID).(string); ok {
                return v
        }
        return ""
}

// Ctx returns a zerolog context pre-loaded with the request/user/trace ids
// found in ctx. Every layer logger should call this once at the start of a
// function so all subsequent lines inherit the correlation ids:
//
//      log := logger.Ctx(ctx, logger.LayerService)
//      log.Info().Str("operation","CreateUser").Msg("SERVICE_IN")
func Ctx(ctx context.Context, layer Layer) *zerolog.Logger {
        l := log.Logger.With().Str("layer", string(layer))
        if rid := RequestIDFrom(ctx); rid != "" {
                l = l.Str("requestId", rid)
        }
        if uid := UserIDFrom(ctx); uid != "" {
                l = l.Str("userId", uid)
        }
        if tid := TraceIDFrom(ctx); tid != "" {
                l = l.Str("traceId", tid)
        }
        out := l.Logger()
        return &out
}

// Timer is a tiny helper for per-method timing logs:
//
//      defer logger.Timer(ctx, logger.LayerRepository, "get_by_id", time.Now())
//
// It emits a line with durationMs when the function returns.
func Timer(ctx context.Context, layer Layer, operation string, start time.Time) {
        elapsed := time.Since(start)
        Ctx(ctx, layer).
                Info().
                Str("operation", operation).
                Dur("durationMs", elapsed).
                Msg("timing")
}
