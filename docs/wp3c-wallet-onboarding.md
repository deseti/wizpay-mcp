# WP3C-1: injected-wallet onboarding

This frontend uses only approved React, TypeScript and browser APIs. It adds no
wallet SDK, backend route, permission, migration or dependency. Production and
staging SIWE remain rejected by existing Go configuration. Nothing here activates
financial execution, human approval, CAW, autonomy or first-time provisioning.

## Supported environment and trust boundaries

Desktop injected EVM extensions and compatible wallet browsers are supported by
an EIP-1193 adapter. EIP-6963 announcements provide explicit multi-wallet discovery;
legacy window.ethereum is an explicitly selected fallback when no provider has
announced. Discovery never requests permissions. UUID/provider deduplication keeps
the first option stable; late announcements never replace the selected provider.
Wallet names are escaped text. Icons, URLs, rdns and branding confer no authority.
Discovery and provider listeners are removed on component cleanup.

QR pairing, mobile deep links, embedded/social wallets, hardware-specific adapters,
EIP-1271 and EIP-6492 are unsupported. ECDSA recovery does not prove absence of
contract code. This frontend does not classify accounts on-chain.

Authentication requires an existing ACTIVE verified EXTERNAL_EVM binding, an ACTIVE
identity and the explicitly configured ACTIVE tenant, chain 5042, MAINNET. Unknown,
inactive, revoked, unverified or ambiguous records receive generic failure UX. No
browser tenant/user claim is submitted, and no account is created or recovered.

## Authentication and consent

Start from the registered AI client's /oauth/authorize request. The Go service
validates client/redirect/resource/scope/S256 and creates the pending cookie before
303 redirecting to /onboarding. Opening the page alone does not create authority.

1. GET /browser/session obtains the server view and CSRF proof in memory.
2. Explicitly select a wallet and click Connect (eth_requestAccounts).
3. Select an exposed account; eth_accounts confirms exposure. eth_chainId must
   equal 5042 (0x13b2). Network switching/addition is never requested.
4. Explicitly request a challenge with {address}; inspect the exact returned text.
5. Explicitly Sign. Check account/network again, UTF-8 encode with TextEncoder and
   hex encode those bytes. personal_sign receives [messageHex, selectedAddress].
   No trimming, newline changes, prehashing, eth_sign, typed-data or fallback order.
6. Recheck account/network and expiry. Submit only challenge_id and signature.
7. GET /browser/session after verification obtains rotated CSRF. Require an
   AUTHENTICATED view, unchanged client/resource/read scope, and changed CSRF.
8. Separately Grant or Deny read-only mcp:read access. Only a server-returned safe
   HTTPS callback is used. Returning to the client does not prove token exchange.
   Denial retains existing local behavior and invents no callback.

Cookies remain HttpOnly, Secure, host-only __Host-wizpay-browser, Path=/,
SameSite=Lax. No cookie/token/signature localStorage or sessionStorage is used.
Signatures are transient function values, not application state or logs. Fetches
are same-origin, no-store, reject redirects and have a 15-second client deadline.
Backend authority and shorter transport/work deadlines remain authoritative.
Logout uses POST /browser/logout with CSRF and an empty body.

## Authentication correlation and pre-sign cancellation

GET /browser/session now includes the server-issued transaction_id. SIWE-derived
authenticated views additionally include challenge_id from immutable PostgreSQL
siwe_authentications evidence, joined to the consumed challenge and original OAuth
transaction. The repository locks/revalidates the current session, active client,
identity, tenant and binding version, then checks fresh database wall-clock expiry.
Neither reference is a credential; pending views expose no authentication proof.
No migration is needed. Legacy non-SIWE backend views retain optional-field
compatibility; the wallet frontend rejects authenticated views missing evidence.

The frontend retains the pending transaction ID and issued challenge ID. After
verification, both must match the refreshed authenticated view in addition to
client/resource/scope and changed CSRF. Same-client wallet or transaction replacement
therefore requires restart. Reconciliation only logs out an authenticated session
whose challenge AND transaction match this flow, or a pending session with the
original CSRF and transaction. Unrelated tab authority is never intentionally
adopted or revoked; a later cookie change causes server CSRF rejection.

The signing helper checks generation after each asynchronous account/network read
and immediately before personal_sign, with no intervening asynchronous boundary.
Disposal invalidates generation. Late provider responses cannot initiate signing.
Throwing provider getters, listener registration/cleanup and malformed rejection
values are contained without granting authority.

## Races and uncertain outcomes

Mutations are serialized. Account, chain, disconnect and provider changes invalidate
flow generations and suspend consent. The initial accountsChanged accompanying
first permission exposure is allowed only before any candidate account/challenge;
returned accounts are then cross-checked through eth_accounts. Later changes always
invalidate the flow, including manual network correction events: restart OAuth.

Late signatures are discarded. A wallet popup cannot be cancelled by AbortController.
HTTP timeout/unmount is not proof of server rollback. Verification failures, wallet
changes during verification, or failed post-verification refresh trigger fresh session
lookup and correlation-guarded, CSRF-guarded logout if possible, then require OAuth restart. Pending-session
logout also invalidates a racing finalizer through backend locking/revocation.
If cleanup itself fails, consent stays unavailable; server authority may remain until
expiry/revocation. No unconditional claim of successful logout is made.

Lost grant responses are never blindly replayed. Restart from the AI client. Multiple
tabs can rotate the shared host cookie: stale CSRF must fail; automatic account
substitution is not supported. Reloaded authenticated sessions display the server's
user identity, not an invented wallet address. Provider state is not login authority.

## Required local HTTPS routing (not deployed by this phase)

Use an owner-managed isolated development HTTPS edge for connect.wizpay.xyz:

- /onboarding and /_next/* go to the existing Next.js app.
- /browser/*, /oauth/* and /.well-known/oauth-authorization-server go to Go.
- Preserve canonical Host and browser Origin; strip untrusted Forwarded,
  X-Forwarded-Host and X-Forwarded-Proto before Go. Isolate upstream listeners.
- Pass Set-Cookie unchanged, including rotation/deletion. Never rewrite Domain.
- Disable sensitive caching; restrict body/time limits and redact sensitive logs.
- Do not introduce wildcard CORS or shared parent-domain cookies.
- Route mcp.wizpay.xyz/mcp independently, preserving Streamable HTTP behavior.

Existing configuration accepts no localhost SIWE fallback. No Next.js rewrite is
added: default rewrites may alter Host or introduce forbidden forwarding headers.
In an approved local browser test, inject a deterministic mock provider, exercise
connect/sign/consent with local fixtures, verify actual cookie rotation and CSRF,
Host/Origin rejection, and HTTP/1.1/HTTP/2 proxy deadline behavior. Never use real
wallets, production records, live Arc RPC or production authentication.

## Local validation

From web/approval-ui, using already-installed locked dependencies:

```sh
./node_modules/.bin/tsc --noEmit --incremental false
./node_modules/.bin/tsc --target es2020 --module commonjs --moduleResolution node \
  --esModuleInterop --skipLibCheck --strict --typeRoots node_modules/@types \
  --outDir /tmp/wizpay-wp3c-tests tests/onboarding.test.ts
node /tmp/wizpay-wp3c-tests/tests/onboarding.test.js
npm run build
```

Tests use Node's built-in runner and deterministic mocks: discovery/dedup/cleanup,
legacy fallback, permissions/accounts/network, exact UTF-8 and signing parameters,
rejection/expiry, stale signature results, rotation/fresh CSRF, cross-client refresh,
uncertain verification, guarded logout, serialization, separate grant/denial and
unsafe redirects. These prove controller/API behavior, not real proxy, extension,
React DOM or AI-host integration. The native node invocation prints individual tests.

From repository root:

```sh
GOPROXY=off WIZPAY_ARC_INTEGRATION=0 go test -count=1 ./internal/auth/... \
  ./internal/browser ./internal/http/browser ./internal/http/oauth ./internal/oauth \
  ./internal/requestauth/... ./internal/config
git diff --check
```

Public deployment still requires reviewed distributed rate limits, least-privilege
DB roles, retention/cleanup, CSP and proxy verification, registration compatibility,
independent security review and separate activation approval. First-time provisioning,
recovery and financial human-approval integration remain separate phases.

### WP3C-1 execution record

TypeScript noEmit and compiled native Node tests passed (18 tests). Focused Go
auth/JWT, browser, browser HTTP, OAuth service, request-auth and configuration
packages passed freshly. OAuth metadata/input/no-login/unsupported-deadline HTTP
tests passed separately. The complete OAuth HTTP package could not finish:
TestTokenBodyDeadlineOnConnection panicked because sandbox TCP listen is forbidden.
Next's default Turbopack build failed because its CSS worker could not bind a port
(Operation not permitted). No sandbox bypass or alternate deployment was attempted.
Build-generated .next artifacts were removed; manifests and lockfiles are unchanged.
Real HTTPS proxy/cookie/React DOM and injected-extension integration remain unverified.

### WP3C-2 remediation coverage

Deterministic frontend tests now cover account/chain/provider/disposal invalidation
while each pre-sign read is blocked, with zero personal_sign calls; unrelated
challenge/transaction and missing evidence; no logout of substituted authenticated
flows; and throwing provider interfaces/null rejection values. New Go unit/HTTP
tests cover additive session references and missing/revoked/expired evidence. The
PostgreSQL correlation regression covers consumed proof retrieval, rotated credential
rejection, same-client distinct transactions/challenges, independent logout and
existing concurrent authentication/replay tests. Real proxy/wallet interoperability
remains owner-run validation; production activation is unchanged.

Remediation local execution: TypeScript and 30 native Node tests passed. Focused Go
browser/browser HTTP/SIWE/OAuth/config/auth/request-auth tests and browser/SIWE/OAuth
race tests passed. go vet passed. PostgreSQL test compilation passed; execution was
blocked by Docker socket permission denial (TestMain), so no PostgreSQL transaction
pass is claimed. Next Turbopack build remained blocked by sandbox port binding.
The read-only default Go build cache was replaced with a disposable /tmp cache for
local validation; no dependency installation or sandbox elevation was performed.

Owner revalidation (repository root, isolated Testcontainers only):

```sh
GOPROXY=off go test -count=1 ./internal/storage/postgres -run 'TestSIWE|TestBrowser|TestOAuth'
GOPROXY=off go test -race -count=1 ./internal/storage/postgres -run 'TestSIWE|TestBrowser|TestOAuth'
WIZPAY_ARC_INTEGRATION=0 go test -count=1 ./...
go vet ./...
```

Repeat the frontend commands above, including npm run build, then perform the
separate final read-only security review. No production readiness is claimed.

### Power-outage recovery inspection

The recovered tree retained the full two-MEDIUM remediation, additive backend
correlation reader, Go unit/HTTP/PostgreSQL regression source, and the 30-test
frontend suite. No truncated or partially written source was found; the original
30 native tests passed freshly after recovery. Existing changes were preserved.

The remaining LOW cleanup edge was hardened: each provider selection owns an event
listener token. Cleanup invalidates that token before attempting removal. If a
provider throws from removeListener and retains a callback, it cannot invalidate a
replacement provider or a disposed controller. Three additional deterministic tests
cover failed removal, partial registration failure and throwing rejection/metadata
getters; all 33 native tests and TypeScript noEmit passed after recovery.

A fourth recovery regression covers a rejection object whose prototype lookup
throws: expiry/error classification is also guarded. Final recovery validation
passed 34 frontend tests, TypeScript, focused Go packages/race tests, selected OAuth
HTTP tests, PostgreSQL test compilation, go vet and diff checking. PostgreSQL
execution remains blocked by Docker socket permissions. The Next.js build was not
rerun during recovery; the earlier sandbox port-binding limitation and owner-run
HTTPS/browser validation requirements remain outstanding.
