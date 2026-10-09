# WP3D-1 — private Docker HTTPS integration

LOCAL ONLY. This supersedes the host-process topology. No DNS, hosts file, global
firewall, certificate trust store or production activation is changed. Root Compose
and backend authentication remain unchanged. Start no financial worker.

## Current topology

The dedicated wizpay-mcp-wp3d-local Compose project has exactly four services on
an internal private network shared by all four services. Nginx additionally joins
a regular project-scoped edge bridge; no other service joins edge. Only nginx
publishes 127.0.0.1:8443:8443. Nginx
listens on its container interface; Go and Next.js have no host ports and bind on
container interfaces. Go uses existing empty SERVER_HOST behavior; explicit
loopback validation in Go is unchanged. PostgreSQL has no published port and
uses disposable tmpfs data. Edge is not internal and may permit outbound access from Nginx. It is not a
fully egress-isolated topology; compromise of the edge could also reach private
upstreams. Host publication remains loopback-only. No external network membership,
host networking, privileged
containers or Docker socket mounts are used. Do not attach unrelated containers.

Nginx upstreams are go:8080 and frontend:3000, not container localhost and not
host.docker.internal. No Docker Desktop host-networking assumption is required.
Connect routes /onboarding and /_next/* to frontend, /browser/* and /oauth/* and
AS metadata to Go. MCP routes /mcp and protected-resource metadata to Go. Other
paths including financial approval pages and public health endpoints return 404.

The edge requires canonical Host, rejects common forwarding-spoof headers, and
uses an upstream header allowlist that drops ALL incoming X-Forwarded-* and
Forwarded. Origin, CSRF, Cookie, Authorization and MCP headers are preserved.
Set-Cookie passes unchanged. No wildcard CORS, redirect/cookie rewriting, request
logging or cache is enabled. HTTP/2 and arbitrary future Next.js RSC/server-action
compatibility remain unverified; the allowlist targets this onboarding page.
MCP idle upstream timeouts are 300 seconds, not an unlimited stream guarantee.

## Image requirements and approval boundary

Runtime: nginx:stable-alpine (manual acquisition approved) and postgres:16-alpine
(owner confirmed cached). Go artifact image base: golang:1.26.3-bookworm.
Frontend artifact image base: node:22.22.2-bookworm-slim. The last two require
separate owner approval if missing. Dockerfiles install no modules/packages and
copy only already-built local Linux artifacts. Dockerfile-specific ignore files
exclude unrelated repository files, .env material and TLS keys from build contexts.
Do not change manifests. Do not run builds until ALL required bases are cached.

Codex cannot inspect the Docker daemon because socket access is denied. No image
cache or digest was verified independently, so no invented digest is pinned.
These exact tags remain mutable; after owner acquisition record/inspect RepoDigests
and freeze a reviewed digest before relying on reproducibility. No build/pull was
executed. Runtime pull_policy is never; owner startup also uses --no-build.

## Owner-run preparation (repository root)

First inspect caches without downloading:

```sh
for image in nginx:stable-alpine postgres:16-alpine golang:1.26.3-bookworm node:22.22.2-bookworm-slim; do
  docker image inspect "$image" --format '{{json .RepoDigests}}' || break
done
```

If any image is missing, STOP. Nginx acquisition is approved for the owner; no
other image download is authorized. Inspect existing project resources before
starting so another local test session is not overwritten. Use no Docker prune.

Disposable certificate setup with installed OpenSSL:

```sh
mkdir -p deploy/local/runtime
chmod 700 deploy/local/runtime
openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 2 \
  -keyout deploy/local/runtime/tls.key -out deploy/local/runtime/tls.crt \
  -subj '/CN=connect.wizpay.xyz' \
  -addext 'subjectAltName=DNS:connect.wizpay.xyz,DNS:mcp.wizpay.xyz' \
  -addext 'basicConstraints=critical,CA:FALSE'
chmod 600 deploy/local/runtime/tls.key
```

Set the Nginx container's nonroot identity to the certificate file owner:

```sh
export WP3D_LOCAL_UID="$(id -u)" WP3D_LOCAL_GID="$(id -g)"
```

If id -u is zero, STOP and use a nonroot local owner. Do not broaden TLS key
permissions. Nginx runs without capabilities under that UID/GID, so it can read
the owner's mode-600 key. These ignored files are bound read-only. Nginx PID and body/proxy temporary files
are in container /tmp, not the certificate directory. No production certificate
is requested. curl uses explicit --cacert, never -k. Browser trust must be separately
approved and removed afterward. Do not capture credential-bearing traffic.

Build artifacts with existing approved dependencies only:

```sh
CGO_ENABLED=1 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go build -o deploy/local/runtime/wizpay-mcp-server ./cmd/server
(cd web/approval-ui && NEXT_TELEMETRY_DISABLED=1 npm run build)
```

STOP on offline module lookup failures; do not install anything. Build the Go
binary for Docker's architecture (normally linux/amd64 on HENA; verify rather than
assume on other machines). The owner validated this offline CGO-enabled build for
linux/amd64, with a maximum required glibc symbol version of 2.34 and successful
Debian Bookworm compatibility. The executable therefore requires compatible glibc
and any linked shared libraries; it must not be treated as a portable static binary
or moved to Alpine/musl or scratch without separate validation. The existing
golang:1.26.3-bookworm runtime base preserves the validated Debian environment.
GOTOOLCHAIN=local prevents automatic toolchain downloads; GOPROXY=off and GOSUMDB=off
keep this build offline using previously approved local dependencies. These flags
do not replace dependency provenance verification. No rebuild is required by this
documentation correction. Frontend must use compatible Linux Node native artifacts;
Windows node_modules are not supported. Native-module/container compatibility needs
runtime validation. Frontend is nonroot and read-only; if a runtime write is required,
review a narrow tmpfs mount rather than broad writable source/dependency mounts.

Privately supply WP3D_TEST_DB_PASSWORD and WP3D_TEST_DATABASE_URL for this disposable
DB, with URI host postgres, port 5432 and database wizpay_mcp_wp3d_test. URI-encode
the password. Never use a production URL or commit values. Both Go database URLs
use this exact isolated test DB; using its disposable owner for LOCAL TEST ONLY
does not solve WP3D-3 least privilege. Existing startup runs migrations; it does
not seed/activate users, tenants, clients or wallets. SIWE and autonomy stay off.

After all bases and local artifacts exist:

```sh
docker compose -f deploy/local/compose.yaml config --quiet
# BuildKit offline execution: no RUN/network steps, no package installation.
DOCKER_BUILDKIT=1 docker compose -f deploy/local/compose.yaml build --pull=false go frontend
docker compose -f deploy/local/compose.yaml up -d --pull never --no-build postgres go frontend
# With service names resolvable on the same project network:
docker compose -f deploy/local/compose.yaml run --rm --no-deps --pull never nginx -t -p /work/ -c nginx.conf
# Start only after the syntax check succeeds:
docker compose -f deploy/local/compose.yaml up -d --pull never --no-build nginx
```

Both build definitions set network: none. Stop if the installed Compose version
rejects this field; do not silently permit downloads. Database health gates Go
startup; Go/frontend readiness must also be verified before interpreting proxy failures.
No service is assumed healthy merely because its container is running.

## Terminal HTTPS checks

```sh
curl --noproxy '*' --cacert deploy/local/runtime/tls.crt \
 --connect-to connect.wizpay.xyz:443:127.0.0.1:8443 \
 https://connect.wizpay.xyz/.well-known/oauth-authorization-server
curl --noproxy '*' --cacert deploy/local/runtime/tls.crt \
 --connect-to mcp.wizpay.xyz:443:127.0.0.1:8443 \
 https://mcp.wizpay.xyz/.well-known/oauth-protected-resource/mcp
curl --noproxy '*' --cacert deploy/local/runtime/tls.crt \
 --connect-to mcp.wizpay.xyz:443:127.0.0.1:8443 \
 -D - https://mcp.wizpay.xyz/mcp
```

Expect metadata 200 with exact canonical identifiers, unauthenticated MCP 401
with resource_metadata challenge, and /onboarding HTML with working /_next assets.
Wrong Host returns 421; spoofed common forwarding headers return 400; unknown routes
return 404. Wrong browser Origin is rejected. No-cookie session returns 401.

curl --connect-to preserves URL port 443, Host and TLS SNI while connecting to
loopback 8443. Browsing canonical-host:8443 remains invalid. Browser integration
requires separately approved temporary canonical hostname resolution and loopback
port-443 publication/mapping. Do not edit hosts or firewall automatically.

Secure __Host-wizpay-browser cookie properties are unchanged: HttpOnly, Secure,
Path=/, SameSite=Lax, no Domain. Reviewed synthetic client/identity/binding fixtures
are required to exercise pending cookie issuance, rotation, old-cookie rejection,
fresh CSRF, exact challenge/transaction correlation, separate grant and logout
cookie deletion. Cookies must not cross from connect to mcp. SIWE test activation
requires explicit reviewed fixtures and test-only configuration, never production
activation. Full fixture E2E is WP3D-2; no real wallet or Arc RPC is needed.

## Shutdown and validation

```sh
docker compose -f deploy/local/compose.yaml down
node deploy/local/config.test.mjs
GOPROXY=off go test -count=1 ./internal/config
(cd web/approval-ui && ./node_modules/.bin/tsc --noEmit --incremental false)
git diff --check
```

Shutdown affects only this dedicated project; PostgreSQL tmpfs data is disposable.
Confirm ownership before stopping. Delete only phase-created artifacts once stopped,
and undo any separately approved browser trust/mappings. No broad cleanup or volume
prune is authorized. TLS keys and built binary remain Git-ignored runtime material.

Static tests are not Nginx syntax, HTTPS routing or cookie integration proof.
Current Docker Compose syntax, config tests, Go config and TypeScript validation
are recorded in the implementation report. No Docker runtime was executed here.
Public deployment still requires rate limits, retention, CSP, least-privilege DB
roles, operational review and separate activation approval. No financial worker,
Mainnet execution, CAW, autonomous dispatch or production SIWE is enabled.

## Preserved validation history

Inspection found Go, Node/npm, OpenSSL, curl and Docker Compose available and
frontend node_modules present. Nginx/Caddy were absent. Docker socket access was
denied, so image availability could not be established. No installation or image
pull was performed. Proxy runtime and Nginx syntax validation await an approved
installed binary; this configuration has only static contract validation here.


Owner reported HENA Docker Desktop working, cached postgres:16-alpine, seven static
tests, PostgreSQL/full Go/vet/TypeScript/Next production build passing before this
topology change. Those are historical owner reports, not current Docker-topology
execution evidence. The prior host-process template was not Docker compatible.

## Edge publication remediation — owner-run only

Owner inspection found requested HostConfig.PortBindings but empty effective
NetworkSettings.Ports on the internal-only topology. Internal-only attachment is
a plausible explanation, not a verified Docker Engine 29 root cause. Engine 29
and Docker Desktop behavior must be established on HENA; Codex daemon access is
denied. A standard project-scoped bridge adds a publication-capable edge while
retaining internal upstream isolation. No special host networking is used.

From repository root, with existing private Compose environment variables set:

```sh
docker version --format '{{.Server.Version}}'
docker compose -f deploy/local/compose.yaml config --quiet
# Print network/port data only; avoid full config/inspect dumps with DB credentials.
docker compose -f deploy/local/compose.yaml config --format json | python3 -c 'import json,sys; c=json.load(sys.stdin); print(c["networks"]); print({k:{"networks":v.get("networks"),"ports":v.get("ports",[])} for k,v in c["services"].items()})'
# Record project container IDs before/after; only nginx should change.
docker compose -f deploy/local/compose.yaml ps -q go frontend postgres
docker compose -f deploy/local/compose.yaml up -d --no-deps --no-build --pull never --force-recreate nginx
docker compose -f deploy/local/compose.yaml ps -q go frontend postgres
nginx_id=$(docker compose -f deploy/local/compose.yaml ps -q nginx)
test -n "$nginx_id"
docker inspect "$nginx_id" --format '{{json .HostConfig.PortBindings}}'
docker inspect "$nginx_id" --format '{{json .NetworkSettings.Ports}}'
docker inspect "$nginx_id" --format '{{json .NetworkSettings.Networks}}'
docker port "$nginx_id" 8443/tcp
```

Expect docker port to report only 127.0.0.1:8443, with the effective 8443/tcp entry
containing HostIp=127.0.0.1 and HostPort=8443. STOP for 0.0.0.0, :: or any other
binding; do not proceed assuming localhost-only isolation. Verify project_private
is internal, project_edge is a regular bridge with only Nginx attached, and the
other service container IDs and memberships are unchanged. Inspect actual network
names from the container output rather than assuming global network names.

Then execute the verified-TLS curl --connect-to metadata/MCP checks above. Do not
claim connectivity from port bindings alone. If it still fails, collect only
scoped network/port inspection, Docker version/context, Nginx state and redacted
critical errors; check loopback TCP listener with `ss -ltn 'sport = :8443'` (Desktop
forwarding may not appear as a normal WSL process socket). Do not dump environments,
restart Docker Desktop/unrelated services, alter firewall, bind wildcard ports or
introduce host networking. Stop and report the remaining Desktop/WSL publication
problem. This phase recreates only the owner-approved Nginx service on HENA.
