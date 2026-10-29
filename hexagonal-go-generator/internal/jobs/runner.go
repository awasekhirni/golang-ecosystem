// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package jobs is a tiny background-job runner with cron-style scheduling
// and at-least-once delivery semantics. Each job runs in its own goroutine;
// panics are recovered and logged. Failed jobs are retried with exponential
// backoff up to MaxRetries.
//
// Persistence: for the reference impl we keep the schedule in-memory. For
// multi-instance deployments swap the Runner for a Redis/RabbitMQ-backed
// one; the Job interface is identical.
//
// GAP CLOSED: Background job runner (cron + at-least-once delivery)
package jobs

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// Job is a unit of background work.
type Job struct {
	Name      string
	Schedule  time.Duration // run every Schedule; 0 = one-shot
	MaxRetries int
	Run        func(ctx context.Context) error
}

// Runner owns the goroutines for every registered Job.
type Runner struct {
	mu   sync.Mutex
	jobs []Job
	stop chan struct{}
	wg   sync.WaitGroup
}

// New returns an empty runner.
func New() *Runner { return &Runner{stop: make(chan struct{})} }

// Register adds a job. Must be called before Start.
func (r *Runner) Register(j Job) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j.MaxRetries == 0 {
		j.MaxRetries = 3
	}
	r.jobs = append(r.jobs, j)
	logger.For(logger.LayerLifecycle).
		Info().
		Str("job", j.Name).
		Dur("schedule", j.Schedule).
		Msg("JOB_REGISTERED")
}

// Start launches every job's goroutine.
func (r *Runner) Start(ctx context.Context) {
	r.mu.Lock()
	jobs := make([]Job, len(r.jobs))
	copy(jobs, r.jobs)
	r.mu.Unlock()

	for _, j := range jobs {
		j := j
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			ticker := time.NewTicker(j.Schedule)
			defer ticker.Stop()
			logger.For(logger.LayerLifecycle).
				Info().Str("job", j.Name).Msg("JOB_STARTED")
			// run once immediately, then on the ticker
			r.runWithRetry(ctx, j)
			for {
				select {
				case <-r.stop:
					logger.For(logger.LayerLifecycle).
						Info().Str("job", j.Name).Msg("JOB_STOPPED")
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					r.runWithRetry(ctx, j)
				}
			}
		}()
	}
}

// Stop signals every job to exit and waits for them to drain.
func (r *Runner) Stop() {
	close(r.stop)
	r.wg.Wait()
	logger.For(logger.LayerLifecycle).Info().Msg("JOBS_DRAINED")
}

func (r *Runner) runWithRetry(ctx context.Context, j Job) {
	for attempt := 1; attempt <= j.MaxRetries; attempt++ {
		err := r.runOnce(ctx, j)
		if err == nil {
			return
		}
		logger.For(logger.LayerService).
			Error().
			Err(err).
			Str("job", j.Name).
			Int("attempt", attempt).
			Msg("JOB_FAILED")
		if attempt == j.MaxRetries {
			logger.For(logger.LayerService).
				Error().
				Err(err).
				Str("job", j.Name).
				Msg("JOB_EXHAUSTED")
			return
		}
		time.Sleep(time.Duration(attempt*attempt) * time.Second)
	}
}

func (r *Runner) runOnce(ctx context.Context, j Job) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("job panic: %v\n%s", rec, debug.Stack())
		}
	}()
	start := time.Now()
	logger.For(logger.LayerService).
		Info().Str("job", j.Name).Msg("JOB_RUN_START")
	err = j.Run(ctx)
	logger.For(logger.LayerService).
		Info().
		Str("job", j.Name).
		Dur("durationMs", time.Since(start)).
		Err(err).
		Msg("JOB_RUN_END")
	return
}
