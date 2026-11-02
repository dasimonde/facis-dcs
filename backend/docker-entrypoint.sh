#!/bin/bash
set -e

# If custom CA certificates exist, update the CA bundle
if [ -d "/usr/local/share/ca-certificates/custom" ] && [ "$(ls -A /usr/local/share/ca-certificates/custom 2>/dev/null)" ]; then
    if [ -f /usr/local/share/ca-certificates/custom/*.crt ]; then
        cat /etc/ssl/certs/ca-certificates.crt /usr/local/share/ca-certificates/custom/*.crt > /tmp/ca-bundle.crt
        export SSL_CERT_FILE=/tmp/ca-bundle.crt
    fi
fi

exec "$@"
