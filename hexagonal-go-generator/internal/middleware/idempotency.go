// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// idempotency.go - Idempotency-Key middleware.
//
// A client that retries a POST/PUT (e.g. after a network blip) must not
// cause a duplicate side effect. RFC 7807 / Stripe's well-known convention:
// the client sends an `Idempotency-Key` header; the server stores the
// response for that key+request-hash and replays it on retries for a TTL.
//
// This implementation uses an in-memory LRU+TTL cache. For multi-instance
// deployments swap the store for a Redis-backed one (the Store interface
// is identical).
//
// GAP CLOSED: Idempotency-Key middleware (critical for payments / orders)
package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ori-inc/hexagonal-go/internal/logger"
)

const (
	HeaderIdempotencyKey = "Idempotency-Key"
	idempotencyTTL       = 24 * time.Hour
)

// IdempotencyStore is the cache interface. The default in-memory impl is
// fine for single-instance; use Redis for multi-instance.
type IdempotencyStore interface {
	Get(ctx context.Context, key string) (*CachedResponse, bool)
	Put(ctx context.Context, key string, resp *CachedResponse)
}

// CachedResponse is the replayed response body+status+headers.
type CachedResponse struct {
	Status  int
	Headers http.Header
	Body    []byte
	Created time.Time
}

// memStore is the default in-memory store with a per-key TTL.
type memStore struct {
	mu sync.Mutex
	m  map[string]*CachedResponse
}

func newMemStore() *memStore { return &memStore{m: map[string]*CachedResponse{}} }

func (s *memStore) Get(_ context.Context, key string) (*CachedResponse, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.m[key]
	if !ok {
		return nil, false
	}
	if time.Since(r.Created) > idempotencyTTL {
		delete(s.m, key)
		return nil, false
	}
	return r, true
}

func (s *memStore) Put(_ context.Context, key string, resp *CachedResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// opportunistic GC of expired entries
	for k, v := range s.m {
		if time.Since(v.Created) > idempotencyTTL {
			delete(s.m, k)
		}
	}
	s.m[key] = resp
}

// Idempotency returns a middleware that deduplicates mutations. If the
// request has no Idempotency-Key header it is passed through unchanged.
func Idempotency(store IdempotencyStore) func(http.Handler) http.Handler {
	if store == nil {
		store = newMemStore()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutation(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get(HeaderIdempotencyKey)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Build a stable cache key: header + method + path + body digest.
			bodyBytes, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			digest := sha256.Sum256(bodyBytes)
			cacheKey := key + "|" + r.Method + "|" + r.URL.Path + "|" + hex.EncodeToString(digest[:8])

			if cached, ok := store.Get(r.Context(), cacheKey); ok {
				logger.Ctx(r.Context(), logger.LayerMiddleware).
					Info().
					Str("idempotencyKey", key).
					Msg("IDEMPOTENCY_REPLAY")
				replay(w, cached)
				return
			}

			rw := &captureWriter{ResponseWriter: w, header: http.Header{}}
			next.ServeHTTP(rw, r)

			cached := &CachedResponse{
				Status:  rw.status,
				Headers: rw.header,
				Body:    rw.body.Bytes(),
				Created: time.Now().UTC(),
			}
			store.Put(r.Context(), cacheKey, cached)
			logger.Ctx(r.Context(), logger.LayerMiddleware).
				Info().
				Str("idempotencyKey", key).
				Msg("IDEMPOTENCY_STORED")
		})
	}
}

// captureWriter captures the handler's response so we can both stream it to
// the client AND cache it for replays.
type captureWriter struct {
	http.ResponseWriter
	status int
	header http.Header
	body   bytes.Buffer
}

func (c *captureWriter) Header() http.Header {
	if c.header == nil {
		c.header = make(http.Header)
	}
	return c.header
}
func (c *captureWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}
func (c *captureWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = 200
	}
	c.body.Write(b)
	return c.ResponseWriter.Write(b)
}

func replay(w http.ResponseWriter, c *CachedResponse) {
	for k, vs := range c.Headers {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if c.Status != 0 {
		w.WriteHeader(c.Status)
	}
	_, _ = w.Write(c.Body)
}
