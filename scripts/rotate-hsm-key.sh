#!/usr/bin/env bash
# Rotate a versioned HSM signing key: generate the next-version EC P-256
# keypair in the SoftHSM token, optionally issue a dev-CA leaf certificate for
# it, and advance the pki_active_key_version pointer so new signing operations
# pick up the new version while historical signatures stay attributable to (and
# verifiable against) the versions that produced them (DCS-OR-C2PA-007).
#
# Rotation is an ops action, not an HTTP endpoint: it provisions token key
# material the running process cannot mint itself.
#
# Usage: rotate-hsm-key.sh <token-dir> <token-label> <pin> <base-label> [module-path] [ca-dir] [x5chain-out]
#   <base-label>  e.g. dcs-c2pa; version N key label is
#                 "<base-label>-v<N>" (v1 is the un-suffixed base label).
# Requires DATABASE_URL in the environment (advances pki_active_key_version).
set -euo pipefail

TOKEN_DIR="$1"
TOKEN_LABEL="$2"
PIN="$3"
BASE_LABEL="$4"
MODULE="${5:-/usr/lib/softhsm/libsofthsm2.so}"
CA_DIR="${6:-}"
X5CHAIN_OUT="${7:-}"

if [ -z "${DATABASE_URL:-}" ]; then
  echo "error: DATABASE_URL must be set (advances pki_active_key_version)" >&2
  exit 1
fi

export SOFTHSM2_CONF="$TOKEN_DIR/softhsm2.conf"

# Current active version (default 1 when the label has no row yet).
CURRENT="$(psql "$DATABASE_URL" -tAc \
  "SELECT active_version FROM pki_active_key_version WHERE label = '$BASE_LABEL'" 2>/dev/null || true)"
CURRENT="${CURRENT:-1}"
NEXT=$((CURRENT + 1))
NEW_LABEL="${BASE_LABEL}-v${NEXT}"

echo "Rotating '$BASE_LABEL': v${CURRENT} -> v${NEXT} (new key label '$NEW_LABEL')..."

# Generate the new-version keypair unless it already exists (idempotent re-run).
existing="$(pkcs11-tool --module "$MODULE" --token-label "$TOKEN_LABEL" --login --pin "$PIN" \
  --list-objects --type privkey 2>/dev/null || true)"
if printf '%s' "$existing" | grep -q "label:[[:space:]]*$NEW_LABEL$"; then
  echo "Key '$NEW_LABEL' already present."
else
  ID="$(printf '%s' "$NEW_LABEL" | md5sum | cut -c1-8)"
  # dcs-ecdh versions are key-agreement keys (CEK wrapping): need CKA_DERIVE usage.
  USAGE=()
  if [ "$BASE_LABEL" = "dcs-ecdh" ]; then
    USAGE=(--usage-derive)
  fi
  pkcs11-tool --module "$MODULE" --token-label "$TOKEN_LABEL" --login --pin "$PIN" \
    --keypairgen --key-type EC:prime256v1 --label "$NEW_LABEL" --id "$ID" ${USAGE[@]+"${USAGE[@]}"}
fi

# Optionally bind the new key to a dev-CA leaf so verifiers can check signatures
# made with the new version.
if [ -n "$CA_DIR" ] && [ -n "$X5CHAIN_OUT" ]; then
  CA_KEY="$CA_DIR/c2pa-ca.key"
  CA_CRT="$CA_DIR/c2pa-ca.crt"
  if [ -f "$CA_KEY" ] && [ -f "$CA_CRT" ]; then
    workdir="$(mktemp -d)"
    trap 'rm -rf "$workdir"' EXIT
    pkcs11-tool --module "$MODULE" --token-label "$TOKEN_LABEL" --login --pin "$PIN" \
      --read-object --type pubkey --label "$NEW_LABEL" --output-file "$workdir/leaf.pub.der"
    openssl pkey -pubin -inform DER -in "$workdir/leaf.pub.der" -out "$workdir/leaf.pub.pem"
    openssl ecparam -name prime256v1 -genkey -noout -out "$workdir/tmp-leaf.key"
    # The O= is load-bearing, not cosmetic: c2pa-rs fails validation outright when
    # the signing certificate carries no organizationName. See c2pa-cert-provision.sh.
    openssl req -new -key "$workdir/tmp-leaf.key" \
      -subj "/O=${CERT_ORG:-FACIS DCS}/CN=DCS Dev Signer $NEW_LABEL" -out "$workdir/leaf.csr"
    cat > "$workdir/leaf.ext" <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=emailProtection
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid,issuer
EOF
    openssl x509 -req -in "$workdir/leaf.csr" -CA "$CA_CRT" -CAkey "$CA_KEY" -CAcreateserial \
      -days 825 -sha256 -force_pubkey "$workdir/leaf.pub.pem" -extfile "$workdir/leaf.ext" \
      -out "$workdir/leaf.crt"
    cat "$workdir/leaf.crt" "$CA_CRT" > "$X5CHAIN_OUT"
    echo "New-version x5chain written to $X5CHAIN_OUT."
  fi
fi

# Advance the active-version pointer (create the row on first rotation).
psql "$DATABASE_URL" -c \
  "INSERT INTO pki_active_key_version (label, active_version, updated_at)
   VALUES ('$BASE_LABEL', $NEXT, now())
   ON CONFLICT (label) DO UPDATE SET active_version = EXCLUDED.active_version, updated_at = now()"

echo "Rotation complete: '$BASE_LABEL' active version is now v${NEXT}."
