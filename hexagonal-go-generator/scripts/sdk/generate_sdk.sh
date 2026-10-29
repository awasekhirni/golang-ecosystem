#!/usr/bin/env bash
# Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
# Author: Awase Khirni Syed
#
# generate_sdk.sh - generates a typed Go client SDK from the live OpenAPI spec.
#
# Prereqs:
#   - npx + @openapitools/openapi-generator-cli (auto-installed on first run)
#   - the hexagonal-go server running (so we can fetch /openapi.json)
#
# Usage:
#   BASE_URL=http://localhost:8080 ./scripts/sdk/generate_sdk.sh
#
# Output:
#   generated/client/  - a ready-to-import Go module with typed UserClient etc.
#
# GAP CLOSED: API client SDK generator (from OpenAPI spec)

set -euo pipefail

BASE="${BASE_URL:-http://localhost:8080}"
OUT="${OUT:-./generated/client}"
LANG="${LANG:-go}"

echo "==> Fetching OpenAPI spec from $BASE/openapi.json"
SPEC_FILE=$(mktemp /tmp/hexgo-openapi-XXXXXX.json)
curl -sS "$BASE/openapi.json" -o "$SPEC_FILE"

if [ ! -s "$SPEC_FILE" ]; then
  echo "ERROR: fetched spec is empty. Is the server running at $BASE?" >&2
  exit 1
fi

echo "==> Generating $LANG SDK into $OUT"
rm -rf "$OUT"
mkdir -p "$OUT"

npx -y @openapitools/openapi-generator-cli generate \
  -i "$SPEC_FILE" \
  -g "$LANG" \
  -o "$OUT" \
  --additional-properties=packageName=hexgoclient,useOneOfDiscriminatorLookup=true

echo "==> SDK generated at $OUT"
echo "    Import path: github.com/ori-inc/hexagonal-go/generated/client"
echo "    Example:"
echo '      import "github.com/ori-inc/hexagonal-go/generated/client"'
echo '      cfg := hexgoclient.NewConfiguration()' 
echo '      cfg.BasePath = "http://localhost:8080"' 
echo '      api := hexgoclient.NewAPIClient(cfg)' 
echo '      users, _, err := api.UsersApi.ListUsers(ctx).Execute()'

rm -f "$SPEC_FILE"
