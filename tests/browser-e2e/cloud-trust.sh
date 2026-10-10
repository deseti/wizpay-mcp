#!/usr/bin/env bash
# OWNER-RUN ONLY in the existing cloud workspace. No installation or cloud calls.
set -euo pipefail
test "${WP3D_CLOUD_TRUST_APPROVED:-}" = yes || { echo 'Explicit owner trust setup approval required'; exit 1; }
test "$(pwd)" = /home/agent/workspace/wizpay-mcp || { echo 'Wrong cloud workspace'; exit 1; }
command -v openssl >/dev/null
command -v certutil >/dev/null || { echo 'libnss3-tools unavailable; owner installation approval required'; exit 1; }
umask 077
tls=deploy/local/runtime/browser-tls
browser_home=deploy/local/runtime/browser-home
test ! -e "$tls" && test ! -e "$browser_home" || { echo 'Existing trust material preserved; stop and review'; exit 1; }
mkdir -p "$tls" "$browser_home/.pki/nssdb"
openssl req -x509 -newkey rsa:3072 -sha256 -nodes -days 2 -keyout "$tls/ca.key" -out "$tls/ca.crt" -subj '/CN=WizPay isolated browser test CA' -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' -addext 'keyUsage=critical,keyCertSign,cRLSign' >/dev/null 2>&1
openssl req -new -newkey rsa:2048 -nodes -keyout "$tls/tls.key" -out "$tls/tls.csr" -subj '/CN=connect.wizpay.xyz' >/dev/null 2>&1
cat > "$tls/extensions" <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:connect.wizpay.xyz,DNS:mcp.wizpay.xyz
EOF
openssl x509 -req -in "$tls/tls.csr" -CA "$tls/ca.crt" -CAkey "$tls/ca.key" -CAcreateserial -out "$tls/tls.crt" -days 2 -sha256 -extfile "$tls/extensions" >/dev/null 2>&1
certutil -N --empty-password -d "sql:$browser_home/.pki/nssdb"
certutil -A -d "sql:$browser_home/.pki/nssdb" -n wp3d-isolated-ca -t 'C,,' -i "$tls/ca.crt"
openssl verify -CAfile "$tls/ca.crt" -verify_hostname connect.wizpay.xyz "$tls/tls.crt"
openssl verify -CAfile "$tls/ca.crt" -verify_hostname mcp.wizpay.xyz "$tls/tls.crt"
echo 'Trust prepared only under ignored isolated HOME. Launch Chrome with that HOME; never import this CA globally.'
