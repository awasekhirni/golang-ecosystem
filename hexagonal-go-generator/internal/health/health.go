// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package health implements a composite health subsystem that distinguishes
// four probe types, matching the Kubernetes probe taxonomy:
//
//   /healthz/startup   - Startup probe: did the app finish booting? (K8s
//                        won't route traffic until this returns 200 once.)
//   /healthz/live      - Liveness probe: is the process not deadlocked?
//                        (NEVER checks downstream deps to avoid restart
//                        storms during a DB blip.)
//   /healthz/ready     - Readiness probe: can we accept traffic? Checks
//                        downstream deps with a short timeout.
//   /healthz/deep      - Deep health: runs every registered checker with
//                        full timeout; intended for dashboards, not K8s.
//
// Each checker returns a Check result with status up/down/degraded +
// latency + optional message. The composite handler aggregates them into
// a single JSON document.
//
// GAP CLOSED: Composite health subsystem (startup/liveness/readiness/deep)
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// Status is the per-check verdict.
type Status string

const (
	StatusUp       Status = "up"
	StatusDown     Status = "down"
	StatusDegraded Status = "degraded"
)

// Check is one component's result.
type Check struct {
	Name     string        `json:"name"`
	Status   Status        `json:"status"`
	Latency  time.Duration `json:"latencyMs"`
	Message  string        `json:"message,omitempty"`
}

// Checker is a function that pings one dependency. It MUST respect ctx
// (use a sub-context with timeout) so a slow dep doesn't stall the probe.
type Checker func(ctx context.Context) error

// Subsystem holds the registered checkers and serves the four probes.
type Subsystem struct {
	mu       sync.RWMutex
	checkers map[string]Checker
	booted   bool
}

// New returns an empty subsystem.
func New() *Subsystem { return &Subsystem{checkers: map[string]Checker{}} }

// Register adds a checker. Call during boot.
func (s *Subsystem) Register(name string, c Checker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkers[name] = c
	logger.For(logger.LayerLifecycle).
		Info().Str("checker", name).Msg("HEALTH_CHECKER_REGISTERED")
}

// MarkBooted flips the startup flag. Call once boot completes successfully.
func (s *Subsystem) MarkBooted() {
	s.mu.Lock()
	s.booted = true
	s.mu.Unlock()
	logger.For(logger.LayerLifecycle).Info().Msg("HEALTH_BOOTED")
}

// --- handlers ---------------------------------------------------------------

// StartupHandler returns 200 once MarkBooted has been called; 503 before.
func (s *Subsystem) StartupHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		booted := s.booted
		s.mu.RUnlock()
		if !booted {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"starting"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"started"}`))
	}
}

// LivenessHandler never checks deps. It only returns 200 if the process is
// alive (which it obviously is, since it's answering).
func (s *Subsystem) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"alive"}`))
	}
}

// ReadinessHandler runs every checker with a 1s budget; if any is down,
// returns 503 so K8s stops routing traffic.
func (s *Subsystem) ReadinessHandler() http.HandlerFunc {
	return s.aggregateHandler(1 * time.Second)
}

// DeepHandler runs every checker with a 5s budget; always returns 200 with
// the full report (intended for dashboards, not K8s).
func (s *Subsystem) DeepHandler() http.HandlerFunc {
	return s.aggregateHandler(5 * time.Second)
}

func (s *Subsystem) aggregateHandler(timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		s.mu.RLock()
		names := make([]string, 0, len(s.checkers))
		checkers := make([]Checker, 0, len(s.checkers))
		for n, c := range s.checkers {
			names = append(names, n)
			checkers = append(checkers, c)
		}
		s.mu.RUnlock()

		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			results []Check
			anyDown bool
		)
		for i, c := range checkers {
			wg.Add(1)
			go func(name string, fn Checker) {
				defer wg.Done()
				start := time.Now()
				err := fn(ctx)
				latency := time.Since(start)
				chk := Check{Name: name, Latency: latency}
				switch {
				case err == nil:
					chk.Status = StatusUp
				default:
					chk.Status = StatusDown
					chk.Message = err.Error()
					mu.Lock()
					anyDown = true
					mu.Unlock()
				}
				mu.Lock()
				results = append(results, chk)
				mu.Unlock()
			}(names[i], c)
		}
		wg.Wait()

		status := StatusUp
		httpStatus := http.StatusOK
		if anyDown {
			status = StatusDown
			httpStatus = http.StatusServiceUnavailable
		}
		out := map[string]any{
			"status":   status,
			"checks":   results,
			"serverTime": time.Now().UTC().Format(time.RFC3339Nano),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(out)

		logger.For(logger.LayerLifecycle).
			Info().
			Str("status", string(status)).
			Int("httpStatus", httpStatus).
			Int("checks", len(results)).
			Msg("HEALTH_PROBE")
	}
}
