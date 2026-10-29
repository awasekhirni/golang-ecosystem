// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package database owns the PostgreSQL connection pool and the migration
// runner. It wraps pgx + sqlx so repositories get a familiar *sqlx.DB while we
// keep pgx's superior observability hooks (slow-query log, prepared-statement
// cache, native UUID/JSON support).
//
// Layer-8 logging (Database): pool lifecycle + slow-query auto-WARN (>100ms).
//
// GAP CLOSED: #14 Graceful shutdown with drain (Close blocks until pooled
// connections drain, then disposes).
package database

import (
        "context"
        "database/sql"
        "fmt"
        "time"

        "github.com/jmoiron/sqlx"
        _ "github.com/jackc/pgx/v5/stdlib" // pgx driver registered as "pgx"
        "github.com/ori-inc/hexagonal-go/internal/config"
        "github.com/ori-inc/hexagonal-go/internal/logger"
)

// DB is the thin facade we hand to repositories.
type DB struct {
        *sqlx.DB
        cfg       *config.Config
        slowQuery time.Duration
}

// New opens the pool, pings it (with retry), and returns a ready *DB.
// Boot failure is fatal and aborts the process.
func New(ctx context.Context, cfg *config.Config) (*DB, error) {
        L := logger.For(logger.LayerDatabase)

        db, err := sqlx.Connect("pgx", cfg.Database.URL)
        if err != nil {
                L.Fatal().Err(err).Str("url", redact(cfg.Database.URL)).Msg("DB_OPEN_FAILED")
                return nil, err
        }

        // Pool sizing.
        maxOpen := cfg.Database.PoolSize + cfg.Database.MaxOverflow
        db.SetMaxOpenConns(maxOpen)
        db.SetMaxIdleConns(cfg.Database.PoolSize)
        db.SetConnMaxLifetime(cfg.Database.PoolRecycle)
        db.SetConnMaxIdleTime(cfg.Database.PoolRecycle / 2)

        // pgx native slow-query hook via stdlib is limited; we approximate it with
        // a Ping + stats sampling done by the repository layer (see Repo.Timer).
        db.SetConnMaxIdleTime(time.Minute)

        slow := time.Duration(cfg.Database.SlowQueryMs) * time.Millisecond
        L.Info().
                Int("maxOpen", maxOpen).
                Int("maxIdle", cfg.Database.PoolSize).
                Dur("connMaxLifetime", cfg.Database.PoolRecycle).
                Dur("slowQueryThreshold", slow).
                Msg("DB_POOL_READY")

        // Verify with retry (3 attempts, exponential backoff).
        if err := pingWithRetry(ctx, db, 3); err != nil {
                L.Fatal().Err(err).Msg("DB_PING_FAILED_AFTER_RETRIES")
                return nil, err
        }

        L.Info().Msg("DB_CONNECTED")
        return &DB{DB: db, cfg: cfg, slowQuery: slow}, nil
}

// Close disposes the pool. Called from the graceful-shutdown drain step.
func (d *DB) Close() error {
        logger.For(logger.LayerDatabase).Info().Msg("DB_POOL_DRAIN_START")
        err := d.DB.Close()
        if err != nil {
                logger.For(logger.LayerDatabase).Error().Err(err).Msg("DB_POOL_DRAIN_ERROR")
        }
        logger.For(logger.LayerDatabase).Info().Msg("DB_POOL_DRAINED")
        return err
}

// Stats snapshot for /healthz and /metrics.
func (d *DB) Stats() sql.DBStats { return d.DB.Stats() }

// SlowQueryThreshold exposes the threshold to repositories so they can decide
// whether to WARN a given SQL call.
func (d *DB) SlowQueryThreshold() time.Duration { return d.slowQuery }

// pingWithRetry retries the ping with simple exponential backoff.
// This avoids boot failures on cold databases (common in containerized CI).
func pingWithRetry(ctx context.Context, db *sqlx.DB, attempts int) error {
        var lastErr error
        for i := 1; i <= attempts; i++ {
                ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
                err := db.PingContext(ctx)
                cancel()
                if err == nil {
                        return nil
                }
                lastErr = err
                backoff := time.Duration(i*i) * time.Second
                logger.For(logger.LayerDatabase).
                        Warn().
                        Err(err).
                        Int("attempt", i).
                        Dur("backoff", backoff).
                        Msg("DB_PING_RETRY")
                time.Sleep(backoff)
        }
        return fmt.Errorf("ping failed after %d attempts: %w", attempts, lastErr)
}

// redact strips the password from a DSN before logging.
func redact(dsn string) string {
        // Naive: keep scheme+host, drop user:pass.
        at := indexOf(dsn, "@")
        if at < 0 {
                return dsn
        }
        scheme := "postgres://"
        if i := indexOf(dsn, "://"); i >= 0 {
                scheme = dsn[:i+3]
        }
        return scheme + "***" + dsn[at:]
}

func indexOf(s, sub string) int {
        for i := 0; i+len(sub) <= len(s); i++ {
                if s[i:i+len(sub)] == sub {
                        return i
                }
        }
        return -1
}
