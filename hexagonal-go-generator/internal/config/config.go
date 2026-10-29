// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package config loads, validates and exposes every runtime knob of the
// service. We use envconfig so that missing/invalid env vars fail the boot
// immediately (12-factor "fail fast") rather than producing mysterious
// runtime errors after the first request.
//
// GAP CLOSED: #15 Env validation (fail fast)
package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config is the single source of truth for runtime configuration.
// Every field is loaded from the environment (see .env.example).
type Config struct {
	App App `envconfig:"APP"`

	HTTP HTTP `envconfig:"HTTP"`

	Database Database `envconfig:"DB"`

	JWT   JWT   `envconfig:"JWT"`
	Auth  Auth  `envconfig:"AUTH"`
	CORS  CORS  `envconfig:"CORS"`
	Rate  Rate  `envconfig:"RATE"`
	Probe Probe `envconfig:"PROBE"`
}

type App struct {
	Name      string `envconfig:"NAME"      default:"hexagonal-go"`
	Env       string `envconfig:"ENV"       default:"development"`
	Port      int    `envconfig:"PORT"      default:"8080"`
	Debug     bool   `envconfig:"DEBUG"     default:"true"`
	LogLevel  string `envconfig:"LOG_LEVEL" default:"info"`
	LogFormat string `envconfig:"LOG_FORMAT" default:"json"`
}

type HTTP struct {
	ReadTimeout      time.Duration `envconfig:"READ_TIMEOUT"      default:"15s"`
	WriteTimeout     time.Duration `envconfig:"WRITE_TIMEOUT"     default:"20s"`
	IdleTimeout      time.Duration `envconfig:"IDLE_TIMEOUT"      default:"60s"`
	ShutdownTimeout  time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"30s"`
}

type Database struct {
	URL         string        `envconfig:"URL"        required:"true"`
	PoolSize    int           `envconfig:"POOL_SIZE"  default:"10"`
	MaxOverflow int           `envconfig:"MAX_OVERFLOW" default:"20"`
	PoolRecycle time.Duration `envconfig:"POOL_RECYCLE" default:"300s"`
	SlowQueryMs int           `envconfig:"SLOW_QUERY_MS" default:"100"`
}

type JWT struct {
	Secret     string        `envconfig:"SECRET"      required:"true"`
	Issuer     string        `envconfig:"ISSUER"      default:"ori-inc"`
	AccessTTL  time.Duration `envconfig:"ACCESS_TTL"  default:"15m"`
	RefreshTTL time.Duration `envconfig:"REFRESH_TTL" default:"720h"`
}

type Auth struct {
	APIKeys []string `envconfig:"API_KEYS" default:"dev-key-1"`
}

type CORS struct {
	AllowedOrigins []string `envconfig:"ALLOWED_ORIGINS" default:"*"`
	AllowedMethods []string `envconfig:"ALLOWED_METHODS" default:"GET,POST,PUT,DELETE,PATCH,OPTIONS"`
}

type Rate struct {
	LimitRPS  float64 `envconfig:"LIMIT_RPS"  default:"50"`
	LimitBurst int    `envconfig:"LIMIT_BURST" default:"100"`
}

type Probe struct {
	LivenessCheckDB  bool `envconfig:"LIVENESS_CHECK_DB"  default:"true"`
	ReadinessCheckDB bool `envconfig:"READINESS_CHECK_DB" default:"true"`
}

// Load reads the environment, validates it, and returns a fully populated Config.
// It panics on any validation failure so the process dies at boot with a clear
// message instead of serving broken traffic.
func Load() *Config {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		log.Fatalf("[config] env validation failed (fail-fast): %v", err)
	}
	c.validate()
	return &c
}

// validate applies business rules that envconfig cannot express on its own
// (cross-field invariants, semantic checks, security warnings).
func (c *Config) validate() {
	var problems []string

	if c.App.Env != "development" && c.App.Env != "staging" && c.App.Env != "production" {
		problems = append(problems, fmt.Sprintf("APP.ENV must be development|staging|production, got %q", c.App.Env))
	}
	if c.App.Env == "production" && c.App.Debug {
		problems = append(problems, "APP.DEBUG must be false in production")
	}
	if c.App.Env == "production" && len(c.JWT.Secret) < 32 {
		problems = append(problems, "JWT.SECRET must be >=32 bytes in production")
	}
	if c.Database.URL == "" {
		problems = append(problems, "DB.URL is required")
	}
	if c.HTTP.ShutdownTimeout < 1*time.Second {
		problems = append(problems, "HTTP.SHUTDOWN_TIMEOUT must be >=1s for graceful drain")
	}
	if c.Database.SlowQueryMs < 1 {
		problems = append(problems, "DB.SLOW_QUERY_MS must be >=1")
	}
	if c.Rate.LimitRPS <= 0 {
		problems = append(problems, "RATE.LIMIT_RPS must be > 0")
	}

	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, "[config] validation failures:")
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
		log.Fatalf("[config] %d validation problem(s); refusing to boot", len(problems))
	}
}

// IsProduction is a tiny convenience used across the codebase.
func (c *Config) IsProduction() bool { return strings.EqualFold(c.App.Env, "production") }
