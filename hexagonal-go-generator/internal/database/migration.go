// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// migration.go is a tiny embeddable migration runner. We intentionally avoid
// golang-migrate to keep the binary footprint small; the algorithm is plain
// SQL applied in lexical order with a schema_migrations ledger table.
//
// Layer-8 logging: every applied file is logged at INFO; failures at ERROR.
//
// GAP CLOSED: #17 Migration runner script (schema-as-code)
package database

import (
        "context"
        "embed"
        "fmt"
        "io/fs"
        "path/filepath"
        "sort"
        "strings"
        "time"

        "github.com/ori-inc/hexagonal-go/internal/logger"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const ledgerDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
        version   TEXT PRIMARY KEY,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

// Migrate applies every *.up.sql file under migrations/ that has not yet been
// recorded in schema_migrations. Files are sorted lexically so naming them
// `000001_create_users.up.sql`, `000002_add_index.up.sql`, etc. keeps order.
func (d *DB) Migrate(ctx context.Context) error {
        L := logger.For(logger.LayerDatabase)

        if _, err := d.ExecContext(ctx, ledgerDDL); err != nil {
                return fmt.Errorf("create ledger: %w", err)
        }

        files, err := fs.Glob(migrationFS, "migrations/*.up.sql")
        if err != nil {
                return fmt.Errorf("glob migrations: %w", err)
        }
        sort.Strings(files)

        for _, f := range files {
                version := strings.TrimSuffix(filepath.Base(f), ".up.sql")

                var exists bool
                if err := d.GetContext(ctx, &exists,
                        `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version); err != nil {
                        return fmt.Errorf("ledger check %s: %w", version, err)
                }
                if exists {
                        L.Debug().Str("version", version).Msg("MIGRATION_SKIP")
                        continue
                }

                body, err := migrationFS.ReadFile(f)
                if err != nil {
                        return fmt.Errorf("read %s: %w", f, err)
                }
                start := time.Now()
                tx, err := d.BeginTxx(ctx, nil)
                if err != nil {
                        return fmt.Errorf("begin tx %s: %w", version, err)
                }
                if _, err := tx.ExecContext(ctx, string(body)); err != nil {
                        _ = tx.Rollback()
                        L.Error().Err(err).Str("version", version).Msg("MIGRATION_FAILED")
                        return fmt.Errorf("apply %s: %w", version, err)
                }
                if _, err := tx.ExecContext(ctx,
                        `INSERT INTO schema_migrations(version) VALUES($1)`, version); err != nil {
                        _ = tx.Rollback()
                        return fmt.Errorf("ledger %s: %w", version, err)
                }
                if err := tx.Commit(); err != nil {
                        return fmt.Errorf("commit %s: %w", version, err)
                }

                L.Info().
                        Str("version", version).
                        Dur("durationMs", time.Since(start)).
                        Msg("MIGRATION_APPLIED")
        }
        return nil
}
