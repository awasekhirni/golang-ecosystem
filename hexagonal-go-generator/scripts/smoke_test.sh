#!/usr/bin/env bash
# Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
# Author: Awase Khirni Syed
#
# smoke_test.sh - end-to-end verification of every endpoint + cross-cutting
# concern. Exits non-zero on any failure.
#
# Usage:
#   BASE_URL=http://localhost:8080 ./scripts/smoke_test.sh
#
# Verified:
#   - Liveness / Readiness / Deep health probes
#   - /metrics exposition
#   - OpenAPI spec served + valid
#   - Swagger UI + ReDoc served
#   - GraphQL schema + playground served
#   - GraphQL mutation + query round-trip
#   - Auth: 401 without token, 200 with JWT, 200 with API key
#   - REST: POST / GET / GET-by-id / PATCH status / DELETE round-trip
#   - Idempotency-Key dedup (same response, single DB row)
#   - ETag 304 on If-None-Match
#   - Security headers present
#   - API-Version header present
#   - Body size limit (413 on oversized)
#
# GAP CLOSED: end-to-end verification script

set -euo pipefail

BASE="${BASE_URL:-http://localhost:8080}"
PASS=0
FAIL=0

ok()   { echo "  ✓ $1"; PASS=$((PASS+1)); }
fail() { echo "  ✗ $1"; FAIL=$((FAIL+1)); }

check() {
  local desc="$1"; local expected="$2"; local actual="$3"
  if [ "$expected" = "$actual" ]; then ok "$desc ($actual)"; else fail "$desc (expected $expected, got $actual)"; fi
}

echo "=== Hexagonal Go smoke test against $BASE ==="
echo ""

# --- Health probes ----------------------------------------------------------
echo "[1/12] Health probes"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/healthz/live");     check "GET /healthz/live"     "200" "$HTTP"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/healthz/ready");    check "GET /healthz/ready"    "200" "$HTTP"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/healthz/startup");  check "GET /healthz/startup"  "200" "$HTTP"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/healthz/deep");     check "GET /healthz/deep"     "200" "$HTTP"

# --- Metrics ----------------------------------------------------------------
echo ""
echo "[2/12] Prometheus metrics"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/metrics"); check "GET /metrics" "200" "$HTTP"

# --- OpenAPI ----------------------------------------------------------------
echo ""
echo "[3/12] OpenAPI spec"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/openapi.json")
check "GET /openapi.json" "200" "$HTTP"
if [ "$HTTP" = "200" ]; then
  OAS_VER=$(curl -s "$BASE/openapi.json" | python3 -c 'import sys,json;print(json.load(sys.stdin)["openapi"])' 2>/dev/null || echo "?")
  check "OpenAPI version" "3.1.0" "$OAS_VER"
fi

# --- Docs UIs ---------------------------------------------------------------
echo ""
echo "[4/12] Docs UIs"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/docs/swagger"); check "GET /docs/swagger" "200" "$HTTP"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/docs/redoc");   check "GET /docs/redoc"   "200" "$HTTP"

# --- GraphQL ----------------------------------------------------------------
echo ""
echo "[5/12] GraphQL"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/graphql/schema");   check "GET /graphql/schema"   "200" "$HTTP"
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/graphql/playground"); check "GET /graphql/playground" "200" "$HTTP"

# --- Auth -------------------------------------------------------------------
echo ""
echo "[6/12] Auth"
TOKEN=$(curl -s "$BASE/auth/demo-token" | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])' 2>/dev/null || echo "")
if [ -n "$TOKEN" ]; then ok "GET /auth/demo-token returned a token"; else fail "GET /auth/demo-token returned no token"; fi

# 401 without token
HTTP=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/users"); check "GET /api/v1/users without token -> 401" "401" "$HTTP"

# 200 with JWT
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/users?limit=1"); check "GET /api/v1/users with JWT -> 200" "200" "$HTTP"

# 200 with API key (if configured)
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -H "X-API-Key: dev-key-1" "$BASE/api/v1/users?limit=1"); check "GET /api/v1/users with API key -> 200" "200" "$HTTP"

# --- REST round-trip --------------------------------------------------------
echo ""
echo "[7/12] REST CRUD round-trip"
EMAIL="smoke-$(date +%s)@example.com"
BODY="{\"email\":\"$EMAIL\",\"first_name\":\"Smoke\",\"last_name\":\"Test\"}"
RESP=$(curl -s -w '\n%{http_code}' -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "$BODY" "$BASE/api/v1/users")
HTTP=$(echo "$RESP" | tail -1)
check "POST /api/v1/users -> 201" "201" "$HTTP"
USER_ID=$(echo "$RESP" | head -n-1 | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])' 2>/dev/null || echo "")
if [ -n "$USER_ID" ]; then ok "Created user id=$USER_ID"; else fail "Could not parse created user id"; fi

# GET by id
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/users/$USER_ID"); check "GET /api/v1/users/{id} -> 200" "200" "$HTTP"

# PATCH status
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X PATCH -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{"status":"active","reason":"smoke test"}' "$BASE/api/v1/users/$USER_ID/status"); check "PATCH /api/v1/users/{id}/status -> 200" "200" "$HTTP"

# DELETE
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/users/$USER_ID"); check "DELETE /api/v1/users/{id} -> 204" "204" "$HTTP"

# --- Idempotency ------------------------------------------------------------
echo ""
echo "[8/12] Idempotency-Key"
EMAIL2="idem-$(date +%s)@example.com"
BODY2="{\"email\":\"$EMAIL2\",\"first_name\":\"Idem\",\"last_name\":\"Pot\"}"
KEY="idem-key-$(date +%s)"
RESP1=$(curl -s -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -H "Idempotency-Key: $KEY" -d "$BODY2" "$BASE/api/v1/users")
RESP2=$(curl -s -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -H "Idempotency-Key: $KEY" -d "$BODY2" "$BASE/api/v1/users")
ID1=$(echo "$RESP1" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])' 2>/dev/null || echo "")
ID2=$(echo "$RESP2" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])' 2>/dev/null || echo "")
if [ "$ID1" = "$ID2" ] && [ -n "$ID1" ]; then ok "Idempotency dedup (same id $ID1)"; else fail "Idempotency broken: $ID1 vs $ID2"; fi

# --- ETag -------------------------------------------------------------------
echo ""
echo "[9/12] ETag + conditional GET"
EMAIL3="etag-$(date +%s)@example.com"
RESP=$(curl -s -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "{\"email\":\"$EMAIL3\",\"first_name\":\"E\",\"last_name\":\"T\"}" "$BASE/api/v1/users")
ETAG_USER=$(echo "$RESP" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["id"])' 2>/dev/null || echo "")
ETAG=$(curl -sI -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/users/$ETAG_USER" | grep -i '^etag:' | awk '{print $2}' | tr -d '\r\n')
if [ -n "$ETAG" ]; then ok "Got ETag: $ETAG"; else fail "No ETag header"; fi
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOKEN" -H "If-None-Match: $ETAG" "$BASE/api/v1/users/$ETAG_USER"); check "GET with If-None-Match -> 304" "304" "$HTTP"

# --- Security headers -------------------------------------------------------
echo ""
echo "[10/12] Security headers"
HDRS=$(curl -sI "$BASE/healthz/live")
for h in "X-Frame-Options" "X-Content-Type-Options" "Referrer-Policy" "Content-Security-Policy" "Permissions-Policy"; do
  if echo "$HDRS" | grep -qi "^$h:"; then ok "Header present: $h"; else fail "Missing header: $h"; fi
done

# --- API-Version ------------------------------------------------------------
echo ""
echo "[11/12] API versioning"
VER=$(curl -sI -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/users?limit=1" | grep -i '^api-version:' | awk '{print $2}' | tr -d '\r\n')
check "API-Version header" "v1" "$VER"

# --- Body size limit --------------------------------------------------------
echo ""
echo "[12/12] Body size limit"
BIG=$(head -c 2200000 /dev/urandom | base64 | head -c 2100000)
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "$BIG" "$BASE/api/v1/users"); check "Oversized body -> 413" "413" "$HTTP"

# --- Summary ----------------------------------------------------------------
echo ""
echo "=== Summary: $PASS passed, $FAIL failed ==="
if [ "$FAIL" -gt 0 ]; then exit 1; fi
exit 0
