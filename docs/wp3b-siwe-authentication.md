# WP3B-1: restricted SIWE authentication

This implementation uses existing go-ethereum v1.16.7. It adds no SIWE package,
wallet SDK, cryptographic dependency, financial permission or provider call.
It is not a general SIWE parser and does not claim verified AI-host interoperability.

## Activation and identity policy

Authentication is disabled by default. `WIZPAY_SIWE_ENABLED=true` is accepted only
in development/test, with AUTH_REQUIRED and OAUTH_ENABLED enabled and autonomy
explicitly disabled. Staging and production activation is rejected. The process
must explicitly supply `WIZPAY_ONBOARDING_TENANT_ID` and
`WIZPAY_SIWE_ORIGIN=https://connect.wizpay.xyz`. No localhost or alternate-chain
fallback exists. Tests inject trusted configuration, not HTTP identity claims.

Migration 010 adds INACTIVE/ACTIVE/REVOKED tenant lifecycle status. Existing and
new tenants default INACTIVE. No tenant is approved or activated by this package.
Revocation is terminal. Administrative approval, restricted database privileges
and tenant review remain owner responsibilities; there is no activation HTTP API.

Only one case-insensitive address match within the configured tenant, chain 5042
and MAINNET is acceptable. Multiple matches are conflicts even if one is inactive.
The binding must be ACTIVE, EXTERNAL_EVM, have non-future verification time,
nonempty evidence and no revocation. Its existing owner must have an ACTIVE identity.
No user, tenant or binding is created, transferred, recovered or reactivated.
Authentication records preserve binding ID/version and existing issuer/subject;
SIWE proof is evidence of wallet control, not a replacement identity-provider claim.

## Restricted EIP-4361 profile

The server emits version 1, canonical EIP-55 address, fixed domain, configured
canonical HTTPS URI, chain 5042, a 256-bit hex nonce, RFC3339 timestamps, and an
explicit expiration of at most two minutes (bounded by pending session/transaction).
Statement and field order are fixed. Optional not-before is not supported.
Each pending session may issue at most three challenges. Distributed rate limiting
remains required before public exposure.

Lower/upper-case addresses are accepted and displayed canonically; mixed case
requires a valid checksum. Message bytes are persisted exactly. Verification
accepts only challenge ID and signature, never client message/domain/URI/chain/time.
The stored message is not reconstructed or normalized during verification.

EOA recovery requires strict 0x hex, 65 bytes, recovery ID 0/1 or 27/28, valid R/S
and low-S. ERC-191 uses byte length and applies the prefix once. Recovered address
must equal the expected signer. Malformed inputs return a safe error, never panic.
EIP-712, EIP-1271 and EIP-6492 verification paths are absent. Non-65-byte wrappers
are rejected. ECDSA recovery does not prove an address has no deployed code;
account classification and delegated-account policy require separate approval.

## HTTP and browser authority

POST /browser/siwe/challenge accepts exactly `{ "address": "0x..." }`.
POST /browser/siwe/verify accepts exactly `{ "challenge_id": "...", "signature": "0x..." }`.
Both require canonical Host/Origin, no forwarded authority headers, the browser
cookie and session-bound X-CSRF-Token. Unknown, duplicate, empty or missing JSON
fields are rejected. Bodies are limited to 1 KiB, transport reads to five seconds;
unsupported read deadlines fail closed. Work has a five-second context deadline
in addition to bounded database queries. Error responses do not enumerate owners.
No signature, cookie or message logging is added.

Success replaces the __Host-wizpay-browser cookie: Secure, HttpOnly, Path=/,
SameSite=Lax, no Domain. New credential and CSRF authority are independent of the
old values. Only credential digests persist. Logout/revocation remain WP3A operations.
The old Authenticate seam is not used: VerifyWallet calls the atomic repository
finalizer directly. No production fixture or client-supplied principal exists.

## Atomic database finalization

The finalizer locks browser authority, unconsumed challenge, OAuth transaction and
registered client; resolves the existing owner; locks identity, tenant and all
matching bindings; then rechecks authority. Identity precedes tenant/binding locks
to agree with OAuth redemption. PostgreSQL clock_timestamp() is sampled after
locks and again after signature recovery; transaction-start time is never the
SIWE expiry authority. Expired/revoked/denied/completed or mismatched records fail.

Challenge consumption, old-session revocation, authenticated-session insertion and
immutable siwe_authentications audit/evidence insertion share one pgx transaction.
Any error rolls back every write. Single-use markers and immutable snapshots are
also enforced by triggers. Authenticated evidence has owner/version foreign keys.
No raw signatures are persisted. Audit links identify the proof and binding only.

Explicit OAuth consent remains separate. Read-only mcp:read, S256 and exact callback,
client/resource checks remain WP2 authority. SIWE binding-version/tenant checks apply
at consent, code redemption and token validation. Existing legacy browser fixtures
without SIWE evidence retain prior WP3A checks. Revocation does not cancel operations
already authenticated and in flight, matching the existing WP2 bound.

Pending sessions referenced by challenges are excluded from WP3A cleanup. Challenge
and evidence retention/cleanup needs an owner-reviewed policy; records are not
silently deleted to restore authority. No destructive rollback migration exists.
Migrations 001–009 and immutable financial snapshots are untouched. SQL is handwritten
pgx, but sqlc schema models still require pinned regeneration for migration 010.

## Frontend limitations

The existing frontend has no injected-wallet connector or SDK. This package does
not add one. It accurately reports unavailable wallet connection, and offers no
fake sign-in, manual identity entry or manufactured success. Grant is available
only for a genuine authenticated server view and requires a separate click.
Returned callbacks must be HTTPS without URL credentials; the server enforces the
exact registered URI. No token/credential localStorage or financial approval UI
activation is introduced. Full connect/sign/message-display UX remains deferred.

## Validation and remaining requirements

Unit tests cover formatting, checksum, nonce structure, exact bytes/UTF-8, wrong
signer/message, malformed signature lengths, recovery IDs, invalid R/S, high-S,
time bounds and rejection of typed-data signatures. HTTP tests cover strict JSON
and unavailable production verification; existing origin/CSRF/cookie tests remain.
PostgreSQL tests cover concurrent/replayed authentication, revoked authority and
session swapping, challenge admission bounds, tenant defaults, ambiguous addresses,
observed lock waits crossing challenge/session expiry, competing logout and injected
rotation/audit failures. These require actual Testcontainers execution.

Local focused unit and race checks and TypeScript validation passed. PostgreSQL
execution is blocked by denied Docker socket access. Full-suite socket-based tests
are also blocked. No database smoke test substitutes for concurrency acceptance.
Pinned sqlc is unavailable offline (GOPROXY=off); generated models were not invented.
No dependency download was performed. sqlc download requires separate approval.

Owner validation in an authorized isolated development environment:

```sh
GOPROXY=off go test -count=1 ./internal/siwe ./internal/browser ./internal/http/browser ./internal/config ./internal/auth ./internal/requestauth ./internal/http/approval ./internal/oauth ./internal/services ./internal/mcp/...
GOPROXY=off go test -race -count=1 ./internal/siwe ./internal/browser ./internal/http/browser
GOPROXY=off go test -count=1 -v ./internal/storage/postgres -run 'TestSIWE|TestBrowser|TestOAuth'
GOPROXY=off go test -race -count=1 ./internal/storage/postgres -run 'TestSIWE|TestBrowser|TestOAuth'
# Only with the already available pinned tool, or after separate download approval:
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 compile
before=$(rg --files internal/storage/postgres/dbsqlc | sort | xargs sha256sum)
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
after=$(rg --files internal/storage/postgres/dbsqlc | sort | xargs sha256sum)
test "$before" = "$after"
(cd web/approval-ui && ./node_modules/.bin/tsc --noEmit --incremental false)
WIZPAY_ARC_INTEGRATION=0 GOPROXY=off go test -count=1 ./...
GOPROXY=off go vet ./...
git diff --check
```

No live Arc/Circle calls, real wallet signatures, provider activation or deployment
are required by these tests. Human financial approval remains separately protected:
no HumanAuthenticator is added. Financial execution, CAW and autonomous dispatch
remain disabled. Public rate limiting, database least privilege, retention, proxy
and HTTP/2 validation, recovery policy and actual wallet/client interoperability
remain prerequisites. Missing integration execution and sqlc reproducibility are
review blockers, not evidence of production readiness.
