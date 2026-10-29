#!/usr/bin/env bash
# Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
# Author: Awase Khirni Syed
#
# sbom.sh - generates a CycloneDX Software Bill of Materials for the Go
# module. Required by EO 14028 and any SOC2 audit that touches supply chain.
#
# Prereqs:
#   - cyclonedx-gomod (https://github.com/CycloneDX/cyclonedx-gomod)
#
# Usage:
#   ./scripts/sbom/sbom.sh > sbom.json
#
# GAP CLOSED: SBOM generation (supply-chain security, EO 14028)

set -euo pipefail

if ! command -v cyclonedx-gomod >/dev/null 2>&1; then
  echo "ERROR: cyclonedx-gomod not installed." >&2
  echo "  go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest" >&2
  exit 1
fi

# Always run from the module root.
cd "$(dirname "$0")/../.."

echo "Generating SBOM..." >&2
cyclonedx-gomod app -json -licenses -main cmd/server -output -
