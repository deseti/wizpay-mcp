# WP2: OAuth Authorization Code + PKCE foundation

WP2 adds an inactive real-user authorization foundation inside this repository.
It follows the external-wallet-first staged hybrid decision and leaves the CAW
roadmap intact. No login, SIWE, wallet onboarding, signing, financial execution,
CAW adapter or autonomous dispatch is added. The proposed domains are not claimed
as deployed.

## Protocol architecture

```text
AI MCP client -> mcp.wizpay.xyz/mcp
             -> protected-resource metadata / bearer challenge
             -> connect.wizpay.xyz authorization-server metadata
             -> /oauth/authorize: exact registered client + redirect + resource
             -> durable pending transaction + S256 challenge
             -> STOP: verified browser authentication/consent unavailable (503)

Future separately reviewed browser adapter
             -> existing ACTIVE tenant/user identity + exact transaction consent
             -> atomic session reference / consent / code digest / audit
             -> registered redirect with code, state and iss
MCP client    -> /oauth/token + code_verifier + exact client/redirect/resource
             -> atomic one-time code redemption / access-token digest / audit
             -> opaque MCP access token
             -> live PostgreSQL validation -> existing auth.TokenVerifier port
             -> persisted identity resolution -> TrustedRequest -> typed permissions
```

Canonical authorization-server issuer: `https://connect.wizpay.xyz`.
Canonical MCP resource/audience: `https://mcp.wizpay.xyz/mcp`.
These are exact constants, not caller-provided configuration or forwarded-header values.
The OAuth issuer is independent of the persisted user's authentication issuer and
wallet provider. Existing identities and historical financial snapshots are not rewritten.

## Supported routes

| Origin | Path | Behavior |
|---|---|---|
| mcp.wizpay.xyz | `/mcp` | Existing Streamable HTTP with OAuth authentication |
| mcp.wizpay.xyz | `/.well-known/oauth-protected-resource/mcp` | Path-specific RFC9728 metadata |
| mcp.wizpay.xyz | `/.well-known/oauth-protected-resource` | Root discovery alias for the same resource |
| connect.wizpay.xyz | `/.well-known/oauth-authorization-server` | RFC8414 discovery |
| connect.wizpay.xyz | `/oauth/authorize` | GET validation and pending state; stops at missing browser dependency |
| connect.wizpay.xyz | `/oauth/token` | POST form Authorization Code + S256 exchange |

There are no login, consent-completion, registration, introspection, public revocation,
JWKS, implicit, password, client-credentials or refresh endpoints. Internal completion
and revocation ports have no production implementations or HTTP callers.
The internal redirect builder includes the RFC9207 `iss` parameter and exact state.
Metadata advertises only the implemented public-client profile, S256 and `mcp:read`.
This is a constrained standards-aligned foundation, not certified full OAuth compliance.

## Registration and interoperability

Only pre-registered public clients with token endpoint authentication method `none`
are supported. Registration validates stable non-URL IDs, exact HTTPS redirect URIs,
allowed scope/resource and ACTIVE status. Client metadata and capabilities become
immutable; revocation is terminal. Changes require a new reviewed registration.
The migration seeds no clients. `Store.RegisterOAuthClient` is an internal owner/admin
port, never a public endpoint or MCP tool. Registry writes require an independently
reviewed administrative process; do not let clients choose capabilities.

CIMD is explicitly advertised false. No metadata URL fetching occurs, so there is
no CIMD SSRF path. DCR and confidential-client authentication are deferred and not
advertised. Current MCP recommends CIMD and also supports preregistration; selecting
preregistration is deliberately conservative and may limit host compatibility.

ChatGPT, Claude and Grok registrations, redirect URIs, refresh requirements and real
MCP interoperability are UNVERIFIED. No synthetic host-specific registration is
installed. Owner-reviewed official client requirements and authorized client tests
are required before claiming any integration. Without refresh, clients must repeat
authorization when the short-lived token expires; persistent connections are not proven.

## Scope and identity boundaries

Only `mcp:read` is accepted, mapping exactly to `intent:read` and `approval:read`.
It does not grant policy evaluation, intent creation, approval requests, preparation,
autonomy control, human decisions, confirmation or signing. Future `mcp:intent:create`
and `mcp:approval:request` scopes need separate review and are not advertised/accepted.
Existing services still enforce tenant/user scope. Client ID comes from the issued
record and registered-client relationship, not MCP inputs or request metadata.

WP1 remains intact: OAuth does not invoke `AuthenticateHumanContext`, implement
`HumanAuthenticator`, or set any human flags. Browser login and OAuth consent are
not transaction approval. Human financial endpoints stay fail closed.

## Durable state and revocation

Migration 008 adds clients, anonymous authorization transactions, non-secret session
references, consents, future consent-wallet relationships, authorization-code digests,
access-token digests and immutable reference-only OAuth audits.

- Transaction expiry: five minutes. Code expiry: at most two minutes, also bounded
  by transaction, verified session and principal expiry.
- Tokens: opaque 256-bit CSPRNG values, SHA-256 digest only in PostgreSQL. The code
  and access-token classes have distinct prefixes. Neither raw value, PKCE verifier,
  browser cookie nor signing secret is persisted or logged.
- Token lifetime: at most ten minutes, bounded by consent/session expiry.
- No signing keys/JWKS are required for this opaque profile. Token rotation means
  issuing a fresh random token through a new approved code; no refresh rotation exists.
- Redemption locks code, registry, consent, session and identity rows. The exact
  client, redirect, PKCE challenge and resource are checked before consumption.
  Code consumption, token insertion and issuance audit commit atomically.
- Consent stores the exact authentication issuer/subject. Identity changes cannot
  silently reinterpret the consent or token.
- Every bearer request reads live client/identity/consent/session/token eligibility.
  No authorization cache exists. Committed revocation applies to the next validation;
  already authenticated in-flight work is not cancelled. Future streaming/session
  strategies must preserve this bound. Current MCP transport remains stateless.
- Internal tenant/user-scoped revocation covers consent, session reference or token
  digest. There is no MCP or unauthenticated revocation endpoint. Future browser
  adapters must validate fresh sessions and CSRF, and the administrative process can
  terminally revoke a registry client.
- The consent-wallet table is empty. No binding/evidence is fabricated. Until a
  reviewed restriction mapper exists, any populated wallet restriction rejects
  both token issuance and access rather than being silently ignored.

Authorization, issuance and revocation audits store tenant/user/client/consent
references only. WP1's separate financial audit atomicity debt is not changed.

## HTTP and infrastructure boundary

`OAUTH_ENABLED=true` requires `AUTH_REQUIRED=true` and selects OAuth-only verification.
There is no legacy JWT fallback in that mode. Defaults remain unchanged (OAuth off).
Legacy JWT configuration remains a distinct compatibility mode and does not publish
OAuth metadata. Opaque mode does not load private/signing keys or legacy JWT keys.

Host matching is exact. Forwarded headers are not trusted. Absolute-form requests,
unregistered redirects, duplicate parameters/Authorization headers, cookie token
substitution and unexpected token grant fields are rejected. Token requests reject
browser Origin headers and HTTP Basic credentials; this server-side public-client
profile grants no token CORS permission. MCP Origin, when supplied, must be the exact
resource origin. Sensitive responses use no-store. Auth challenges include
resource_metadata and scope; insufficient scope uses 403.

HTTPS termination is an external deployment prerequisite. A reviewed proxy must
preserve canonical Host, reject untrusted public HTTP, strip misleading forwarding
headers, protect upstream listener access and redact authorization/token traffic.
Go accepts internal HTTP behind that isolated edge; it does not infer TLS from
X-Forwarded-Proto. Health/readiness remain infrastructure routes. WP2 modifies no DNS,
proxy, TLS, hosting or infrastructure and starts no application listener for tests.

Authorization/token endpoints have a bounded global per-process rate limit (120
requests/minute) and body/query limits. Multi-node distributed throttling, abuse
controls, pending-state retention/cleanup and capacity planning remain prerequisites.
Rate-limiter state does not authorize anything.

## Migration and deployment preflight

Existing applied migrations and financial tables are untouched. Forward migration
008 creates new tables and immutable authority/lifecycle triggers; no existing user,
wallet, approval, intent, digest or status is transformed. Session/code revocation
cannot be undone. No destructive rollback is supplied. Rolling code back leaves
new tables inactive; do not enable legacy tokens as an OAuth fallback.

Before any authorized deployment: confirm migrations1–7, migration007's documented
historical-data requirements, reviewed client registry, restricted database privileges,
OAuth audit sequence privileges, HTTPS origin/proxy isolation and durable backups.
Opaque validation requires PostgreSQL availability and fails closed on database errors.
No local check establishes production readiness.

## WP3 and activation prerequisites

WP3 must provide independently validated browser human authentication, SIWE challenge
and EOA/EIP-1271 evidence, origin/CSRF protection, session freshness and revocation,
explicit transaction-bound OAuth consent, and a safe state-bound browser continuation.
Never adapt an MCP bearer verifier or a static development header into the trusted
browser ports. Test fixture adapters exist only in `_test.go` files.

Wallet signing and Mainnet activation require later separate authorization/review.
OAuth scope additions, confidential clients, CIMD/DCR and refresh need independent
security work. No WP2 component authorizes autonomous funds or CAW credentials.

## Local validation

Use the existing Go dependencies and pinned sqlc v1.30.0; no new library is installed.
Standard-library crypto/rand, SHA-256 and base64 implement the opaque-token and RFC7636
primitives, not a new cryptographic algorithm. pgx provides atomic database operations.
The narrow protocol implementation still requires independent security review.

```sh
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 compile
WIZPAY_ARC_INTEGRATION=0 go test ./...
go vet ./...
git diff --check
```

Standard PostgreSQL tests use Testcontainers (`postgres:16-alpine`); report Docker
startup failures explicitly. An isolated socket-only PostgreSQL16 cluster and temporary
Go overlay replacing only TestMain container startup can validate the same tests.
Do not treat that alternative as a successful Testcontainers run. Use a fresh cluster
for each complete run because some existing fixtures create cluster-global roles.
Remove temporary clusters/overlays after validation. Never use a production DSN.

## Standards consulted

- [MCP authorization 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization)
- [RFC7636 PKCE](https://www.rfc-editor.org/rfc/rfc7636)
- [RFC8414 authorization server metadata](https://www.rfc-editor.org/rfc/rfc8414)
- [RFC9728 protected resource metadata](https://www.rfc-editor.org/rfc/rfc9728)
- [RFC8707 resource indicators](https://www.rfc-editor.org/rfc/rfc8707)
- [RFC9700 OAuth security BCP](https://www.rfc-editor.org/rfc/rfc9700)
- [RFC9207 issuer identification](https://www.rfc-editor.org/rfc/rfc9207)

## WP2 implementation validation record

Local validation during WP2 passed focused OAuth/HTTP/middleware/config/app tests,
`go vet ./...`, sqlc compile, byte-identical repeated sqlc generation, formatting,
`git diff --check`, focused Go race tests and PostgreSQL OAuth race tests. The final
fresh socket-only PostgreSQL16 full-suite overlay run passed, including the existing
financial identity/recovery/guard regressions. Twelve concurrent code redemptions
produced exactly one success.

An earlier expanded full-suite run failed the unchanged
`TestAutonomyClaimNextPreservesSameTenantOwner` expectation (`claim owners=map[owner_2:true],
want both users`). Its isolated fresh-cluster rerun and the final full-suite rerun
passed. The intermittent failure is recorded, not fixed or suppressed by WP2.
Standard Testcontainers validation remained blocked: WSL Docker was unavailable
(`checked path: $XDG_RUNTIME_DIR, failed to create Docker provider`). Initial HTTP
fixture failures were corrected to model origin-form inbound requests; application
absolute-form rejection was preserved. Live Arc/Circle checks were disabled.
Temporary cluster and overlay were removed; no production readiness is claimed.
