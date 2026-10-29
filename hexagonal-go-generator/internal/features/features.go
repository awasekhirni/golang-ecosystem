// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package features is a tiny feature-flag registry. Flags are env-driven
// for the reference impl; swap the Loader for a LaunchDarkly / Unleash /
// ConfigCat adapter to get hot-reload + UI without touching call sites.
//
// Usage:
//
//	if features.On("use_graphql") {
//	    r.Handle("/graphql", gqlHandler)
//	}
//
// GAP CLOSED: Feature flags (trunk-based delivery)
package features

import (
	"os"
	"strings"
	"sync"
)

// Flag is the registered name. Convention: snake_case, e.g. "use_graphql".
type Flag string

const (
	FlagGraphQL       Flag = "use_graphql"
	FlagOutboxRelay   Flag = "use_outbox_relay"
	FlagOpenAPI       Flag = "use_openapi_ui"
	FlagCompression   Flag = "use_compression"
	FlagOTel          Flag = "use_otel_tracing"
	FlagIdempotency   Flag = "use_idempotency"
	FlagSecurityHdrs  Flag = "use_security_headers"
	FlagETag          Flag = "use_etag"
	FlagProfiling     Flag = "use_pprof"
)

// Registry holds the flag values. Default values are tuned so the reference
// impl ships with everything ON in dev.
type Registry struct {
	mu   sync.RWMutex
	flag map[Flag]bool
}

// New returns a registry populated from the FEATURES env var (comma-sep list
// of `+flag` / `-flag`). Unlisted flags use the defaults below.
func New() *Registry {
	r := &Registry{flag: map[Flag]bool{
		FlagGraphQL:      true,
		FlagOutboxRelay:  true,
		FlagOpenAPI:      true,
		FlagCompression:  true,
		FlagOTel:         true,
		FlagIdempotency:  true,
		FlagSecurityHdrs: true,
		FlagETag:         true,
		FlagProfiling:    false, // opt-in for prod
	}}
	r.Load(os.Getenv("FEATURES"))
	return r
}

// Load parses "+flag,-flag" overrides.
func (r *Registry) Load(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		enable := true
		if tok[0] == '-' {
			enable = false
			tok = tok[1:]
		} else if tok[0] == '+' {
			tok = tok[1:]
		}
		r.flag[Flag(tok)] = enable
	}
}

// On reports whether a flag is enabled.
func (r *Registry) On(f Flag) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.flag[f]
}

// Default is the package-level registry used by the On helper.
var Default = New()

// On is a shortcut for Default.On.
func On(f Flag) bool { return Default.On(f) }
