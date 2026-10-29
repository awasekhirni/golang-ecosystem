// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// cmd/server/main.go - process entrypoint.
//
// Wires config -> logger -> server.Build -> server.Run. On fatal boot error
// we exit non-zero so the container runtime restarts us.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/config"
	"github.com/ori-inc/hexagonal-go/internal/logger"
	"github.com/ori-inc/hexagonal-go/internal/server"
)

func main() {
	cfg := config.Load()
	logger.Init(cfg.App.LogLevel, cfg.App.LogFormat)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Allow SIGINT/SIGTERM to interrupt boot (e.g. slow DB).
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := server.Build(ctx, cfg)
	if err != nil {
		logger.For(logger.LayerLifecycle).Fatal().Err(err).Msg("BOOT_FAILED")
		os.Exit(1)
	}

	srv := server.New(c)
	if err := srv.Run(); err != nil {
		logger.For(logger.LayerLifecycle).Fatal().Err(err).Msg("RUN_FAILED")
		os.Exit(1)
	}
}
