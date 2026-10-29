// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// features_integration_test.go - end-to-end tests for every second-pass
// enhancement. Requires a running PostgreSQL (DATABASE_URL env var).
//
// Run with:
//   go test -race -count=1 -tags=integration ./tests/...
//
//go:build integration

package tests

import (
        "bytes"
        "context"
        "encoding/json"
        "io"
        "net/http"
        "net/http/httptest"
        "os"
        "strings"
        "testing"
        "time"

        "github.com/jmoiron/sqlx"
        "github.com/ori-inc/hexagonal-go/internal/config"
        "github.com/ori-inc/hexagonal-go/internal/server"
)

// testEnv holds a fully-wired server backed by a real PostgreSQL.
type testEnv struct {
        srv    *httptest.Server
        db     *sqlx.DB
        token  string
        apiKey string
}

func setup(t *testing.T) *testEnv {
        t.Helper()
        dsn := os.Getenv("DATABASE_URL")
        if dsn == "" {
                t.Skip("DATABASE_URL not set; skipping integration test")
        }

        cfg := &config.Config{
                App:     config.App{Env: "development", Port: 0, Debug: true, LogLevel: "info", LogFormat: "console"},
                HTTP:    config.HTTP{ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, ShutdownTimeout: 5 * time.Second},
                Database: config.Database{URL: dsn, PoolSize: 5, MaxOverflow: 10, PoolRecycle: 60 * time.Second, SlowQueryMs: 100},
                JWT:     config.JWT{Secret: "test-secret-32-bytes-long-xxxxxxxxxx", Issuer: "test", AccessTTL: 1 * time.Hour, RefreshTTL: 24 * time.Hour},
                Auth:    config.Auth{APIKeys: []string{"test-key"}},
                CORS:    config.CORS{AllowedOrigins: []string{"*"}, AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}},
                Rate:    config.Rate{LimitRPS: 1000, LimitBurst: 2000},
                Probe:   config.Probe{LivenessCheckDB: true, ReadinessCheckDB: true},
        }

        ctx := context.Background()
        db, err := sqlx.Connect("pgx", dsn)
        if err != nil {
                t.Fatalf("connect: %v", err)
        }
        // Clean slate.
        _, _ = db.Exec("TRUNCATE users, outbox RESTART IDENTITY CASCADE")

        c, err := server.Build(ctx, cfg)
        if err != nil {
                t.Fatalf("build: %v", err)
        }

        srv := server.New(c)
        ts := httptest.NewServer(srv.Handler())
        if ts == nil {
                // server.New doesn't expose Handler; we use httptest with the
                // http.Server directly. Fall back to a manual listener.
        }

        // Mint a token via the demo endpoint.
        token := ""
        if resp, err := http.Get(ts.URL + "/auth/demo-token"); err == nil {
                var body map[string]string
                _ = json.NewDecoder(resp.Body).Decode(&body)
                token = body["token"]
                resp.Body.Close()
        }

        return &testEnv{srv: ts, db: db, token: token, apiKey: "test-key"}
}

func (e *testEnv) teardown() {
        e.srv.Close()
        _ = e.db.Close()
}

func authHeaders(e *testEnv) http.Header {
        h := http.Header{}
        h.Set("Authorization", "Bearer "+e.token)
        h.Set("Content-Type", "application/json")
        return h
}

// =============================================================================
// Test: OpenAPI spec is served and valid
// =============================================================================

func TestIntegration_OpenAPI_Served(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        resp, err := http.Get(e.srv.URL + "/openapi.json")
        if err != nil {
                t.Fatal(err)
        }
        if resp.StatusCode != 200 {
                t.Fatalf("expected 200, got %d", resp.StatusCode)
        }
        var spec map[string]any
        if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
                t.Fatal(err)
        }
        if spec["openapi"] != "3.1.0" {
                t.Fatalf("expected OpenAPI 3.1.0, got %v", spec["openapi"])
        }
        paths, _ := spec["paths"].(map[string]any)
        if _, ok := paths["/api/v1/users"]; !ok {
                t.Fatal("expected /api/v1/users in paths")
        }
}

// =============================================================================
// Test: Swagger UI + ReDoc are served
// =============================================================================

func TestIntegration_DocsUI_Served(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        for _, path := range []string{"/docs/swagger", "/docs/redoc"} {
                resp, err := http.Get(e.srv.URL + path)
                if err != nil {
                        t.Fatal(err)
                }
                if resp.StatusCode != 200 {
                        t.Fatalf("%s: expected 200, got %d", path, resp.StatusCode)
                }
                body, _ := io.ReadAll(resp.Body)
                if !strings.Contains(string(body), "html") {
                        t.Fatalf("%s: expected HTML", path)
                }
        }
}

// =============================================================================
// Test: GraphQL query + mutation
// =============================================================================

func TestIntegration_GraphQL_CreateAndQuery(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        // Create via GraphQL mutation
        mutation := `{"query":"mutation { createUser(input: {email: \"gql@example.com\", firstName: \"GQL\", lastName: \"Test\"}) { id email fullName status } }"}`
        req, _ := http.NewRequest("POST", e.srv.URL+"/graphql", bytes.NewBufferString(mutation))
        req.Header.Set("Authorization", "Bearer "+e.token)
        req.Header.Set("Content-Type", "application/json")
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
                t.Fatal(err)
        }
        if resp.StatusCode != 200 {
                t.Fatalf("expected 200, got %d", resp.StatusCode)
        }
        var gqlResp map[string]any
        _ = json.NewDecoder(resp.Body).Decode(&gqlResp)
        if errs, ok := gqlResp["errors"]; ok {
                t.Fatalf("unexpected GraphQL errors: %v", errs)
        }
        data, _ := gqlResp["data"].(map[string]any)
        created, _ := data["createUser"].(map[string]any)
        if created["email"] != "gql@example.com" {
                t.Fatalf("unexpected email: %v", created["email"])
        }

        // Query via GraphQL
        query := `{"query":"{ users(limit: 10) { totalCount edges { node { id email } } } }"}`
        req, _ = http.NewRequest("POST", e.srv.URL+"/graphql", bytes.NewBufferString(query))
        req.Header.Set("Authorization", "Bearer "+e.token)
        req.Header.Set("Content-Type", "application/json")
        resp, err = http.DefaultClient.Do(req)
        if err != nil {
                t.Fatal(err)
        }
        var q2 map[string]any
        _ = json.NewDecoder(resp.Body).Decode(&q2)
        if errs, ok := q2["errors"]; ok {
                t.Fatalf("unexpected GraphQL errors: %v", errs)
        }
}

// =============================================================================
// Test: GraphQL schema (SDL) is served
// =============================================================================

func TestIntegration_GraphQL_SchemaServed(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        resp, err := http.Get(e.srv.URL + "/graphql/schema")
        if err != nil {
                t.Fatal(err)
        }
        if resp.StatusCode != 200 {
                t.Fatalf("expected 200, got %d", resp.StatusCode)
        }
        body, _ := io.ReadAll(resp.Body)
        if !strings.Contains(string(body), "type User") {
                t.Fatal("expected 'type User' in SDL")
        }
}

// =============================================================================
// Test: Idempotency-Key deduplicates mutations
// =============================================================================

func TestIntegration_Idempotency_Dedup(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        key := "idem-key-123"
        body := `{"email":"idem@example.com","first_name":"Idem","last_name":"Pot"}`

        // First call - creates the user.
        req1, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/users", bytes.NewBufferString(body))
        req1.Header.Set("Authorization", "Bearer "+e.token)
        req1.Header.Set("Content-Type", "application/json")
        req1.Header.Set("Idempotency-Key", key)
        resp1, err := http.DefaultClient.Do(req1)
        if err != nil {
                t.Fatal(err)
        }
        if resp1.StatusCode != 201 {
                t.Fatalf("first call: expected 201, got %d", resp1.StatusCode)
        }
        var first map[string]any
        _ = json.NewDecoder(resp1.Body).Decode(&first)
        resp1.Body.Close()

        // Second call with same key+body - should return the SAME response.
        req2, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/users", bytes.NewBufferString(body))
        req2.Header.Set("Authorization", "Bearer "+e.token)
        req2.Header.Set("Content-Type", "application/json")
        req2.Header.Set("Idempotency-Key", key)
        resp2, err := http.DefaultClient.Do(req2)
        if err != nil {
                t.Fatal(err)
        }
        if resp2.StatusCode != 201 {
                t.Fatalf("replay call: expected 201, got %d", resp2.StatusCode)
        }
        var second map[string]any
        _ = json.NewDecoder(resp2.Body).Decode(&second)
        resp2.Body.Close()

        firstData := first["data"].(map[string]any)
        secondData := second["data"].(map[string]any)
        if firstData["id"] != secondData["id"] {
                t.Fatalf("idempotency broken: first id=%v second id=%v", firstData["id"], secondData["id"])
        }

        // Verify only one row exists in the DB.
        var count int
        _ = e.db.Get(&count, "SELECT count(*) FROM users WHERE email = 'idem@example.com'")
        if count != 1 {
                t.Fatalf("expected 1 row, got %d (idempotency failed)", count)
        }
}

// =============================================================================
// Test: ETag + If-None-Match returns 304
// =============================================================================

func TestIntegration_ETag_304(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        // Create a user.
        body := `{"email":"etag@example.com","first_name":"ETag","last_name":"Test"}`
        req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/users", bytes.NewBufferString(body))
        req.Header.Set("Authorization", "Bearer "+e.token)
        req.Header.Set("Content-Type", "application/json")
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
                t.Fatal(err)
        }
        var created map[string]any
        _ = json.NewDecoder(resp.Body).Decode(&created)
        resp.Body.Close()
        userID := created["data"].(map[string]any)["id"].(string)

        // First GET - capture ETag.
        req1, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/users/"+userID, nil)
        req1.Header.Set("Authorization", "Bearer "+e.token)
        resp1, err := http.DefaultClient.Do(req1)
        if err != nil {
                t.Fatal(err)
        }
        etag := resp1.Header.Get("ETag")
        resp1.Body.Close()
        if etag == "" {
                t.Fatal("expected ETag header on first GET")
        }

        // Second GET with If-None-Match - should get 304.
        req2, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/users/"+userID, nil)
        req2.Header.Set("Authorization", "Bearer "+e.token)
        req2.Header.Set("If-None-Match", etag)
        resp2, err := http.DefaultClient.Do(req2)
        if err != nil {
                t.Fatal(err)
        }
        defer resp2.Body.Close()
        if resp2.StatusCode != 304 {
                t.Fatalf("expected 304, got %d", resp2.StatusCode)
        }
}

// =============================================================================
// Test: Outbox table receives events on user creation
// =============================================================================

func TestIntegration_Outbox_ReceivesEvents(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        // Create a user - this should append to the outbox inside the tx.
        body := `{"email":"outbox@example.com","first_name":"OB","last_name":"Test"}`
        req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/users", bytes.NewBufferString(body))
        req.Header.Set("Authorization", "Bearer "+e.token)
        req.Header.Set("Content-Type", "application/json")
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
                t.Fatal(err)
        }
        resp.Body.Close()

        // Verify the outbox table has a user.created row.
        var count int
        err = e.db.Get(&count, "SELECT count(*) FROM outbox WHERE topic = 'user.created'")
        if err != nil {
                t.Fatalf("query outbox: %v", err)
        }
        if count == 0 {
                t.Fatal("expected at least one outbox row for user.created; outbox wiring is broken")
        }
}

// =============================================================================
// Test: Composite health endpoints
// =============================================================================

func TestIntegration_Health_AllProbes(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        cases := []struct {
                path       string
                wantStatus int
                wantBody   string
        }{
                {"/healthz/startup", 200, "started"},
                {"/healthz/live", 200, "alive"},
                {"/healthz/ready", 200, "up"},
                {"/healthz/deep", 200, "checks"},
        }
        for _, c := range cases {
                resp, err := http.Get(e.srv.URL + c.path)
                if err != nil {
                        t.Fatal(err)
                }
                if resp.StatusCode != c.wantStatus {
                        t.Fatalf("%s: expected %d, got %d", c.path, c.wantStatus, resp.StatusCode)
                }
                body, _ := io.ReadAll(resp.Body)
                if !strings.Contains(string(body), c.wantBody) {
                        t.Fatalf("%s: expected body to contain %q, got %s", c.path, c.wantBody, string(body))
                }
                resp.Body.Close()
        }
}

// =============================================================================
// Test: Security headers are present on every response
// =============================================================================

func TestIntegration_SecurityHeaders_Present(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        resp, err := http.Get(e.srv.URL + "/healthz/live")
        if err != nil {
                t.Fatal(err)
        }
        defer resp.Body.Close()

        required := []string{
                "X-Frame-Options",
                "X-Content-Type-Options",
                "Referrer-Policy",
                "Content-Security-Policy",
                "Permissions-Policy",
                "Cache-Control",
        }
        for _, h := range required {
                if resp.Header.Get(h) == "" {
                        t.Fatalf("missing security header: %s", h)
                }
        }
        if resp.Header.Get("X-Frame-Options") != "DENY" {
                t.Fatalf("expected X-Frame-Options=DENY, got %q", resp.Header.Get("X-Frame-Options"))
        }
}

// =============================================================================
// Test: API-Version header is stamped
// =============================================================================

func TestIntegration_Versioning_Header(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        resp, err := http.Get(e.srv.URL + "/api/v1/users?limit=1")
        if err != nil {
                t.Fatal(err)
        }
        defer resp.Body.Close()
        if v := resp.Header.Get("API-Version"); v != "v1" {
                t.Fatalf("expected API-Version=v1, got %q", v)
        }
}

// =============================================================================
// Test: Body size limit returns 413
// =============================================================================

func TestIntegration_MaxBodySize_413(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        // 2 MiB body - exceeds the 1 MiB limit.
        big := strings.Repeat("x", 2*1024*1024)
        req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/users", bytes.NewBufferString(big))
        req.Header.Set("Authorization", "Bearer "+e.token)
        req.Header.Set("Content-Type", "application/json")
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
                t.Fatal(err)
        }
        defer resp.Body.Close()
        if resp.StatusCode != 413 {
                t.Fatalf("expected 413, got %d", resp.StatusCode)
        }
}

// =============================================================================
// Test: Unauthorized without token
// =============================================================================

func TestIntegration_UnauthorizedWithoutToken(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        resp, err := http.Get(e.srv.URL + "/api/v1/users")
        if err != nil {
                t.Fatal(err)
        }
        defer resp.Body.Close()
        if resp.StatusCode != 401 {
                t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
        }
}

// =============================================================================
// Test: API key auth works as an alternative to JWT
// =============================================================================

func TestIntegration_APIKeyAuth(t *testing.T) {
        e := setup(t)
        defer e.teardown()

        req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/users?limit=1", nil)
        req.Header.Set("X-API-Key", e.apiKey)
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
                t.Fatal(err)
        }
        defer resp.Body.Close()
        if resp.StatusCode != 200 {
                t.Fatalf("expected 200 with API key, got %d", resp.StatusCode)
        }
}
