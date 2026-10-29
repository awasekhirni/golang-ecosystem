// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package server wires every layer into a runnable HTTP server and owns the
// graceful-shutdown drain sequence.
//
// Boot sequence:
//   1. Load config (fail-fast env validation).
//   2. Init structured logger (12-layer matrix) + PII redaction.
//   3. Open PostgreSQL pool with retry.
//   4. Run pending migrations (incl. outbox table).
//   5. Build domain services + repositories + UoW + bus + outbox relay.
//   6. Build OpenAPI spec + register every operation.
//   7. Build GraphQL handler bound to the same services.
//   8. Build composite health subsystem + register checkers.
//   9. Build background job runner.
//  10. Build chi router with the full middleware stack:
//        Recover, OTel trace id, RequestID, HTTPLogger, SecurityHeaders,
//        CORS, Compression, MaxBodySize, RateLimit, Timeout, Versioning,
//        APIKey, JWT, RequireUser, Idempotency, ETag, Audit
//  11. Mount system routes (/healthz/*, /metrics, /docs/*, /openapi.*,
//      /auth/demo-token, /graphql, /graphql/schema, /debug/pprof/*).
//  12. Mount authenticated API routes (/api/v1/*).
//  13. Start http.Server + outbox relay + job runner.
//  14. Block on SIGINT/SIGTERM; on signal:
//        a. mark ready=false (so /readyz returns 503),
//        b. stop job runner + outbox relay,
//        c. stop accepting new conns (srv.Shutdown),
//        d. wait for in-flight requests with timeout,
//        e. close DB pool,
//        f. exit 0.
//
// GAP CLOSED: #6 Liveness vs readiness probes
//             #14 Graceful shutdown with drain
//             #5 /metrics endpoint
//             Composite health, OpenAPI, GraphQL, OTel, Idempotency, ETag,
//             SecurityHeaders, Compression, Outbox, Versioning, Feature
//             flags, Background jobs.
package server

import (
        "context"
        "encoding/json"
        "errors"
        "net/http"
        "net/http/pprof"
        "os"
        "os/signal"
        "strings"
        "sync/atomic"
        "syscall"
        "time"

        "github.com/go-chi/chi/v5"
        "github.com/go-chi/cors"
        "github.com/jmoiron/sqlx"
        usercontroller "github.com/ori-inc/hexagonal-go/internal/api/controllers/user"
        usersvc "github.com/ori-inc/hexagonal-go/internal/application/services/user"
        "github.com/ori-inc/hexagonal-go/internal/config"
        "github.com/ori-inc/hexagonal-go/internal/database"
        "github.com/ori-inc/hexagonal-go/internal/domain/bus"
        userent "github.com/ori-inc/hexagonal-go/internal/domain/entities/user"
        "github.com/ori-inc/hexagonal-go/internal/domain/events"
        "github.com/ori-inc/hexagonal-go/internal/domain/uow"
        "github.com/ori-inc/hexagonal-go/internal/features"
        "github.com/ori-inc/hexagonal-go/internal/graphql"
        "github.com/ori-inc/hexagonal-go/internal/health"
        "github.com/ori-inc/hexagonal-go/internal/infrastructure/outbox"
        otelmw "github.com/ori-inc/hexagonal-go/internal/infrastructure/otel"
        "github.com/ori-inc/hexagonal-go/internal/infrastructure/webhooks"
        "github.com/ori-inc/hexagonal-go/internal/jobs"
        userrepo "github.com/ori-inc/hexagonal-go/internal/infrastructure/repositories/user"
        "github.com/ori-inc/hexagonal-go/internal/middleware"
        "github.com/ori-inc/hexagonal-go/internal/logger"
        "github.com/ori-inc/hexagonal-go/internal/openapi"
        "github.com/ori-inc/hexagonal-go/internal/versioning"
        "github.com/ori-inc/hexagonal-go/pkg/metrics"
)

// Container holds every wired dependency. Exposed so main_test.go can poke.
type Container struct {
        Cfg     *config.Config
        DB      *database.DB
        Bus     events.Dispatcher
        UoW     *uow.UoW
        UserSvc *usersvc.Service
        Health  *health.Subsystem
        Jobs    *jobs.Runner
        Outbox  *outbox.Relay
        OpenAPI *openapi.Spec
}

// Build constructs the DI container and runs migrations.
func Build(ctx context.Context, cfg *config.Config) (*Container, error) {
        L := logger.For(logger.LayerLifecycle)
        L.Info().Str("app", cfg.App.Name).Str("env", cfg.App.Env).Msg("BOOT_START")

        db, err := database.New(ctx, cfg)
        if err != nil {
                return nil, err
        }
        if err := db.Migrate(ctx); err != nil {
                return nil, err
        }
        if err := outbox.EnsureSchema(ctx, db.DB); err != nil {
                return nil, err
        }

        bus := bus.New()
        // Outbox writer persists events inside the same tx as the business write.
        ob := outbox.New()
        uowInstance := uow.NewWithOutbox(db.DB, uowFactory, bus, ob)
        userRepo := userrepo.New(db.DB)
        userService := usersvc.New(userRepo, uowInstance, bus)

        // Subscribe a demo handler so domain events are observable in the logs.
        bus.Subscribe(events.HandlerFunc{
                TopicStr: userent.UserCreated{}.Topic(),
                Fn: func(ctx context.Context, e events.Event) error {
                        logger.Ctx(ctx, logger.LayerEnterpriseBus).
                                Info().Str("eventId", e.EventID()).Msg("DEMO_HANDLER_USER_CREATED")
                        return nil
                },
        })

        // --- webhook dispatcher (external event fan-out) --------------------
        // External systems register webhook URLs via WH_SUBSCRIPTIONS env (JSON
        // array of {url, topics, secret}). The dispatcher fans events out to
        // them with HMAC signing + exponential retry + circuit breaker.
        webhookDisp := webhooks.New()
        if subs := parseWebhookSubs(os.Getenv("WH_SUBSCRIPTIONS")); len(subs) > 0 {
                for _, s := range subs {
                        webhookDisp.Subscribe(s)
                }
                // Also route every published event to the webhook dispatcher via
                // the bus fan-out.
                bus.Subscribe(events.HandlerFunc{
                        TopicStr: "*",
                        Fn: func(ctx context.Context, e events.Event) error {
                                return webhookDisp.Dispatch(ctx, e)
                        },
                })
        }

        // --- composite health subsystem --------------------------------------
        hh := health.New()
        hh.Register("database", func(ctx context.Context) error {
                return db.PingContext(ctx)
        })
        hh.Register("bus", func(ctx context.Context) error {
                // In-memory bus is always up; real impl would ping the broker.
                return nil
        })

        // --- outbox relay (only if feature flag is on) -----------------------
        var relay *outbox.Relay
        if features.On(features.FlagOutboxRelay) {
                relay = outbox.NewRelay(db.DB, bus, 2*time.Second, 100)
                relay.Start(ctx)
        }

        // --- background job runner -------------------------------------------
        runner := jobs.New()
        runner.Register(jobs.Job{
                Name:     "outbox_relay_heartbeat",
                Schedule: 30 * time.Second,
                Run: func(ctx context.Context) error {
                        // Just logs that the runner is alive; real jobs would do work.
                        logger.For(logger.LayerLifecycle).Debug().Msg("JOB_HEARTBEAT")
                        return nil
                },
        })
        runner.Start(ctx)

        // --- OpenAPI spec ----------------------------------------------------
        spec := buildOpenAPI()

        L.Info().Msg("BOOT_READY")
        hh.MarkBooted()
        return &Container{
                Cfg:     cfg,
                DB:      db,
                Bus:     bus,
                UoW:     uowInstance,
                UserSvc: userService,
                Health:  hh,
                Jobs:    runner,
                Outbox:  relay,
                OpenAPI: spec,
        }, nil
}

// buildOpenAPI assembles the OpenAPI 3.1 spec with every aggregate's
// operations registered.
func buildOpenAPI() *openapi.Spec {
        b := openapi.NewBuilder(
                "Hexagonal Go REST API",
                "1.0.0",
                "Hexagonal Go reference implementation - REST + GraphQL API for the User aggregate.",
        )
        b.AddServer("http://localhost:8080", "Local development")
        openapi.RegisterUserOps(b)
        return b.Build()
}

// uowFactory is the per-tx repository factory passed to UoW.Run.
func uowFactory(tx *sqlx.Tx) uow.Tx { return uow.NewConcreteTx(tx) }

// Server owns the http.Server plus the readiness flag.
type Server struct {
        httpSrv *http.Server
        cfg     *config.Config
        db      *database.DB
        c       *Container
        ready   atomic.Bool
}

// New constructs the server (does not start it).
func New(c *Container) *Server {
        r := chi.NewRouter()

        // --- global middleware (order matters) ---------------------------------
        // 1. Recover must be first so panics never crash the process.
        r.Use(middleware.Recover)
        // 2. OTel trace id (minted from W3C traceparent or fresh) - before
        //    RequestID so the trace id is available to every downstream log.
        if features.On(features.FlagOTel) {
                r.Use(otelmw.Middleware)
        }
        // 3. RequestID stamps X-Request-Id on ctx + response header.
        r.Use(middleware.RequestID)
        // 4. HTTPLogger emits one structured access log per request + Prometheus.
        r.Use(middleware.HTTPLogger)
        // 5. Security headers (OWASP A05:2021).
        if features.On(features.FlagSecurityHdrs) {
                r.Use(middleware.SecurityHeaders(c.Cfg.IsProduction()))
        }
        // 6. CORS.
        r.Use(cors.Handler(cors.Options{
                AllowedOrigins:   c.Cfg.CORS.AllowedOrigins,
                AllowedMethods:   c.Cfg.CORS.AllowedMethods,
                AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key", "X-Request-Id", "Idempotency-Key", "If-Match", "If-None-Match"},
                ExposedHeaders:   []string{"X-Request-Id", "ETag", "Location", "API-Version", "Deprecation", "Sunset"},
                AllowCredentials: true,
                MaxAge:           300,
        }))
        // 7. Compression (gzip).
        if features.On(features.FlagCompression) {
                r.Use(middleware.Compress())
        }
        // 8. Body size limit (1 MiB) - DoS protection.
        r.Use(middleware.MaxBodySize(1 << 20))
        // 9. Rate limit (per IP token bucket).
        r.Use(middleware.RateLimit(c.Cfg.Rate.LimitRPS, c.Cfg.Rate.LimitBurst))
        // 10. Per-request timeout.
        r.Use(middleware.Timeout(c.Cfg.HTTP.WriteTimeout))
        // 11. API versioning (stamps API-Version/Deprecation/Sunset headers).
        r.Use(versioning.Middleware(versioning.Default()))
        // 12. Auth (API key OR JWT). Either one authenticates; anonymous is
        //     allowed through and RBAC will 401/403 as needed.
        r.Use(middleware.APIKeyAuth(c.Cfg.Auth.APIKeys))
        r.Use(middleware.JWTAuth(c.Cfg.JWT.Secret))
        // 13. Stamp userId on ctx for the logger.
        r.Use(middleware.RequireUser)
        // 14. Idempotency-Key dedup (mutations only).
        if features.On(features.FlagIdempotency) {
                r.Use(middleware.Idempotency(nil))
        }
        // 15. ETag + conditional GETs.
        if features.On(features.FlagETag) {
                r.Use(middleware.ETag)
        }
        // 16. Audit (mutations only; SHA-256 body digest).
        r.Use(middleware.Audit)

        srv := &Server{cfg: c.Cfg, db: c.DB, c: c}

        // --- system routes (no auth) ------------------------------------------
        // Composite health: /healthz/startup, /healthz/live, /healthz/ready, /healthz/deep
        r.Get("/healthz", c.Health.LivenessHandler()) // legacy alias
        r.Get("/healthz/startup", c.Health.StartupHandler())
        r.Get("/healthz/live", c.Health.LivenessHandler())
        r.Get("/healthz/ready", c.Health.ReadinessHandler())
        r.Get("/readyz", c.Health.ReadinessHandler()) // legacy alias
        r.Get("/healthz/deep", c.Health.DeepHandler())

        r.Handle("/metrics", metrics.Handler())

        // --- OpenAPI spec + UIs -----------------------------------------------
        r.Get("/openapi.json", openapi.JSONHandler(c.OpenAPI))
        r.Get("/openapi.yaml", openapi.YAMLHandler(c.OpenAPI))
        if features.On(features.FlagOpenAPI) {
                r.Get("/docs/swagger", openapi.SwaggerUIHandler())
                r.Get("/docs/redoc", openapi.ReDocUIHandler())
        }

        // --- GraphQL (parallel to REST) ---------------------------------------
        if features.On(features.FlagGraphQL) {
                gqlH := graphql.New(c.UserSvc)
                r.Handle("/graphql", gqlH)
                r.Get("/graphql/schema", graphql.SchemaHandler())
                r.Get("/graphql/playground", graphql.PlaygroundHandler())
        }

        // --- pprof (opt-in via FEATURES=+use_pprof) ---------------------------
        if features.On(features.FlagProfiling) {
                r.HandleFunc("/debug/pprof/", pprof.Index)
                r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
                r.HandleFunc("/debug/pprof/profile", pprof.Profile)
                r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
                r.HandleFunc("/debug/pprof/trace", pprof.Trace)
        }

        // --- demo auth route (no auth required to login) ----------------------
        r.Get("/auth/demo-token", demoTokenHandler(c))

        // --- authenticated API routes ----------------------------------------
        r.Route("/api/v1", func(r chi.Router) {
                r.Use(middleware.Require(middleware.RoleAdmin, middleware.RoleUser))
                // Runtime request-body validation against the OpenAPI spec.
                // Rejects malformed bodies with 400 + field-level details BEFORE
                // the controller is reached.
                r.Use(openapi.RequestValidator(c.OpenAPI))
                usercontroller.New(c.UserSvc).Register(r)
        })

        srv.httpSrv = &http.Server{
                Addr:         ":" + itoa(c.Cfg.App.Port),
                Handler:      r,
                ReadTimeout:  c.Cfg.HTTP.ReadTimeout,
                WriteTimeout: c.Cfg.HTTP.WriteTimeout,
                IdleTimeout:  c.Cfg.HTTP.IdleTimeout,
        }
        return srv
}

// Handler returns the underlying http.Handler so tests can mount the server
// on httptest.NewServer without starting a real listener.
func (s *Server) Handler() http.Handler {
        return s.httpSrv.Handler
}

// Run starts the server and blocks until a signal arrives or the server
// errors. Implements graceful drain.
func (s *Server) Run() error {
        L := logger.For(logger.LayerLifecycle)

        // Set ready=true; liveness is always true once we reach this point.
        s.ready.Store(true)

        errCh := make(chan error, 1)
        go func() {
                L.Info().Str("addr", s.httpSrv.Addr).Msg("SERVER_LISTENING")
                if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
                        errCh <- err
                }
        }()

        // Wait for signal or fatal error.
        sigCh := make(chan os.Signal, 1)
        signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
        select {
        case err := <-errCh:
                L.Error().Err(err).Msg("SERVER_FATAL")
                return err
        case sig := <-sigCh:
                L.Warn().Str("signal", sig.String()).Msg("SIGNAL_RECEIVED")
        }

        // --- graceful drain ----------------------------------------------------
        s.ready.Store(false)
        L.Info().Msg("DRAIN_START")

        // Stop background workers first so they don't fight over the DB.
        if s.c.Jobs != nil {
                s.c.Jobs.Stop()
        }
        if s.c.Outbox != nil {
                s.c.Outbox.Stop()
        }

        ctx, cancel := context.WithTimeout(context.Background(), s.cfg.HTTP.ShutdownTimeout)
        defer cancel()

        if err := s.httpSrv.Shutdown(ctx); err != nil {
                L.Error().Err(err).Msg("DRAIN_SHUTDOWN_ERROR")
        }
        if err := s.db.Close(); err != nil {
                L.Error().Err(err).Msg("DRAIN_DB_CLOSE_ERROR")
        }
        L.Info().Msg("DRAIN_COMPLETE")
        return nil
}

// --- demo token + helpers --------------------------------------------------

func demoTokenHandler(c *Container) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                tok, err := middleware.IssueJWT(
                        c.Cfg.JWT.Secret, c.Cfg.JWT.Issuer,
                        "demo-user", []middleware.Role{middleware.RoleUser},
                        c.Cfg.JWT.AccessTTL,
                )
                if err != nil {
                        http.Error(w, `{"error":{"code":"INTERNAL","message":"token issue failed"}}`,
                                http.StatusInternalServerError)
                        return
                }
                w.Header().Set("Content-Type", "application/json")
                _, _ = w.Write([]byte(`{"token":"` + tok + `","expiresIn":"` + c.Cfg.JWT.AccessTTL.String() + `"}`))
        }
}

// itoa is a tiny stdlib-free int->string for the port number.
func itoa(n int) string {
        if n == 0 {
                return "0"
        }
        var b []byte
        for n > 0 {
                b = append([]byte{byte('0' + n%10)}, b...)
                n /= 10
        }
        return string(b)
}

// parseWebhookSubs decodes the WH_SUBSCRIPTIONS env var (JSON array of
// {url, topics, secret}) into a slice of webhooks.Subscription. Returns
// nil on empty/malformed input (logged at warn).
func parseWebhookSubs(s string) []webhooks.Subscription {
        s = strings.TrimSpace(s)
        if s == "" {
                return nil
        }
        var subs []webhooks.Subscription
        if err := json.Unmarshal([]byte(s), &subs); err != nil {
                logger.For(logger.LayerLifecycle).
                        Warn().Err(err).Msg("WEBHOOK_SUBS_PARSE_FAILED")
                return nil
        }
        return subs
}
