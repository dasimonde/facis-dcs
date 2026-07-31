#!/usr/bin/env bash
set -ex
cd /tmp/local-tsa
HASH=$(echo -n test | sha256sum | cut -d' ' -f1)
echo "hash=$HASH"
openssl ts -query -digest "$HASH" -sha256 -cert -out request.tsq
openssl ts -reply -queryfile request.tsq -signer tsa-cert.pem -inkey tsa-key.pem -chain tsa-ca-cert.pem -out response.tsr
echo "--- reply generated ---"
openssl ts -reply -in response.tsr -text
