# WP3A: browser sessions and OAuth consent foundation

WP3A adds infrastructure, not real-user authentication. The canonical issuer is
`https://connect.wizpay.xyz`; the resource remains `https://mcp.wizpay.xyz/mcp`.
Neither origin is assumed deployed. No SIWE package, wallet connection, wallet
activation, provider operation, human financial approval, CAW or autonomous
execution is added.

## Implemented flow

A valid `/oauth/authorize` request still passes the WP2 registered-client, exact
redirect, resource, scope and S256 checks. Production OAuth wiring now creates an
anonymous PENDING browser record tied to that exact durable transaction, sets a
fresh cookie, and redirects to `/onboarding` without a transaction/session ID in
the URL. Existing standalone WP2 handler construction retains the old unavailable
response; production explicitly uses the browser-foundation constructor.

Next.js reuses `web/approval-ui`. `/onboarding` fetches `/browser/session` on the
same origin and presents the database-registered application, resource and read
scope. The UI explains that fund transfers, wallet signing and financial decisions
are outside this consent. Grant is disabled. Cancellation is operational; it
consumes the pending authorization request without creating consent, codes or
access tokens. There is no fake login, wallet-verification or success state.

## Sessions and secrets

`internal/browser` owns browser state independently of MCP bearer authority.
The 256-bit random cookie credential has its own `wb_` class. Only SHA-256 digests
are persisted. Session references are independently random non-secret identifiers.
Cookies use `__Host-wizpay-browser`, Secure, HttpOnly, Path=/, no Domain, and
SameSite=Lax. Lax supports top-level OAuth navigation while POSTs require additional
Origin and CSRF checks. No HTTP development exception is implemented.

A CSRF value is a domain-separated SHA-256 derivation of the high-entropy cookie;
only its digest is stored. The derived value is returned to the same-origin UI and
held in component memory. No raw cookie or OAuth access token is exposed to JS or
stored in localStorage. Service checks use constant-time digest comparison.
Changing the session credential changes CSRF authority; another session's proof
cannot authorize the current session.

Pending sessions live at most five minutes, bounded by the OAuth transaction.
Authenticated records live at most thirty minutes, bounded by trusted principal
expiry. Session rotation revokes the old cookie record and any related OAuth
session atomically, and creates a new immutable snapshot. Renewal requires fresh
trusted verification, the same identity, and an open, unexpired authorization
transaction. Renewal after completed consent, general-purpose login and account
switching require WP3B design; they are not silently enabled here.

## Trusted verification and consent boundaries

`IdentityVerifier.VerifyBrowser` must produce a verified principal and evidence
reference bound to the exact pending browser reference and transaction. The
service supplies no caller-chosen identity to this port. WP3A supplies no production
implementation; only `_test.go` fixtures do. `Authenticate` is an internal seam
with no HTTP caller. There is no login or authenticate route. Bootstrap passes a
nil verifier, so grant completion fails closed even if an authenticated database
record were present. Caller-supplied identity fields and authentication flags are
rejected by HTTP handlers.

The internal consent adapter supplies only the immutable session identity and
exact stored request to WP2. It supplies no human-decision permissions and never
calls `AuthenticateHumanContext`. A browser-session reference in the trusted
BrowserDecision causes PostgreSQL to lock and revalidate the browser session in
WP2's code/consent/audit transaction. Only one grant can commit. Logout locks and
revokes the browser record and associated OAuth session; existing live token
validation then rejects its access tokens. In-flight previously authenticated
operations are not cancelled, matching WP2's documented revocation bound.

All locks that can wait precede final fresh server-clock expiry checks for session
rotation, consent denial and browser-bound code issuance. Transaction-start time
and caller-supplied timestamps are not authority. Existing legacy WP2 trusted test
fixtures without a browser reference retain their established flow.

## HTTP and deployment requirements

- `GET /browser/session`: correlated consent view and CSRF proof, no code issuance.
- `POST /browser/consent`: JSON with only `decision: deny|grant`; grant unavailable
  in production pending WP3B. Cancellation is allowed for an anonymous request and
  does not represent consent by an authenticated user.
- `POST /browser/logout`: empty body, Origin and session-bound CSRF required.
- Canonical Host is required. Forwarded, X-Forwarded-Host and X-Forwarded-Proto are
  rejected; an edge must strip them before forwarding to this private listener.
- No CORS permissions are granted. Sensitive responses use no-store and no-referrer.
- POST bodies are bounded to 1 KiB and have a five-second transport read deadline;
  unsupported deadline writers fail closed. Failed reads retain deadlines and
  close HTTP/1 connections. MCP streaming has no new global timeout.
- The Next.js UI and Go endpoints must be routed under the same HTTPS connect
  origin: Next handles `/onboarding`, Go handles `/oauth/*`, `/browser/*` and
  authorization-server metadata. No deployment, DNS or proxy change is included.
- CSP/production proxy buffering, HTTP/2 deadline behavior, distributed abuse
  controls, and actual browser navigation remain deployment validation items.
  WP2's bounded authorization admission remains in place.

## Migration 009 and retention

Migration 009 creates `browser_sessions`, with digest constraints, immutable
snapshots, terminal decisions/revocation, transaction foreign keys, and explicit
nullable pending versus tenant/user-owned authenticated records. It modifies no
migrations 001–008 or historical financial records. Rotation is revoke+insert;
pending records are never promoted in place. The existing active identity is
checked under lock before authentication attachment. No users or wallets are
provisioned. Runtime role grants and evidence authenticity remain trusted-backend
boundaries; the schema cannot establish cryptographic truth on its own.

`CleanupBrowserSessions` deletes only anonymous records expired over 24 hours ago.
It is not automatically scheduled. Authenticated snapshots are retained for audit
and revocation; owner-reviewed retention, privilege restrictions and cleanup
scheduling are prerequisites before public activation. No destructive down
migration is provided. Old binaries ignore the new table; browser-bound authority
requires the new code. Do not delete sessions to restore revoked authority.

SQL statements follow the existing explicit pgx transaction pattern. No sqlc
queries are added. Schema-driven model regeneration with pinned sqlc v1.30.0 is
still required for reproducibility when tooling is available; do not hand-edit
generated files as a substitute.

## Tests and remaining WP3B work

Unit/HTTP tests cover pending-state denial, absent verifier, session/CSRF isolation,
expiry/revocation, trusted fixture rotation/renewal, exact proof correlation,
cookie attributes, origin/host/forwarding rejection, caller identity rejection,
cancellation and logout. HTTP recorder deadline adapters exist only in test files;
they do not prove network deadline behavior. Existing WP2 TCP tests remain intact.

PostgreSQL tests cover rotation, identity reassignment denial, cancellation without
codes, eight concurrent grants with one success, old-cookie rejection and OAuth
session revocation. Run these with the existing Testcontainers setup. Socket and
Docker restrictions must be reported as blockers rather than test successes.

Local checks (no dependency installation):

```sh
GOCACHE=/tmp/wizpay-wp3a-go-cache GOPROXY=off go test ./internal/browser ./internal/http/browser ./internal/auth ./internal/requestauth ./internal/http/approval ./internal/services ./internal/app ./internal/oauth ./internal/mcp/...
GOCACHE=/tmp/wizpay-wp3a-go-cache GOPROXY=off go test ./internal/storage/postgres -run 'TestBrowser|TestOAuth' -count=1
GOCACHE=/tmp/wizpay-wp3a-go-cache GOPROXY=off go vet ./...
git diff --check
```

Frontend type checks require the already approved dependencies to be available.
No lint script is currently provided in the existing manifest. WP3B must select
and validate SIWE verification, durable replay-safe challenges, wallet ownership,
identity provisioning/recovery policy, cookie rotation handoff and actual consent
completion UX. Financial human-approval activation remains a separate phase.

### Implementation validation record

Focused Go tests, focused race tests, non-network OAuth HTTP tests, PostgreSQL
test compilation, `go vet ./...` and `git diff --check` passed locally. Full-suite
execution was blocked by prohibited local sockets and Docker access; PostgreSQL
integration and real HTTP deadline tests are not independently validated here.
PostgreSQL 16 single-user mode successfully applied migrations 001–009 and checked
pending-session promotion, grant, decision reopening and revocation-reset
constraints. This is schema smoke validation, not pgx/concurrency acceptance.
Frontend type checking was blocked by absent node_modules; the existing frontend
has no lint script. Pinned sqlc generation was blocked by unavailable cached
tool dependencies with GOPROXY=off. No dependencies were installed or changed.

### Pre-commit validation remediation

Additional tests in `browser_security_integration_test.go` cover concurrent
rotation; competing grant/logout and grant/denial with synchronized starts;
transaction, browser and existing OAuth-session expiry after observed database
lock waits; client/tenant/user isolation using real isolated fixture records;
terminal consent revocation; and injected INSERT failures at consent, code and
AUTHORIZATION audit persistence. Rollback assertions check that no partial code,
consent, newly inserted session, audit or consumed transaction survives. These
fixtures and fault-injection triggers exist only in tests and are cleaned up.
The scope of tests does not change production authority or authentication wiring.
Unit coverage also checks exact consent snapshot bindings, including PKCE/state.

Migration 009 should add a schema-derived `BrowserSession` model to
`dbsqlc/models.go`: its digest/reference/state fields, nullable identity/evidence
fields, creation/expiry timestamps and nullable revocation timestamp. No sqlc
query inputs or query result shapes were changed. Pinned generation, compile and
repeat-generation comparison remain mandatory; this expected output has not been
manually substituted for generated code. The offline pinned-tool probe fails with
`module lookup disabled by GOPROXY=off`; download requires owner approval.

The existing frontend lockfile must be preserved. `node_modules` is absent, so
TypeScript cannot run until the owner authorizes installation of the existing
locked dependencies. No packages or application dependency versions changed.
Docker API access and both TCP and Unix-domain sockets are denied in this sandbox.
Installed PostgreSQL 16 alone cannot supply a usable pgx test listener. No external
DB or single-user smoke check is substituted for concurrency validation.

Owner validation in an authorized local environment (never production):

```sh
# After explicit approval for the pinned development-tool download:
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 compile
wp3a_generated_before=$(rg --files internal/storage/postgres/dbsqlc | sort | xargs sha256sum)
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
wp3a_generated_after=$(rg --files internal/storage/postgres/dbsqlc | sort | xargs sha256sum)
test "$wp3a_generated_before" = "$wp3a_generated_after"
go test -count=1 ./internal/storage/postgres -run 'TestBrowser|TestOAuth'
go test -race -count=1 ./internal/storage/postgres -run 'TestBrowser|TestOAuth'
# After explicit approval for installation of the existing lockfile:
(cd web/approval-ui && npm ci && ./node_modules/.bin/tsc --noEmit --incremental false)
go test -count=1 ./...
go vet ./...
git diff --check
```

Fresh focused Go and race tests pass, PostgreSQL tests compile and Go vet passes.
Actual PostgreSQL test execution, sqlc reproducibility and frontend typechecking
remain blocked. Browser rate limiting and restricted database role privileges
remain public-deployment prerequisites; this remediation does not implement them.
