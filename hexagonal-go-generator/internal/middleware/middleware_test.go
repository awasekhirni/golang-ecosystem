// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// middleware_test.go - unit tests for the middleware stack that don't need
// a database. Run with: go test -short ./internal/middleware/...
package middleware

import (
        "context"
        "io"
        "net/http"
        "net/http/httptest"
        "strings"
        "testing"
        "time"

        "github.com/ori-inc/hexagonal-go/internal/logger"
)

// --- Recover ----------------------------------------------------------------

func TestRecover_NoPanic(t *testing.T) {
        called := false
        h := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                called = true
                w.WriteHeader(200)
        }))
        rec := httptest.NewRecorder()
        h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
        if !called {
                t.Fatal("handler not called")
        }
        if rec.Code != 200 {
                t.Fatalf("expected 200, got %d", rec.Code)
        }
}

func TestRecover_Panic(t *testing.T) {
        h := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                panic("boom")
        }))
        rec := httptest.NewRecorder()
        h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
        if rec.Code != 500 {
                t.Fatalf("expected 500 on panic, got %d", rec.Code)
        }
}

// --- RequestID --------------------------------------------------------------

func TestRequestID_MintsWhenAbsent(t *testing.T) {
        var gotID string
        h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                gotID = logger.RequestIDFrom(r.Context())
                w.WriteHeader(200)
        }))
        rec := httptest.NewRecorder()
        h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
        if gotID == "" {
                t.Fatal("expected minted request id")
        }
        if rec.Header().Get(HeaderRequestID) != gotID {
                t.Fatal("response header should carry the same id")
        }
}

func TestRequestID_PreservesIncoming(t *testing.T) {
        incoming := "incoming-id-123"
        h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if id := logger.RequestIDFrom(r.Context()); id != incoming {
                        t.Fatalf("expected %q, got %q", incoming, id)
                }
        }))
        rec := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/", nil)
        req.Header.Set(HeaderRequestID, incoming)
        h.ServeHTTP(rec, req)
}

// --- JWTAuth ----------------------------------------------------------------

func TestJWTAuth_NoToken_PassesThrough(t *testing.T) {
        called := false
        h := JWTAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                called = true
                if PrincipalFrom(r.Context()) != nil {
                        t.Fatal("expected nil principal without token")
                }
        }))
        rec := httptest.NewRecorder()
        h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
        if !called {
                t.Fatal("expected pass-through without token")
        }
}

func TestJWTAuth_ValidToken_SetsPrincipal(t *testing.T) {
        tok, err := IssueJWT("secret", "test", "user-1", []Role{RoleUser}, time.Hour)
        if err != nil {
                t.Fatal(err)
        }
        var p *Principal
        h := JWTAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                p = PrincipalFrom(r.Context())
        }))
        rec := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/", nil)
        req.Header.Set("Authorization", "Bearer "+tok)
        h.ServeHTTP(rec, req)
        if p == nil || p.UserID != "user-1" {
                t.Fatalf("expected principal user-1, got %v", p)
        }
}

func TestJWTAuth_InvalidToken_401(t *testing.T) {
        h := JWTAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                t.Fatal("should not call next on invalid token")
        }))
        rec := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/", nil)
        req.Header.Set("Authorization", "Bearer garbage")
        h.ServeHTTP(rec, req)
        if rec.Code != 401 {
                t.Fatalf("expected 401, got %d", rec.Code)
        }
}

// --- RBAC -------------------------------------------------------------------

func TestRequire_Anonymous_401(t *testing.T) {
        h := Require(RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                t.Fatal("should not call next when anonymous")
        }))
        rec := httptest.NewRecorder()
        h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
        if rec.Code != 401 {
                t.Fatalf("expected 401, got %d", rec.Code)
        }
}

func TestRequire_WrongRole_403(t *testing.T) {
        h := Require(RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                t.Fatal("should not call next with wrong role")
        }))
        rec := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/", nil)
        req = req.WithContext(WithPrincipal(context.Background(), &Principal{
                UserID: "u1", Roles: []Role{RoleUser},
        }))
        h.ServeHTTP(rec, req)
        if rec.Code != 403 {
                t.Fatalf("expected 403, got %d", rec.Code)
        }
}

func TestRequire_CorrectRole_200(t *testing.T) {
        called := false
        h := Require(RoleAdmin, RoleUser)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                called = true
        }))
        rec := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/", nil)
        req = req.WithContext(WithPrincipal(context.Background(), &Principal{
                UserID: "u1", Roles: []Role{RoleUser},
        }))
        h.ServeHTTP(rec, req)
        if !called {
                t.Fatal("expected handler to be called")
        }
}

// --- Idempotency ------------------------------------------------------------

func TestIdempotency_Dedup(t *testing.T) {
        calls := 0
        h := Idempotency(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                calls++
                w.WriteHeader(201)
                _, _ = w.Write([]byte(`{"id":"abc"}`))
        }))

        body := `{"x":1}`
        // First call.
        rec1 := httptest.NewRecorder()
        req1 := httptest.NewRequest("POST", "/api/v1/users", stringReader(body))
        req1.Header.Set(HeaderIdempotencyKey, "k1")
        h.ServeHTTP(rec1, req1)
        if rec1.Code != 201 {
                t.Fatalf("first: expected 201, got %d", rec1.Code)
        }

        // Replay.
        rec2 := httptest.NewRecorder()
        req2 := httptest.NewRequest("POST", "/api/v1/users", stringReader(body))
        req2.Header.Set(HeaderIdempotencyKey, "k1")
        h.ServeHTTP(rec2, req2)
        if rec2.Code != 201 {
                t.Fatalf("replay: expected 201, got %d", rec2.Code)
        }
        if rec2.Body.String() != rec1.Body.String() {
                t.Fatalf("replay body mismatch: %q vs %q", rec2.Body.String(), rec1.Body.String())
        }
        if calls != 1 {
                t.Fatalf("expected handler called once, got %d", calls)
        }
}

func TestIdempotency_NoKey_PassesThrough(t *testing.T) {
        calls := 0
        h := Idempotency(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                calls++
                w.WriteHeader(200)
        }))
        // GET passes through (not a mutation).
        h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
        // POST without key also passes through (no dedup).
        h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/", stringReader("{}")))
        // Both should reach the handler since neither has an Idempotency-Key.
        if calls != 2 {
                t.Fatalf("expected 2 handler calls (GET + POST, no key), got %d", calls)
        }
}

// --- ETag -------------------------------------------------------------------

func TestETag_304_OnMatch(t *testing.T) {
        body := `{"hello":"world"}`
        h := ETag(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(200)
                _, _ = w.Write([]byte(body))
        }))

        // First GET - capture ETag.
        rec1 := httptest.NewRecorder()
        h.ServeHTTP(rec1, httptest.NewRequest("GET", "/x", nil))
        etag := rec1.Header().Get(HeaderETag)
        if etag == "" {
                t.Fatal("expected ETag header")
        }

        // Second GET with If-None-Match - should 304.
        rec2 := httptest.NewRecorder()
        req2 := httptest.NewRequest("GET", "/x", nil)
        req2.Header.Set("If-None-Match", etag)
        h.ServeHTTP(rec2, req2)
        if rec2.Code != 304 {
                t.Fatalf("expected 304, got %d", rec2.Code)
        }
}

// --- Security headers -------------------------------------------------------

func TestSecurityHeaders_AllPresent(t *testing.T) {
        h := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(200)
        }))
        rec := httptest.NewRecorder()
        h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
        checks := map[string]string{
                "X-Frame-Options":          "DENY",
                "X-Content-Type-Options":   "nosniff",
                "Referrer-Policy":          "strict-origin-when-cross-origin",
                "Cache-Control":            "no-store",
        }
        for h, v := range checks {
                if got := rec.Header().Get(h); got != v {
                        t.Fatalf("expected %s=%q, got %q", h, v, got)
                }
        }
}

func TestSecurityHeaders_HSTS_InProd(t *testing.T) {
        h := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
        rec := httptest.NewRecorder()
        h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
        if v := rec.Header().Get("Strict-Transport-Security"); v == "" {
                t.Fatal("expected HSTS in prod")
        }
}

// --- MaxBodySize ------------------------------------------------------------

func TestMaxBodySize_Reject(t *testing.T) {
        h := MaxBodySize(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                t.Fatal("should not call next on oversized body")
        }))
        rec := httptest.NewRecorder()
        req := httptest.NewRequest("POST", "/", stringReader("this is way too long"))
        req.ContentLength = 24
        h.ServeHTTP(rec, req)
        if rec.Code != 413 {
                t.Fatalf("expected 413, got %d", rec.Code)
        }
}

// --- RateLimit --------------------------------------------------------------

func TestRateLimit_AllowsUnderBurst(t *testing.T) {
        h := RateLimit(100, 10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(200)
        }))
        for i := 0; i < 5; i++ {
                rec := httptest.NewRecorder()
                h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
                if rec.Code != 200 {
                        t.Fatalf("call %d: expected 200, got %d", i, rec.Code)
                }
        }
}

// stringReader is a tiny io.Reader for test bodies.
type stringReaderImpl struct{ s string; i int }

func (r *stringReaderImpl) Read(p []byte) (int, error) {
        if r.i >= len(r.s) {
                return 0, io.EOF
        }
        n := copy(p, r.s[r.i:])
        r.i += n
        return n, nil
}

func stringReader(s string) *stringReaderImpl { return &stringReaderImpl{s: s} }

// keep strings import alive (used in MaxBodySize test body)
var _ = strings.NewReader
