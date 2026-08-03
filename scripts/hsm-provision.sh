#!/usr/bin/env bash
# Idempotently provision a SoftHSM2 token with the four DCS ECDSA P-256 keys.
# One token dir per DCS instance keeps instance A (port 8991) and instance B
# (port 8992) key material separate, mirroring how .env.dev1/.env.dev2 already
# separate per-instance secrets.
#
# No contract-signing key: the DCS holds no key that produces a contract
# signature (ADR-12/ADR-20) — the signatory's wallet does. dcs-did/dcs-vc/
# dcs-oid4vp-jar/dcs-c2pa are all the DCS attesting as itself (DID document,
# lifecycle VCs, OID4VP request objects, C2PA claims), never as a signatory.
#
# Usage: hsm-provision.sh <token-dir> <token-label> <pin> <so-pin> [module-path]
set -euo pipefail

TOKEN_DIR="$1"
TOKEN_LABEL="$2"
PIN="$3"
SO_PIN="$4"
MODULE="${5:-/usr/lib/softhsm/libsofthsm2.so}"

KEY_LABELS=(dcs-did dcs-vc dcs-oid4vp-jar dcs-c2pa dcs-ecdh)

# Install SoftHSM2 + OpenSC (pkcs11-tool) if missing.
if ! command -v softhsm2-util >/dev/null 2>&1 || ! command -v pkcs11-tool >/dev/null 2>&1; then
  echo "Installing softhsm2 + opensc..."
  sudo apt-get update -qq
  sudo apt-get install -y softhsm2 opensc
fi

mkdir -p "$TOKEN_DIR/tokens"
CONF="$TOKEN_DIR/softhsm2.conf"
cat > "$CONF" <<EOF
directories.tokendir = $TOKEN_DIR/tokens
objectstore.backend = file
log.level = INFO
EOF
export SOFTHSM2_CONF="$CONF"

# Initialize the token if it does not exist yet.
if ! softhsm2-util --show-slots 2>/dev/null | grep -q "Label:[[:space:]]*$TOKEN_LABEL"; then
  echo "Initializing token '$TOKEN_LABEL' in $TOKEN_DIR..."
  softhsm2-util --init-token --free --label "$TOKEN_LABEL" --pin "$PIN" --so-pin "$SO_PIN"
else
  echo "Token '$TOKEN_LABEL' already present in $TOKEN_DIR."
fi

existing="$(pkcs11-tool --module "$MODULE" --token-label "$TOKEN_LABEL" --login --pin "$PIN" --list-objects --type privkey 2>/dev/null || true)"

for LABEL in "${KEY_LABELS[@]}"; do
  if printf '%s' "$existing" | grep -q "label:[[:space:]]*$LABEL$"; then
    echo "Key '$LABEL' already present."
    continue
  fi
  ID="$(printf '%s' "$LABEL" | md5sum | cut -c1-8)"
  echo "Generating EC P-256 key '$LABEL'..."
  # dcs-ecdh is the key-agreement key (CEK wrapping): needs CKA_DERIVE usage.
  USAGE=()
  if [ "$LABEL" = "dcs-ecdh" ]; then
    USAGE=(--usage-derive)
  fi
  pkcs11-tool --module "$MODULE" --token-label "$TOKEN_LABEL" --login --pin "$PIN" \
    --keypairgen --key-type EC:prime256v1 --label "$LABEL" --id "$ID" ${USAGE[@]+"${USAGE[@]}"}
done

echo "SoftHSM token '$TOKEN_LABEL' ready ($TOKEN_DIR)."
echo "SOFTHSM2_CONF=$CONF"
