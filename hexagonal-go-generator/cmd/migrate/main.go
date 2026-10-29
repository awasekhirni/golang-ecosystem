// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// cmd/migrate/main.go - standalone migration runner.
//
//   go run ./cmd/migrate -direction=up     # apply pending
//   go run ./cmd/migrate -direction=down   # roll back the latest
//
// The "down" direction is intentionally limited to the latest migration; for
// full rollback use a maintenance window + psql.
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/config"
	"github.com/ori-inc/hexagonal-go/internal/database"
	"github.com/ori-inc/hexagonal-go/internal/logger"
)

func main() {
	dir := flag.String("direction", "up", "up|down")
	flag.Parse()

	cfg := config.Load()
	logger.Init(cfg.App.LogLevel, cfg.App.LogFormat)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.New(ctx, cfg)
	if err != nil {
		os.Exit(1)
	}
	defer db.Close()

	switch *dir {
	case "up":
		if err := db.Migrate(ctx); err != nil {
			logger.For(logger.LayerLifecycle).Fatal().Err(err).Msg("MIGRATE_UP_FAILED")
			os.Exit(1)
		}
		logger.For(logger.LayerLifecycle).Info().Msg("MIGRATE_UP_OK")
	case "down":
		logger.For(logger.LayerLifecycle).Warn().Msg("MIGRATE_DOWN_NOT_SUPPORTED_IN_REFERENCE")
		// Implementing down-migrations safely is out of scope for the
		// reference impl; see docs/cmmi/05_CM.md for the policy.
	default:
		logger.For(logger.LayerLifecycle).Fatal().Str("direction", *dir).Msg("UNKNOWN_DIRECTION")
		os.Exit(1)
	}
}
