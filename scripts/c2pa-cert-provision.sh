#!/usr/bin/env bash
# Idempotently issue a dev-CA leaf certificate whose public key is the SoftHSM
# token's dcs-c2pa key, and write the leaf+CA chain as the C2PA COSE x5chain
# (RFC 9360) that pdf-core embeds. pdf-core signs COSE Sig_structure bytes via
# the backend's PKCS#11 dcs-c2pa key (DCS-IR-HI-01); the x5chain must therefore
# carry that key's public half so a verifier can check the ES256 signature.
#
# The dev CA (out-dir/c2pa-ca.key + c2pa-ca.crt) is generated once and reused so
# it stays a stable trust anchor across runs; the leaf is re-issued every run so
# it always matches the current token key (surviving key rotation).
#
# Usage: c2pa-cert-provision.sh <token-dir> <token-label> <pin> <x5chain-out> [module-path]
set -euo pipefail

TOKEN_DIR="$1"
TOKEN_LABEL="$2"
PIN="$3"
X5CHAIN_OUT="$4"
MODULE="${5:-/usr/lib/softhsm/libsofthsm2.so}"
CRL_URL="${6:-http://localhost:8991/crl/dcs-c2pa.crl}"

KEY_LABEL="${KEY_LABEL:-dcs-c2pa}"
CA_CN="DCS Dev C2PA CA"
LEAF_CN="DCS Dev ${KEY_LABEL} Signer"
# c2pa-rs reads the signing certificate's organizationName once the COSE
# signature has verified, and fails the whole validation when it is absent, so a
# CN-only leaf yields manifests every C2PA verifier reports as
# claimSignature.mismatch. Both subjects therefore carry an O=.
CERT_ORG="${CERT_ORG:-FACIS DCS}"
VALIDITY_DAYS="825"

export SOFTHSM2_CONF="$TOKEN_DIR/softhsm2.conf"

OUT_DIR="$(dirname "$X5CHAIN_OUT")"
mkdir -p "$OUT_DIR"
CA_KEY="$OUT_DIR/c2pa-ca.key"
CA_CRT="$OUT_DIR/c2pa-ca.crt"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

# Export the dcs-c2pa public key (SubjectPublicKeyInfo DER) from the token.
pkcs11-tool --module "$MODULE" --token-label "$TOKEN_LABEL" --login --pin "$PIN" \
  --read-object --type pubkey --label "$KEY_LABEL" --output-file "$workdir/leaf.pub.der"
openssl pkey -pubin -inform DER -in "$workdir/leaf.pub.der" -out "$workdir/leaf.pub.pem"

# Generate the dev CA once; reuse it on subsequent runs.
if [ ! -f "$CA_KEY" ] || [ ! -f "$CA_CRT" ]; then
  echo "Generating dev C2PA CA in $OUT_DIR..."
  openssl ecparam -name prime256v1 -genkey -noout -out "$CA_KEY"
  openssl req -x509 -new -sha256 -key "$CA_KEY" -days "$VALIDITY_DAYS" \
    -subj "/O=$CERT_ORG/CN=$CA_CN" -out "$CA_CRT"
else
  echo "Reusing existing dev C2PA CA in $OUT_DIR."
fi

# Issue a leaf whose public key is forced to the token's dcs-c2pa public key.
# The throwaway CSR key only carries the subject; -force_pubkey overrides it.
openssl ecparam -name prime256v1 -genkey -noout -out "$workdir/tmp-leaf.key"
openssl req -new -key "$workdir/tmp-leaf.key" -subj "/O=$CERT_ORG/CN=$LEAF_CN" -out "$workdir/leaf.csr"

cat > "$workdir/leaf.ext" <<EOF
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=emailProtection
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid,issuer
crlDistributionPoints=URI:$CRL_URL
EOF

openssl x509 -req \
  -in "$workdir/leaf.csr" \
  -CA "$CA_CRT" \
  -CAkey "$CA_KEY" \
  -CAcreateserial \
  -days "$VALIDITY_DAYS" \
  -sha256 \
  -force_pubkey "$workdir/leaf.pub.pem" \
  -extfile "$workdir/leaf.ext" \
  -out "$workdir/leaf.crt"

cat "$workdir/leaf.crt" "$CA_CRT" > "$X5CHAIN_OUT"
echo "C2PA x5chain written to $X5CHAIN_OUT (leaf pubkey = token key '$KEY_LABEL')."
