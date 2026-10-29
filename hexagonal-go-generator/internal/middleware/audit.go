// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// audit.go - audit-log middleware for all mutations (POST/PUT/PATCH/DELETE).
//
// Every mutation produces a single structured record at the Middleware layer
// with: requestId, userId, method, path, status, ip, userAgent, and (for the
// audit consumer) a JSON body digest. We never log full request bodies for
// PII reasons; we log a SHA-256 digest so audit trail integrity can be
// verified without leaking secrets.
//
// GAP CLOSED: #4 Audit log for all mutations (SOC2)
package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/ori-inc/hexagonal-go/internal/logger"
	"github.com/ori-inc/hexagonal-go/pkg/metrics"
)

// isMutation reports whether the HTTP method modifies state.
func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// Audit wraps the handler. For mutations it reads the body once (then
// restores it via NopCloser so the downstream handler still sees it),
// computes a digest, and emits an audit log line.
func Audit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutation(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		p := PrincipalFrom(r.Context())
		userID := ""
		if p != nil {
			userID = p.UserID
		}

		var digest string
		if r.Body != nil {
			body, err := io.ReadAll(r.Body)
			if err == nil {
				if len(body) > 0 {
					sum := sha256.Sum256(body)
					digest = hex.EncodeToString(sum[:])
				}
				// restore body for downstream handler
				r.Body = io.NopCloser(readerFromBytes(body))
			}
		}

		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)

		status := sw.status
		if status == 0 {
			status = 200
		}

		logger.Ctx(r.Context(), logger.LayerMiddleware).
			Info().
			Str("audit", "true").
			Str("userId", userID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", status).
			Str("ip", clientIP(r)).
			Str("userAgent", r.UserAgent()).
			Str("bodyDigest", digest).
			Msg("AUDIT_MUTATION")

		metrics.AuditEvents.WithLabelValues(r.Method + " " + r.URL.Path).Inc()
	})
}

// readerFromBytes returns a bytes.Reader as an io.Reader. Kept here to avoid
// an extra bytes import in the main middleware file.
func readerFromBytes(b []byte) io.Reader {
	return &bytesReader{b: b}
}

type bytesReader struct{ b []byte; i int }

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
