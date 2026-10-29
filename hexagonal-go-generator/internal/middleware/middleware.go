// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package middleware bundles the full production HTTP middleware stack.
// Order matters and is documented in server.go; each file below implements
// exactly one cross-cutting concern with its own layer-2 (Middleware) logging.
//
// Stack:
//   Recover      - never panic the process
//   RequestID    - stamps requestId on ctx + X-Request-Id header
//   HTTPLogger   - structured access log (Layer 1 HTTP)
//   CORS         - preflight handling
//   RateLimit    - token-bucket per IP
//   Timeout      - per-request deadline
//   Auth (JWT or APIKey) - 401 if missing
//   RBAC         - 403 if role insufficient
//   Audit        - log mutations for SOC2
//   Metrics      - already counted by HTTPLogger, no separate mw needed
//
// GAP CLOSED: #1 JWT auth + API key auth + RBAC
//             #2 Request ID / correlation ID
//             #4 Audit log for all mutations
//             #12 (HTTP-layer) per-method logging with timings
package middleware

import (
        "context"
        "net/http"
        "time"

        "github.com/go-chi/chi/v5"
        "github.com/google/uuid"
        "github.com/ori-inc/hexagonal-go/internal/logger"
        "github.com/ori-inc/hexagonal-go/pkg/metrics"
        "github.com/rs/zerolog/log"
)

// --- 1. Recover -------------------------------------------------------------

// Recover catches panics from any downstream handler, logs them at the Errors
// layer with the stack, and returns a 500 envelope. The process stays alive.
func Recover(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                defer func() {
                        if rec := recover(); rec != nil {
                                logger.Ctx(r.Context(), logger.LayerErrors).
                                        Error().
                                        Interface("panic", rec).
                                        Str("path", r.URL.Path).
                                        Msg("PANIC_RECOVERED")
                                http.Error(w, `{"error":{"code":"INTERNAL","message":"internal error"}}`,
                                        http.StatusInternalServerError)
                        }
                }()
                next.ServeHTTP(w, r)
        })
}

// --- 2. RequestID ------------------------------------------------------------

const HeaderRequestID = "X-Request-Id"

// RequestID reads an incoming X-Request-Id (or mints a new UUID) and stamps it
// onto the request context + the response header. Every downstream log line
// inherits the id via logger.Ctx.
func RequestID(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                id := r.Header.Get(HeaderRequestID)
                if id == "" {
                        id = uuid.NewString()
                }
                w.Header().Set(HeaderRequestID, id)
                ctx := logger.WithRequestID(r.Context(), id)
                next.ServeHTTP(w, r.WithContext(ctx))
        })
}

// --- 3. HTTPLogger (Layer 1) ------------------------------------------------

// statusWriter captures the status code so the logger can read it after the
// handler runs. Without this, http.ResponseWriter discards the code.
type statusWriter struct {
        http.ResponseWriter
        status int
        bytes  int
}

func (s *statusWriter) WriteHeader(code int) {
        s.status = code
        s.ResponseWriter.WriteHeader(code)
}
func (s *statusWriter) Write(b []byte) (int, error) {
        if s.status == 0 {
                s.status = 200
        }
        n, err := s.ResponseWriter.Write(b)
        s.bytes += n
        return n, err
}

// HTTPLogger emits one structured access log per request and records Prometheus
// counters/histograms. Route is derived from chi's RoutePattern() so the
// cardinality stays low (e.g. /api/v1/users/{id} not /api/v1/users/123).
func HTTPLogger(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                start := time.Now()
                sw := &statusWriter{ResponseWriter: w}

                defer func() {
                        status := sw.status
                        if status == 0 {
                                status = 200
                        }
                        route := chi.RouteContext(r.Context()).RoutePattern()
                        if route == "" {
                                route = "unknown"
                        }
                        metrics.ObserveHTTP(r.Method, route, statusField(status), start)

                        evt := logger.Ctx(r.Context(), logger.LayerHTTP).Info()
                        if status >= 500 {
                                evt = logger.Ctx(r.Context(), logger.LayerHTTP).Error()
                        } else if status >= 400 {
                                evt = logger.Ctx(r.Context(), logger.LayerHTTP).Warn()
                        }
                        evt.
                                Str("method", r.Method).
                                Str("path", r.URL.Path).
                                Str("route", route).
                                Int("status", status).
                                Int("bytes", sw.bytes).
                                Str("ip", clientIP(r)).
                                Str("userAgent", r.UserAgent()).
                                Dur("durationMs", time.Since(start)).
                                Msg("HTTP_REQUEST")
                }()

                next.ServeHTTP(sw, r)
        })
}

func statusField(s int) string {
        switch {
        case s < 200:
                return "1xx"
        case s < 300:
                return "2xx"
        case s < 400:
                return "3xx"
        case s < 500:
                return "4xx"
        default:
                return "5xx"
        }
}

func clientIP(r *http.Request) string {
        if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
                return xff
        }
        return r.RemoteAddr
}

// --- 4. CORS ----------------------------------------------------------------

// CORS just wraps go-chi/cors with sensible defaults. Real config lives in
// server.go so we can read .env.
func CORS(allowedOrigins, allowedMethods []string) func(http.Handler) http.Handler {
        return func(next http.Handler) http.Handler {
                // We delegate to chi's cors middleware by returning a passthrough;
                // the actual cors.Cors handler is mounted in server.go.
                log.Debug().Strs("origins", allowedOrigins).Msg("CORS_CONFIGURED")
                return next
        }
}

// --- 5. RateLimit ------------------------------------------------------------

// RateLimit returns a per-IP token-bucket limiter. Buckets are kept in a
// sync.Map; production deployments with many IPs should swap this for a Redis
// bucket (the interface stays the same).
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
        var buckets ipBuckets
        return func(next http.Handler) http.Handler {
                return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                        lim := buckets.get(clientIP(r), rps, burst)
                        if !lim.Allow() {
                                logger.Ctx(r.Context(), logger.LayerMiddleware).
                                        Warn().
                                        Str("ip", clientIP(r)).
                                        Msg("RATE_LIMITED")
                                w.Header().Set("Retry-After", "1")
                                http.Error(w, `{"error":{"code":"RATE_LIMITED","message":"too many requests"}}`,
                                        http.StatusTooManyRequests)
                                return
                        }
                        next.ServeHTTP(w, r)
                })
        }
}

// --- 6. Timeout --------------------------------------------------------------

// Timeout enforces a per-request wall-clock deadline. Any handler that
// respects r.Context() will abort automatically when the deadline expires.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
        return func(next http.Handler) http.Handler {
                return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                        ctx, cancel := context.WithTimeout(r.Context(), d)
                        defer cancel()
                        r = r.WithContext(ctx)
                        next.ServeHTTP(w, r)
                })
        }
}
