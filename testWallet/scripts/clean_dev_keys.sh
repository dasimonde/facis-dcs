#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "$ROOT/.." && pwd)"

rm -f "$ROOT"/keys/*.jwk
rm -f "$ROOT"/credentials/*.jwt
rm -f "$ROOT"/trust.dev.json
rm -f "$REPO_ROOT"/backend/config/oid4vp/trust.dev.json

echo "Cleaned generated keys, JWT credentials, and trust.dev.json files."
echo "Templates were kept: $ROOT/credentials/*.template.json"
