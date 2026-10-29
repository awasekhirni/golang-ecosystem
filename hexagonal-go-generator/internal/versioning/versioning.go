// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package versioning implements API-versioning policy enforcement.
//
// Strategy: path-based versioning (`/api/v1/...`, `/api/v2/...`). The
// middleware reads the version from the path and stamps two headers on the
// response:
//
//   API-Version:      the served version (e.g. "v1")
//   Deprecation:      present + date if the version is deprecated
//   Sunset:           present + date if the version is scheduled for removal
//
// Deprecation/Sunset follow RFC 8594 / the IETF deprecation header draft so
// client libraries can surface warnings automatically.
//
// GAP CLOSED: API versioning strategy + Deprecation/Sunset headers
package versioning

import (
	"net/http"
	"strings"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// Policy declares per-version deprecation/sunset dates. Zero time means
// "not deprecated" / "no sunset".
type Policy struct {
	Deprecated map[string]time.Time // version -> deprecation date
	Sunset     map[string]time.Time // version -> sunset date
}

// Default is a sane starting policy: v1 is current.
func Default() Policy {
	return Policy{
		Deprecated: map[string]time.Time{},
		Sunset:     map[string]time.Time{},
	}
}

// Middleware returns the versioning middleware. Mount it OUTSIDE the
// /api/v1, /api/v2 route groups so every response gets the headers.
func Middleware(p Policy) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ver := extractVersion(r.URL.Path)
			if ver != "" {
				w.Header().Set("API-Version", ver)
				if d, ok := p.Deprecated[ver]; ok {
					w.Header().Set("Deprecation", d.UTC().Format(time.RFC1123))
					logger.Ctx(r.Context(), logger.LayerMiddleware).
						Info().Str("version", ver).Msg("API_DEPRECATED_VERSION_USED")
				}
				if s, ok := p.Sunset[ver]; ok {
					w.Header().Set("Sunset", s.UTC().Format(time.RFC1123))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractVersion returns "v1" for /api/v1/users, "" for non-API paths.
func extractVersion(path string) string {
	if !strings.HasPrefix(path, "/api/") {
		return ""
	}
	rest := strings.TrimPrefix(path, "/api/")
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return rest
	}
	return rest[:slash]
}
