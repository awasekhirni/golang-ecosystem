// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// security.go - Security-headers middleware (OWASP A05:2021).
//
// Sets the canonical set of defensive headers on every response. Values are
// tuned for an API server: CSP is permissive (we don't serve first-party
// HTML except /docs/* which loads CDN scripts); HSTS is enabled in prod.
//
// GAP CLOSED: Security headers middleware (OWASP A05:2021)
package middleware

import (
	"net/http"

	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// SecurityHeaders returns the middleware. isProd toggles HSTS include.
func SecurityHeaders(isProd bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			// Prevent clickjacking
			h.Set("X-Frame-Options", "DENY")
			// Prevent MIME sniffing
			h.Set("X-Content-Type-Options", "nosniff")
			// Referrer policy
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			// Disable browser caching for API responses (we have our own ETag layer)
			h.Set("Cache-Control", "no-store")
			// Permissions policy: lock down everything we don't use
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			// Content-Security-Policy: only allow the CDN scripts used by /docs/*
			h.Set("Content-Security-Policy",
				"default-src 'self'; "+
					"script-src 'self' 'unsafe-inline' https://unpkg.com https://cdn.jsdelivr.net; "+
					"style-src 'self' 'unsafe-inline' https://unpkg.com; "+
					"img-src 'self' data:; "+
					"connect-src 'self'")
			// X-XSS-Protection is deprecated but harmless; legacy browsers only
			h.Set("X-XSS-Protection", "1; mode=block")
			// Cross-Origin headers for the OpenAPI/GraphQL docs
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			if isProd {
				// HSTS: 1 year, include subdomains, preload-list eligible
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
			}
			logger.Ctx(r.Context(), logger.LayerMiddleware).
				Debug().Msg("SECURITY_HEADERS_SET")
			next.ServeHTTP(w, r)
		})
	}
}

// MaxBodySize returns a middleware that rejects request bodies larger than
// `maxBytes` with 413 Payload Too Large. Prevents memory-exhaustion DoS.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				http.Error(w, `{"error":{"code":"PAYLOAD_TOO_LARGE","message":"body too large"}}`,
					http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
