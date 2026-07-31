#!/usr/bin/env bash
# Idempotently maintain a CRL for the dev C2PA/PAdES signing CA (the same CA
# c2pa-cert-provision.sh issues the signing leaf from) and publish it at a
# stable path that the leaf's crlDistributionPoints extension points at.
#
# Without a revoke target it (re)generates an empty, current CRL so verifiers
# always find a fresh, valid list. With a revoke target (a leaf certificate
# PEM) it revokes that certificate's serial and regenerates the CRL — this is
# the ops action AC11 (DCS-OR-C2PA-007) exercises: revoking the dev signing
# certificate flips a previously valid signature to certificate-revoked.
#
# Usage: crl-provision.sh <ca-dir> <crl-out> [leaf-to-revoke.pem]
#   <ca-dir>   directory holding c2pa-ca.key + c2pa-ca.crt (as written by
#              c2pa-cert-provision.sh)
#   <crl-out>  path to write the PEM CRL to
set -euo pipefail

CA_DIR="$1"
CRL_OUT="$2"
REVOKE_LEAF="${3:-}"

CA_KEY="$CA_DIR/c2pa-ca.key"
CA_CRT="$CA_DIR/c2pa-ca.crt"
CRL_DAYS="30"

if [ ! -f "$CA_KEY" ] || [ ! -f "$CA_CRT" ]; then
  echo "error: dev CA ($CA_KEY / $CA_CRT) not found — run c2pa-cert-provision.sh first" >&2
  exit 1
fi

# openssl ca needs a small database layout; keep it beside the CA so the CRL
# number and revocation index survive across runs.
DB_DIR="$CA_DIR/crl-db"
mkdir -p "$DB_DIR/newcerts"
[ -f "$DB_DIR/index.txt" ] || : > "$DB_DIR/index.txt"
[ -f "$DB_DIR/crlnumber" ] || echo "1000" > "$DB_DIR/crlnumber"

CONF="$DB_DIR/openssl-ca.cnf"
cat > "$CONF" <<EOF
[ ca ]
default_ca = dcs_ca

[ dcs_ca ]
dir               = $DB_DIR
database          = \$dir/index.txt
new_certs_dir     = \$dir/newcerts
certificate       = $CA_CRT
private_key       = $CA_KEY
serial            = \$dir/serial
crlnumber         = \$dir/crlnumber
default_md        = sha256
default_crl_days  = $CRL_DAYS
policy            = dcs_policy

[ dcs_policy ]
commonName = supplied
EOF
[ -f "$DB_DIR/serial" ] || echo "01" > "$DB_DIR/serial"

if [ -n "$REVOKE_LEAF" ]; then
  if [ ! -f "$REVOKE_LEAF" ]; then
    echo "error: leaf-to-revoke '$REVOKE_LEAF' not found" >&2
    exit 1
  fi
  echo "Revoking $REVOKE_LEAF in the dev CRL..."
  # openssl ca -revoke needs the cert registered in index.txt; -valid records
  # it there first when unknown, so re-revoking is idempotent.
  openssl ca -config "$CONF" -valid "$REVOKE_LEAF" 2>/dev/null || true
  openssl ca -config "$CONF" -revoke "$REVOKE_LEAF" 2>/dev/null || true
fi

openssl ca -config "$CONF" -gencrl -out "$CRL_OUT"
echo "Dev CRL written to $CRL_OUT (valid ${CRL_DAYS}d)."
