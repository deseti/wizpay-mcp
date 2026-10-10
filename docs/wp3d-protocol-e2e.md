# WP3D-2C — continuous SIWE / OAuth / MCP protocol tests

These are PostgreSQL-backed, in-process HTTP integration tests, not browser E2E,
TLS/proxy validation, real-wallet interoperability or public-deployment proof.
Production/staging SIWE restrictions and all financial execution guards remain
unchanged. No first-time provisioning feature, dependency or migration is added.

## Application and fixture architecture

`protocol_e2e_helpers_test.go` uses the existing PostgreSQL Testcontainers
`TestMain`, migrations and disposable database. Each fixture gets unique synthetic
tenant/user/client/binding identifiers. The existing base fixture seeds historical
synthetic intent and approval records for read assertions; no execution is created.
Only the test fixture explicitly activates its tenant and seeds a reviewed ACTIVE
EXTERNAL_EVM binding for its existing ACTIVE identity. It uses chain 5042 / MAINNET
and the existing deterministic test signer. Nothing provisions a real account.

The primary test never calls the trusted browser-authenticator fixture or supplies
an authenticated principal. It starts at GET /oauth/authorize, obtains the pending
cookie, retrieves CSRF, obtains a server-issued challenge, signs its exact stored
message bytes, and verifies through POST /browser/siwe/verify. Cookie rotation and
immutable challenge/transaction correlation precede explicit consent. The registered
callback's code/state/issuer are checked without following it over the network.
The code is exchanged with S256 before the resulting token initializes MCP,
discovers exactly the two read tools, and reads actual tenant-owned database rows.

The app.Server ServeHTTP method delegates to the same assembled handler used by
Run. It adds no route, authentication shortcut or listener. Handlers, services,
OAuth verification, identity resolution and repositories are the actual implementations.
The in-process ResponseWriter supplies the existing test-style read-deadline
adapter for fully buffered bodies. It does not prove connection deadlines. Requests
use canonical Host and origin-form URLs matching the existing private Go upstream.
A Go cookie jar checks host/HTTPS scope; cookie headers check Secure, HttpOnly,
Path=/, no Domain and SameSite=Lax. Actual browser SameSite/HttpOnly enforcement
requires WP3D-2D and the separately approved local HTTPS routing contract.

## Coverage

This inventory describes implemented assertions, not an executed PostgreSQL result.

- Continuous pending session -> SIWE -> rotation -> fresh CSRF/correlation ->
  explicit consent -> code -> PKCE exchange -> OAuth bearer -> MCP.
- Exact registered redirect, state, issuer, client, resource, user, tenant and
  session binding; digest-only token lookup; exactly two read permissions.
- Old-cookie and stale-CSRF rejection; pending consent rejection; wrong/missing
  CSRF, wrong Origin and cross-session proof/challenge rejection.
- Replay, wrong signer, unknown wallet, wrong binding chain and rejected browser
  chain/identity input. Expired challenge fixtures are inserted through unchanged
  constraints; no immutable record is edited or trigger disabled. Session/token
  expiry tests advance only a fixture-local injected clock without sleeping.
- Exact callback/resource/PKCE restrictions; unsuccessful exchanges do not consume
  codes; reused codes fail. Revoked consent blocks redemption and token creation.
- Concurrent HTTP code exchanges start behind one synchronization barrier and
  require exactly one success, one persisted token and one token-issuance audit.
  The continuous flow also checks consistent code/token/authorization audit counts.
- Real MCP initialization and exact tools/list; all 22 excluded names fail direct
  invocation; real tenant-owned reads succeed and foreign-tenant reads fail.
- Missing/malformed/unrecognized bearer credentials produce exact 401 challenges.
  The application unit test separately checks exact 403 insufficient_scope using
  a partial-principal verifier fixture. Production accepts only mcp:read.
- OAuth bearer rejection at human-decision and confirmation HTTP boundaries.
- Same-client/different-transaction cookie substitution returns its own durable
  correlation; the old flow's CSRF cannot grant or log out the substituted flow.
- Independent token, consent and OAuth-session revocation. Browser logout revokes
  its browser row and linked OAuth session, invalidating the token via live lookup;
  it does not mark the token or consent rows themselves revoked.

Revocation tests use the existing repository control-plane port against synthetic
records. They do not introduce a public revocation endpoint or browser-authority bypass.
Failure diagnostics deliberately omit response bodies and raw cookies, codes,
signatures and tokens. Runtime application/SDK logs are discarded in the fixture.

## Bootstrap and response regressions

`cmd/server/oauth_wiring_test.go` parses the actual bootstrap source and requires
selection of NewOAuthReadOnlyRegistry before its registrations are passed to the
OAuth browser-foundation constructor. This is a source-contract guard, not a full
bootstrap runtime test. The existing application transport test now asserts exact
401 / invalid_token and 403 / insufficient_scope status/challenge/cache behavior.

## Local validation

Use installed modules only; no downloads are required or authorized:

```bash
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 \
  ./cmd/server ./internal/app ./internal/mcp/... ./internal/oauth \
  ./internal/requestauth ./internal/browser ./internal/siwe \
  ./internal/auth ./internal/services ./internal/http/browser ./internal/http/approval
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go vet ./...
git diff --check
```

In a sandbox without Docker/socket access, compile without executing TestMain:

```bash
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -c \
  -o /tmp/wizpay-wp3d2c-postgres.test ./internal/storage/postgres
```

Compilation is not a PostgreSQL integration PASS. A sandbox with a read-only
default Go build cache can set GOCACHE to a writable task-specific /tmp directory.

## Owner-run HENA integration validation

Confirm the existing approved Testcontainers images are cached before running;
image pulls are not authorized. Do not run compose up/down, reuse the running
WP3D-1 database, or change any existing container. The test package creates and
cleans its own disposable PostgreSQL environment using the established TestMain.

```bash
docker image inspect postgres:16-alpine >/dev/null
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  go test -count=1 -v ./internal/storage/postgres -run '^TestProtocol'
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  go test -race -count=1 ./internal/storage/postgres -run '^TestProtocol'
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  go test -count=1 ./internal/storage/postgres -run 'TestSIWE|TestBrowser|TestOAuth'
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off WIZPAY_ARC_INTEGRATION=0 \
  go test -count=1 ./...
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go vet ./...
git diff --check
```

Expected: all protocol tests pass, including real database lifecycle assertions.
No PostgreSQL execution result is claimed by this document. Record owner results
separately. Testcontainers support images must also be already available.

## Remaining gaps

WP3D-2D must validate browser DOM behavior, mock EIP-1193/EIP-6963 interactions,
SameSite enforcement, HTTPS Set-Cookie delivery, frontend account/chain cancellation,
uncertain responses and multi-tab reconciliation. Existing frontend deterministic
mocks are separate evidence. Real connection deadlines, HTTP/2, Nginx streaming,
AI-host registration/interoperability and public hardening remain separate work.
Existing PostgreSQL lock-wait/concurrency tests remain necessary; this sequence
does not replace them. Public activation, recovery and provisioning remain disabled.
