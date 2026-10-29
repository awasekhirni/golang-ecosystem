// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// etag.go - ETag + conditional request middleware (RFC 7232).
//
// For GET/HEAD requests the middleware buffers the response body, computes a
// weak ETag (SHA-256 prefix), and:
//   - if the client sent If-None-Match with a matching ETag, returns 304 Not
//     Modified WITHOUT the body;
//   - otherwise stamps the ETag header and streams the buffered body.
//
// For mutations that take a version, the controller checks If-Match against
// the entity's version itself (the UoW already enforces optimistic
// concurrency via the version column).
//
// GAP CLOSED: ETag + conditional requests (REST caching + concurrency)
package middleware

import (
        "bytes"
        "crypto/sha256"
        "encoding/hex"
        "net/http"
        "strconv"

        "github.com/ori-inc/hexagonal-go/internal/logger"
)

const HeaderETag = "ETag"

// ETag computes a weak ETag for the response body and short-circuits to 304
// when the client already has it.
func ETag(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if r.Method != http.MethodGet && r.Method != http.MethodHead {
                        next.ServeHTTP(w, r)
                        return
                }

                cw := &etagWriter{header: make(http.Header)}
                next.ServeHTTP(cw, r)

                if cw.body.Len() == 0 {
                        // No body — just forward whatever the handler wrote.
                        flushETagResponse(w, cw, "")
                        return
                }

                sum := sha256.Sum256(cw.body.Bytes())
                etag := `W/"` + hex.EncodeToString(sum[:8]) + `"`
                cw.header.Set(HeaderETag, etag)

                if inm := r.Header.Get("If-None-Match"); inm == etag {
                        logger.Ctx(r.Context(), logger.LayerMiddleware).
                                Debug().Str("etag", etag).Msg("ETAG_304")
                        w.Header().Set(HeaderETag, etag)
                        w.WriteHeader(http.StatusNotModified)
                        return
                }
                flushETagResponse(w, cw, etag)
        })
}

// flushETagResponse copies the buffered headers + body to the real writer.
func flushETagResponse(w http.ResponseWriter, cw *etagWriter, etag string) {
        for k, vs := range cw.header {
                for _, v := range vs {
                        w.Header().Add(k, v)
                }
        }
        if etag != "" {
                w.Header().Set(HeaderETag, etag)
        }
        if cw.status != 0 {
                w.WriteHeader(cw.status)
        } else {
                w.WriteHeader(200)
        }
        _, _ = w.Write(cw.body.Bytes())
}

// etagWriter buffers the response so we can compute the ETag before flushing.
type etagWriter struct {
        header http.Header
        status int
        body   bytes.Buffer
}

func (e *etagWriter) Header() http.Header {
        if e.header == nil {
                e.header = make(http.Header)
        }
        return e.header
}
func (e *etagWriter) WriteHeader(code int) { e.status = code }
func (e *etagWriter) Write(b []byte) (int, error) {
        if e.status == 0 {
                e.status = 200
        }
        return e.body.Write(b)
}

// ContentLength returns the buffered body length (for tests).
func (e *etagWriter) ContentLength() int { return e.body.Len() }

// keep strconv import for potential future use (Content-Length stamping).
var _ = strconv.Itoa
