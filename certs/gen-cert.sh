#!/usr/bin/env bash
# Generate a self-signed EC certificate for the lab. LAB ONLY.
#
# An EC key (secp256r1 / prime256v1) is deliberate: the handshake signature is
# cheaper than RSA, which matters once you open thousands of connections/sec.
#
# The SAN matters: without a subjectAltName, modern clients reject the cert
# outright and you spend the hour debugging TLS instead of measuring it.
# We include the container hostnames (service, edge) so intra-rig TLS is valid
# too, not just localhost.
set -euo pipefail
cd "$(dirname "$0")"

openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout key.pem -out cert.pem -days 365 -nodes \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,DNS:service,DNS:edge,IP:127.0.0.1"

chmod 644 cert.pem key.pem
echo "wrote certs/cert.pem and certs/key.pem"
openssl x509 -in cert.pem -noout -subject -ext subjectAltName
