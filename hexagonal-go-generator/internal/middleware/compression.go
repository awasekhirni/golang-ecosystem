// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// compression.go - gzip response compression + Accept-Encoding negotiation.
//
// Uses a sync.Pool of gzip.Writer instances so allocations stay flat under
// load. Skips compression for small responses (<256 bytes) and for already-
// compressed content types (image/*, video/*, application/zip).
//
// GAP CLOSED: Compression middleware (performance)
package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

const gzipMinSize = 256

var gzPool = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

// Compress returns the gzip middleware.
func Compress() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}
			gw := gzPool.Get().(*gzip.Writer)
			defer gzPool.Put(gw)
			gw.Reset(w)
			defer gw.Close()

			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Del("Content-Length") // length no longer matches

			gzr := &gzipResponseWriter{ResponseWriter: w, Writer: gw}
			next.ServeHTTP(gzr, r)
		})
	}
}

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if len(b) < gzipMinSize {
		return g.ResponseWriter.Write(b)
	}
	// Sniff content-type to skip already-compressed payloads.
	ct := g.Header().Get("Content-Type")
	if strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/") ||
		strings.Contains(ct, "application/zip") || strings.Contains(ct, "application/gzip") {
		return g.ResponseWriter.Write(b)
	}
	if g.Header().Get("Content-Type") == "" && len(b) > 0 {
		g.Header().Set("Content-Type", http.DetectContentType(b))
	}
	return g.Writer.Write(b)
}
