# Phase B browser harness preparation (baseline 13cc761)

Phase B.1 prepares test code only. No real-browser, Compose-runtime, PostgreSQL
integration, certificate-trust or port-mapping result has been obtained. Phase B.2
requires explicit owner approval. Production application code and Phase A controls
are unchanged. Do not use the historical ACTIVE-binding bootstrap in older WP3D
instructions with the current server.

## Synthetic enrollment and independent owner activation

The signer generates two test-only keys in process memory. It never serializes
keys; only restricted server-issued AUTHENTICATE and ENROLL messages for those
addresses can be signed. No wallet transaction or chain call is supported.

`fixture.sql` guards the database name, creates a tenant-scoped expiring NOLOGIN
owner role, registers the mcp:read synthetic client, and calls the real
`admin_onboarding('STAGE', ...)` under that role's session authorization. Both
identities remain UNKNOWN, bindings PENDING/version 1 and tenant INACTIVE. No
verified metadata, enrollment proof or approval is fabricated.

After owner application of that SQL and READY, the browser requests ENROLL
challenges, checks consent denial and invalid-signature rejection, and signs with
the ephemeral synthetic wallets. The real backend verifies signatures and creates
immutable proof. The UI's enrollment logout must leave session presentation denied
and consent blocked. Only then does the runner write `activate.sql` and wait for
ACTIVATED. The owner separately reviews/applies this script, which invokes
`admin_activate_wallet` under the delegated role and checks actual proof/approval
and binding-version relationships. It cannot insert proof or directly activate
bindings. Fresh OAuth/SIWE AUTHENTICATE attempts follow activation.

The role switch is local test bootstrap using the disposable database owner,
matching existing PostgreSQL tests. It is not production runtime least-privilege
validation. Owner consent to test setup is not a financial approval.

## Prepared browser scenarios

- Existing denial, wrong-network, rejected/delayed signing, replay, stale CSRF,
  shared-cookie tabs, interrupted real verification and two-tool MCP allowlist.
- One failed post-verification session GET followed by exactly one successful
  recovery GET: one verification, two reads, no automatic consent or logout.
- Two failed session reads: no third read, verification replay, consent or logout.
- Negative correlation corruption of a real authenticated session read
  (transaction/challenge/CSRF): consent and cleanup remain blocked. This is
  explicitly negative fault injection, not manufactured verification success.
  These injected response IDs alone may lack network metadata; other responses
  require local, trusted transport metadata.
- A newer real OAuth tab replacing the cookie before the older tab verifies:
  stale verification fails and the newer transaction remains intact.
- Navigation away while a real successful verification response is held: no
  session read, logout, consent or callback from the departed page. Navigation
  may cancel the fetch; exact dispose/late-promise races remain controller tests,
  not a claim that the destroyed browser controller resumed.
- A delayed successful logout response has no Set-Cookie, and cannot replace or
  revoke a newer authenticated tab's cookie/session.
- Optional real five-minute transaction expiry and ten-minute access-token expiry,
  then fresh OAuth, SIWE, explicit consent, fresh state/S256 PKCE, and both read
  tools. No clock, DB timestamps, token lifetime or refresh behavior is changed.

Enable `WP3D_BROWSER_EXPIRY=yes` for the two-minute challenge case and
`WP3D_BROWSER_LONG_EXPIRY=yes` for transaction/token expiry. Disabled long cases
print SKIP and are not complete Phase B validation. Long cases add about 15
minutes plus challenge waiting and normal flow time. Only static stage/checkpoint
failure diagnostics are printed: no HAR, screenshots, network-body dumps, callback
codes, cookies, CSRF, signatures or tokens.

## HENA browser, TLS and port prerequisites

Prefer an owner-provisioned Linux Chromium process in a disposable OS account or
VM/container, with a fresh profile and certificate trust isolated from the ordinary
user/Windows certificate stores. Merely setting --user-data-dir does not isolate
trust. Preserve the process account's HOME; do not change global NSS/OS/Windows
trust. A browser container would require a separately approved image and reviewed
loopback connectivity; none is added, pulled or started by Phase B.1.
Preparation, controller and browser must run in the isolated execution environment
that owns the reserved run directory, with approved offline tooling/caches. Do not
make another account's private runtime or signing process world-accessible to solve
permissions; use that execution account's UID/GID for the test edge.

HENA assessment found Windows Chrome/Edge, but no Linux Chromium or certutil.
Installation/image acquisition, trust provisioning and browser startup remain
blocked pending separate approval. The older cloud-trust.sh is hardcoded for a
different cloud workspace; do not run it unchanged in HENA.

No host port 443 relay is required by the new harness. Browser resolver rules map
only the canonical hosts' destination port 443 to 127.0.0.1:9443 and fail other DNS
names. Node token/discovery requests directly dial 127.0.0.1:9443 with canonical
Host/SNI and a test CA, without DNS or a shared agent. URL origins remain canonical;
all server Host, Origin, CSRF and TLS checks stay enabled. Chromium's resolver
supports replacement ports in [HostMappingRules](https://chromium.googlesource.com/chromium/src/+/refs/heads/main/net/base/host_mapping_rules.cc)
and applies those mappings in [MappedHostResolver](https://chromium.googlesource.com/chromium/src/+/refs/heads/main/net/dns/mapped_host_resolver.cc).
Actual installed-browser behavior remains a Phase B.2 compatibility check. If it
fails, stop; do not bypass TLS or weaken server origins.

The runner requires these exact sandboxed-browser policies: loopback debugging
address/port 9222, the run's profile path, no proxy, no extensions, disabled
background networking/QUIC and the exact resolver rules. It rejects conflicting
resolver/proxy/debugging flags and TLS/sandbox bypass. CDP Fetch.continueResponse,
response-body fault injection, trusted securityState and remote-address/port
metadata must work in the chosen browser. Unsupported capabilities are blockers.

## Isolated Phase B.2 commands — approval required; not executed

Use the standalone `tests/browser-e2e/compose.phase-b.yaml`, not an override of an
existing project. It has distinct phaseb-* service names, run-specific images,
new private/edge networks, tmpfs PostgreSQL, no DB/upstream publication, no env_file,
production mounts or external networks. It fixes both DB URLs to phaseb-db and
wizpay_mcp_browser_e2e, and disables Arc/Circle/autonomy. Only 127.0.0.1:9443 is
published. The new edge still shares its private network with upstreams; this is
local test isolation, not a hardened production edge architecture.

Choose a new lowercase alphanumeric/hyphen run ID. Never reuse directories or
existing project resources. Preflight validates available image cache and ports;
stop if a base image/dependency is absent. No installation or pull is authorized.

```sh
export PHASE_B_RUN_ID=13cc761-01
export PHASE_B_RUNTIME="$PWD/tests/browser-e2e/runtime/$PHASE_B_RUN_ID"
export WP3D_LOCAL_UID="$(id -u)" WP3D_LOCAL_GID="$(id -g)"
test "$WP3D_LOCAL_UID" != 0
umask 077
mkdir -p tests/browser-e2e/runtime
mkdir "$PHASE_B_RUNTIME"  # existing run must fail
CGO_ENABLED=1 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  go build -o "$PHASE_B_RUNTIME/server" ./cmd/server
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  go build -o "$PHASE_B_RUNTIME/signer" ./tests/browser-e2e/signer
(cd web/approval-ui && NEXT_TELEMETRY_DISABLED=1 npm run build)
```

Separately approved owner preparation must place a fresh test CA public certificate
and matching SAN-valid server certificate/private key under this run's `tls/`,
and establish browser-only trusted roots in the disposable account/VM. Never reuse
production material or print private keys. Backend image context includes only
run server binaries; frontend context excludes TLS/credential material.

After that preparation, launch the owner-approved browser from its isolated
account (replace the binary placeholder with the approved installed binary):

```sh
/OWNER/APPROVED/CHROMIUM --headless=new --enable-automation \
  --user-data-dir="$PHASE_B_RUNTIME/profile" \
  --remote-debugging-address=127.0.0.1 --remote-debugging-port=9222 \
  --no-proxy-server --disable-extensions --disable-background-networking --disable-quic \
  --host-resolver-rules='MAP connect.wizpay.xyz:443 127.0.0.1:9443, MAP mcp.wizpay.xyz:443 127.0.0.1:9443, MAP * ~NOTFOUND'
```

Obtain its local browser WebSocket endpoint without a traffic trace. Terminal A:

```sh
WP3D_BROWSER_OWNER_APPROVED=yes WP3D_BROWSER_EXPIRY=yes \
WP3D_BROWSER_LONG_EXPIRY=yes \
WP3D_BROWSER_CDP='ws://127.0.0.1:9222/devtools/browser/OWNER_BROWSER_ID' \
node tests/browser-e2e/run.mjs
```

The runner reserves fixture/public files exclusively and waits for READY. Keep
its process alive so its signing keys remain available. Terminal B:

```sh
export WP3D_BROWSER_TENANT_ID="$(node -p 'JSON.parse(require("fs").readFileSync(process.env.PHASE_B_RUNTIME+"/public.json","utf8")).tenant_id')"
# Privately generate a new local-only hexadecimal WP3D_TEST_DB_PASSWORD.
# Do not echo it, reuse production credentials, or print full Compose config.
phaseb_compose() {
  env -u DOCKER_HOST -u DOCKER_CONTEXT -u DOCKER_TLS -u DOCKER_TLS_VERIFY \
    -u DOCKER_CERT_PATH docker --host=unix:///var/run/docker.sock compose \
    --env-file /dev/null -p "wizpay-mcp-phaseb-$PHASE_B_RUN_ID" \
    -f tests/browser-e2e/compose.phase-b.yaml "$@"
}
phaseb_compose config --quiet
# Inspect only sanitized effective project/network/mount/port/DB host+name data.
# Reject shared resources, wildcard ports, nonlocal DB hosts, wrong names or files.
DOCKER_BUILDKIT=1 phaseb_compose build --pull=false phaseb-backend phaseb-ui
phaseb_compose up -d --pull never --no-build
phaseb_compose port phaseb-edge 8443  # exactly 127.0.0.1:9443
# Only after the fresh backend schema is ready:
phaseb_compose exec -T phaseb-db psql -X -qAt -v ON_ERROR_STOP=1 \
  -U wizpay_phaseb -d wizpay_mcp_browser_e2e < "$PHASE_B_RUNTIME/fixture.sql"
```

Enter READY in A only after verifying isolated resource membership and trusted
canonical HTTPS at the high-port mapping. After the runner creates activate.sql,
review its two exact synthetic proof/binding selections. Terminal B:

```sh
phaseb_compose exec -T phaseb-db psql -X -qAt -v ON_ERROR_STOP=1 \
  -U wizpay_phaseb -d wizpay_mcp_browser_e2e < "$PHASE_B_RUNTIME/activate.sql"
```

Then enter ACTIVATED in A. Do not retry failed owner SQL blindly. The real server
must remain the signature/evidence verifier; no manufactured auth success is used.

Cleanup after execution needs owner approval and must scope to this new project
(`phaseb_compose down`), browser and tracked run directory only. No docker prune,
shared-volume removal, broad process kill or production database cleanup.

## Focused local validation

These commands do not start browsers or containers. Do not replace them with
`go test ./...`: the PostgreSQL package TestMain starts Testcontainers.

```sh
node --test tests/browser-e2e/harness.test.mjs
node --check tests/browser-e2e/run.mjs
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  go test ./tests/browser-e2e/signer ./internal/siwe -count=1
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go vet ./tests/browser-e2e/signer
(cd web/approval-ui && ./node_modules/.bin/tsc --noEmit --incremental false)
(cd web/approval-ui && ./node_modules/.bin/tsc --target es2020 --module commonjs \
  --moduleResolution node --esModuleInterop --skipLibCheck --strict \
  --typeRoots node_modules/@types --outDir /tmp/wizpay-phase-b1-tests tests/onboarding.test.ts)
node --test /tmp/wizpay-phase-b1-tests/tests/onboarding.test.js
# Static Compose validation with dummy values only; no services are started:
PHASE_B_RUN_ID=static-check WP3D_LOCAL_UID=1000 WP3D_LOCAL_GID=1000 \
WP3D_BROWSER_TENANT_ID=synthetic-static-only WP3D_TEST_DB_PASSWORD=synthetic-static-placeholder \
docker compose --env-file /dev/null -f tests/browser-e2e/compose.phase-b.yaml config --quiet
git diff --check
```

Local passes prove harness logic and restricted signing, not browser/database
runtime results. Synthetic browser reauthorization does not prove Claude's actual
connector reconnect behavior. No Cloudflare Tunnel or live WizPay/Neon endpoint
is used. Concurrent tabs still share a cookie; delayed SIWE cookie rotation remains
an application limitation requiring separately reviewed handling if reproduced.
