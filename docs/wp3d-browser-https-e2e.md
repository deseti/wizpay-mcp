# WP3D-2D — owner-run real-browser HTTPS harness

Implemented, not browser-executed. This adds test support only: no production
handler, manifest, migration, Compose configuration or authentication policy changes.
Node controller tests and Go signer tests are NOT real-browser results.

## Architecture and prerequisites

`tests/browser-e2e/run.mjs` uses Node 22's built-in WebSocket and an owner-started
Chromium browser's loopback Chrome DevTools Protocol endpoint. No Playwright,
browser, driver or wallet SDK is downloaded. The harness creates a fresh incognito
browser context and disposes only that context. Use a dedicated disposable browser
process/profile, never an ordinary personal browser or one containing real wallets.
Do not enable remote debugging on a public address. CDP can read browser credentials:
restrict it to loopback and end the browser process after testing.

The actual Next.js page discovers an injected EIP-6963 provider, explicitly connects
through EIP-1193 and calls personal_sign with exact UTF-8 hexadecimal message bytes
and the selected address. A separate Go process generates a random ephemeral key
using already-approved go-ethereum. It accepts only the exact, unexpired restricted
server SIWE profile for its own address. It never serializes the key. Only public
fixture SQL/metadata are written under Git-ignored `deploy/local/runtime/`.
Keys disappear when the process exits; restarting requires a NEW fixture and tenant.
Do not reuse these synthetic bindings as a real identity or recovery mechanism.

The signer communicates over inherited pipes, not a public signing endpoint. It
has no RPC, wallet extension, database connection or financial-signature method.
No HAR, screenshots, network bodies, cookies, tokens or signatures are logged.
The registered callback is `https://connect.wizpay.xyz/browser-e2e/callback`.
Nginx deliberately returns 404 there; the harness inspects the real navigated URL
without adding a callback handler or contacting another host. The URL contains a
short-lived code: never record browser screenshots/history exports or CDP traces.
Backend authentication responses are not mocked or intercepted to fabricate success.

## Implemented browser assertions (await execution)

- Real /oauth/authorize navigation, pending session and onboarding UI.
- Secure context; Secure/HttpOnly/host-only/Path=/SameSite=Lax cookie inspection;
  no document.cookie access and no sibling-host cookie eligibility.
- Missing CSRF, authenticated stale CSRF, revoked old-cookie reuse.
- Explicit wallet selection, connection, signing and separate consent.
- Exact issued challenge correlation through CDP response observation, transaction
  continuity, identity, rotated cookie and fresh CSRF; reload after authentication.
- UI denial; wrong network; rejected signing; delayed signing followed by account, chain or
  disconnect events; consent stays disabled after invalidation.
- Same-client/different-transaction multi-tab rotation: old tab cannot grant and
  cannot destroy the substituted session. This now uses two ephemeral wallets and
  different synthetic users within the configured test tenant.
- Invalid registered redirect rejection. A response-stage CDP fault aborts SIWE
  delivery only after the actual backend has returned 200; the UI must reconcile
  and require restart without enabling consent. This is browser transport fault
  injection, not a fabricated authentication response. Cookie delivery on an
  interrupted response is browser-dependent; correlated authority may remain
  until expiry if cleanup cannot reach it. No unconditional logout is asserted.
- Actual HTTPS callback state/issuer/code, S256 token exchange, MCP initialize,
  exact two-tool discovery and direct rejection of all 22 excluded tool names.
- Missing/invalid MCP bearer challenges, consumed SIWE challenge rejection, logout,
  cookie deletion and session rejection. WP3D_BROWSER_EXPIRY=yes additionally waits
  for a real issued challenge to expire and asserts verification rejection.

After successful consent, browser-session presentation is intentionally rejected
because its OAuth transaction/decision is terminal. Logout uses the still-current
cookie and CSRF obtained after authentication, not a fabricated post-grant session
refresh. Consent does not itself rotate the browser credential.

The Node harness tests cover mock discovery, exact Unicode signing bytes, parameter
ordering, refusal of transaction methods, rejection controls and listener cleanup.
The Go unit test covers the signer's restricted-profile and expiry gate.

Not implemented as browser assertions yet: wrong-Origin cross-origin transport
inspection, actual cross-site SameSite requests, HTTP versus HTTPS cookie sending,
pre-sign delayed account/
chain reads, explicit provider replacement and backend binding revocation. WP3C
controller tests and WP3D-2C protocol tests provide separate coverage, not browser
proof. No real-wallet/AI-client interoperability or production readiness is claimed.

## Offline checks (no installation)

From the repository root:

```sh
node --test tests/browser-e2e/harness.test.mjs
node --check tests/browser-e2e/run.mjs
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test ./tests/browser-e2e/signer
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go vet ./tests/browser-e2e/signer
node deploy/local/config.test.mjs
(cd web/approval-ui && ./node_modules/.bin/tsc --noEmit --incremental false)
git diff --check
```

Compile the signer only with installed modules:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go build \
  -o deploy/local/runtime/browser-e2e-signer ./tests/browser-e2e/signer
```

## Approval-dependent owner preparation — do not execute automatically

The existing WP3D-1 stack must not be assumed current or modified by this harness.
Approve a SEPARATE Compose project (`wizpay-mcp-wp3d-browser`) before execution.
Use the reviewed local configuration and already-cached images, with the test-only
`tests/browser-e2e/compose.override.yaml`. Required differences:

1. PostgreSQL POSTGRES_DB and both Go database URLs target exactly
   `wizpay_mcp_browser_e2e`, a NEW disposable tmpfs database. Fix its healthcheck to
   target that database as well. Do not copy or reset the owner's running database.
2. Only the new Nginx publishes `127.0.0.1:9443:8443`, replacing rather than merging
   the original port list. With Compose supporting `!override`, the override is
   `ports: !override ["127.0.0.1:9443:8443"]`. Verify effective publication before
   starting; reject wildcard IPv4/IPv6 bindings. Do not alter the existing project.
3. Go uses APP_ENV=test, WIZPAY_SIWE_ENABLED=true, the exact generated
   WIZPAY_ONBOARDING_TENANT_ID, WIZPAY_SIWE_ORIGIN=https://connect.wizpay.xyz,
   AUTH_REQUIRED=true, OAUTH_ENABLED=true, WIZPAY_AUTONOMY_ENABLED=false.
   These are test-only settings. Production/staging guards remain unchanged.
4. Build current Go/Next artifacts only AFTER separate owner approval, following
   WP3D-1's offline CGO_ENABLED=1 Debian-compatible build instructions. Tag the new
   test images distinctly and override image names so the old running artifacts
   are not assumed or replaced. No pulls; no dependency installation.

Run the harness in terminal A; it generates fixture SQL and waits for READY:

```sh
WP3D_BROWSER_OWNER_APPROVED=yes \
WP3D_BROWSER_CDP='ws://127.0.0.1:9222/devtools/browser/OWNER_BROWSER_ID' \
node tests/browser-e2e/run.mjs
```

In terminal B, read only the public JSON metadata to set the test tenant. With
privately supplied disposable DB environment and the approved runtime override:

```sh
export WP3D_BROWSER_TENANT_ID="$(node -p 'JSON.parse(require("fs").readFileSync("deploy/local/runtime/browser-e2e-public.json","utf8")).tenant_id')"
# WP3D_TEST_DATABASE_URL must target postgres:5432/wizpay_mcp_browser_e2e.
# Supply disposable credentials privately; do not print the URL or reuse production.
docker compose -p wizpay-mcp-wp3d-browser -f deploy/local/compose.yaml \
  -f tests/browser-e2e/compose.override.yaml config --quiet
# Only AFTER approval and offline artifact preparation (WP3D-1), build distinct images.
DOCKER_BUILDKIT=1 docker compose -p wizpay-mcp-wp3d-browser \
  -f deploy/local/compose.yaml -f tests/browser-e2e/compose.override.yaml \
  build --pull=false go frontend
# After explicit owner approval, start ONLY this new project with cached images.
docker compose -p wizpay-mcp-wp3d-browser -f deploy/local/compose.yaml \
  -f tests/browser-e2e/compose.override.yaml up -d --pull never --no-build
# Go startup applies the existing migrations. Seed only after schema is ready.
docker compose -p wizpay-mcp-wp3d-browser -f deploy/local/compose.yaml \
  -f tests/browser-e2e/compose.override.yaml exec -T postgres \
  psql -X -v ON_ERROR_STOP=1 -U wizpay_mcp_wp3d_test \
  -d wizpay_mcp_browser_e2e < deploy/local/runtime/browser-e2e-fixture.sql
```

Confirm the dedicated edge, not the original project:

```sh
docker compose -p wizpay-mcp-wp3d-browser -f deploy/local/compose.yaml \
  -f tests/browser-e2e/compose.override.yaml port nginx 8443
```

Expected exactly 127.0.0.1:9443. Check private-network membership and unchanged
original project container IDs as in WP3D-1. Reject merged 8443 publication,
wildcard publication, shared networks or unexpected database URLs. Never issue
up/down commands without the explicit new `-p` project argument.

The seed transaction checks current_database and inserts unique records without
ON CONFLICT reactivation. It invokes existing binding-version triggers. This is
explicit synthetic test setup, never production onboarding. Do not print full
Compose configuration, credentials or container environments.

## Canonical HTTPS and certificate trust — owner decision required

Canonical URLs must use port 443; browsing :8443 or :9443 changes Origin and fails
backend policy. No system hosts/DNS/firewall/trust changes are made here.

An optional dependency-free TCP relay is supplied but NOT started. After separate
owner approval it binds only 127.0.0.1:443 and forwards unchanged TLS bytes to the
new test edge at 127.0.0.1:9443:

```sh
WP3D_BROWSER_PORT_MAPPING_APPROVED=yes node tests/browser-e2e/loopback-relay.mjs
```

If binding fails, STOP; do not escalate privileges, change global firewall/sysctl,
use host networking or fall back to a public address. Owner must resolve the local
mapping policy separately. Stop the relay with Ctrl-C after validation.

Use an already-installed Chromium with a disposable profile and browser-only
hostname resolver rules, for example:

```sh
/OWNER/INSTALLED/CHROMIUM --user-data-dir=/OWNER/DISPOSABLE/PROFILE \
  --remote-debugging-address=127.0.0.1 --remote-debugging-port=9222 \
  --host-resolver-rules='MAP connect.wizpay.xyz 127.0.0.1, MAP mcp.wizpay.xyz 127.0.0.1, MAP * ~NOTFOUND'
```

This template requires owner verification on the installed browser/WSL environment;
it is not an executed launch record. Use no personal profile, extensions, password
store, sync, proxy/PAC or certificate-error exceptions. Do not use --no-sandbox,
--ignore-certificate-errors, CDP Security.setIgnoreCertificateErrors or disabled
web security. Obtain the browser WebSocket endpoint locally without saving traces.

The existing self-signed leaf is NOT assumed browser-trusted. Approve a genuinely
verified temporary trust arrangement scoped to an isolated browser environment,
using already-installed tools. Linux Chromium may use user-level NSS trust outside
its profile: --user-data-dir alone does NOT isolate trust. Do not modify that store
or OS trust automatically. If isolated trust cannot be established, STOP: browser
execution is blocked. A certificate error click-through is not acceptable evidence.
Before entering READY, owner must verify both canonical HTTPS origins load without
certificate errors, inspect certificate SAN/expiry, and confirm loopback-only ports
443/9443/9222 and untouched WP3D-1 container IDs. curl --cacert can independently
validate TLS through port 443, but cannot establish browser trust.

## Cleanup and evidence

The harness disposes its own browser context and terminates its ephemeral signer.
Do not export its browser history, traffic, memory or sensitive response bodies.
Stop only the dedicated browser, optional relay and explicitly approved NEW project.
Do not run docker prune or remove the existing project/database. Delete only new
fixture SQL/public metadata/profile/trust material after owner-approved cleanup.
Immutable retained authentication evidence is cleaned by disposing the new database,
not by deleting correlated production records. Record individual browser scenario
PASS/FAIL results only after actual execution. Current status: harness implemented;
browser/runtime HTTPS execution BLOCKED pending installed browser, isolated trust,
approved mapping, current dedicated test images and synthetic database setup.

## Implementation-session validation evidence

Executed locally: seven harness unit tests; 34 existing transpiled onboarding
controller tests; frontend TypeScript noEmit; signer package unit test and race
test; scoped Go vet; JavaScript syntax checks; 12 existing deployment static tests;
merged Compose config --quiet with non-secret placeholder environment; and
git diff --check. All passed. TypeScript test output was confined to /tmp.
The signer executable was not built, the browser runner/relay were not started,
and no Docker service or database fixture was created, seeded or modified.
No real-browser, HTTPS-cookie, fault-injection or actual Compose-runtime result
is asserted here. Owner execution is required to establish those outcomes.

Browser launch must include --enable-automation and
--remote-debugging-address=127.0.0.1. The harness reads the actual browser command
line and rejects sandbox/TLS/web-security bypass flags before creating its context.
