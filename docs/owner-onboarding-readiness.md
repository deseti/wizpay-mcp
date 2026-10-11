# Owner-controlled real-wallet enrollment readiness

Baseline `bb6b7cb`, main. No deployment, migration, role creation, real enrollment,
wallet signing or public integration has been performed. Production SIWE remains
disabled and rejected by configuration. This is a bounded administration
foundation, not a completed real-wallet activation implementation.

## Unresolved trust decision — activation remains blocked

The existing login verifier proves possession of an EOA signing capability for a
server-issued, expiring, session-bound SIWE challenge. It does not establish the
application identity or explain why that wallet belongs to that identity. Today
an ACTIVE binding's nonempty verification_reference is metadata, not validated
identity evidence. Letting an administrator enter an arbitrary string and mark a
wallet verified would not resolve this boundary.

Owner approval is required for the identity authority, accepted identity evidence,
initial wallet-ownership verification ceremony, authorized reviewers and evidence
retention/revocation policy. In particular, decide whether an independently
reviewed offline enrollment authority is sufficient or a specific identity
provider must attest the relationship. No authority or provider is chosen here.
The administrative review reference below is only a change-ticket reference;
it is deliberately NOT a wallet-verification reference.

Therefore this change exposes no ACTIVATE/VERIFY operation, creates no verified
binding, and does not permit staging/production SIWE even with an administrator
approval flag. The production-capable activation path still needs implementation
after the policy decision. Do not change APP_ENV or insert ACTIVE fixture rows to
work around this gate. The existing reviewed chain profile (5042/MAINNET in this
repository) is retained without adding a network selector, provider call or
financial capability.

## Implemented lifecycle boundary

Migration 011 adds expiring per-login, per-tenant administrative delegations and
an immutable audit table. It seeds no users, roles or authority and grants no
public function access. The owner-only admin_onboarding function supports:

| Action | Result |
|---|---|
| STAGE | Creates an INACTIVE tenant if absent, a new UNKNOWN identity and PENDING EXTERNAL_EVM binding. No verification evidence or active authority. Existing identity/binding conflicts fail; this is not an update/reassignment API. |
| REVOKE_TENANT | Terminal tenant revocation. Subsequent SIWE issuance and existing SIWE-derived authority checks fail. |
| REVOKE_IDENTITY | Terminal identity revocation with lifecycle-version advancement. |
| REVOKE_BINDING | Terminal revocation of the exact tenant/user/binding, with version advancement and retained historical binding evidence. |

Every request has a unique tenant-scoped request ID, an opaque review reference,
and a deadline no more than 15 minutes ahead. Replay is rejected, including a
changed payload with the same ID. Repeating a terminal revocation also rejects;
inspect the protected audit to reconcile an uncertain result instead of assuming
a failed retry means the first operation failed. Request expiry and delegation
expiry are checked with database wall time after locks/state changes; a failure
rolls back both state and audit. Failed operations are not persisted as success.

Authorization uses PostgreSQL session_user, not caller JSON or SET ROLE. An
administrator can act only on explicitly delegated tenants and cannot read or
mutate tables directly, grant its own delegation, issue credentials, register
OAuth clients, or change financial records. Delegation removal/expiry blocks
future requests; an in-flight transaction holding the delegation lock may finish
before removal commits. No in-flight MCP operation cancellation is claimed.

Database triggers preserve immutable identity attribution and terminal identity
and wallet revocation. Existing session/consent/code/token revalidation remains
in force. Revocation retains immutable authentication evidence; it does not delete
financial history or rewrite credentials. The SIWE-derived checks do not redefine
legacy non-SIWE fixture authority; production bootstrap continues to install no
legacy browser verifier.

## Separate operator tooling, only after approval

The application runtime role and Compose profile are unchanged. Never give them
the administrative credential or delegation. cmd/onboarding-admin is a separate
operator binary and is excluded by the release Go image's explicit COPY list.
Build only after review, without activating the application:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go build -o deploy/release/runtime/onboarding-admin ./cmd/onboarding-admin
```

Migration execution remains a separately approved operation with the existing
migration owner. Before any operator use, validate migration 011 and least
privilege on a new disposable database, not the existing E2E DB. Older application
binaries will reject the new migration set; plan rollout compatibility explicitly.

Create a fresh dedicated LOGIN role using protected credential tooling:
NOSUPERUSER, NOCREATEDB, NOCREATEROLE, NOREPLICATION, NOBYPASSRLS, no memberships or
object ownership/privileges. Review ownership of SECURITY DEFINER functions and
deny untrusted CREATE in the public schema. The migration owner separately runs
deploy/release/onboarding-admin-role.sql with reviewed psql variables admin_role,
tenant_id and delegation_expires. Use a short finite delegation; the script
refuses runtime/migration logins and reused privileged roles. Removing its
onboarding_administrators row withdraws the delegation. Do not delete audit rows.

The tool requires ONBOARDING_ADMIN_APPROVED=yes and a regular mode-0600 (or more
restrictive) ONBOARDING_DATABASE_URL_FILE. No credential argument or fallback to
the application DATABASE_URL is supported. PostgreSQL TLS must verify the server
(verify-full, with approved CA/hostname); plaintext and insecure fallback configs
are refused. This is an additional private-host prerequisite; the existing
release PostgreSQL profile does not configure TLS for this operator connection.
Do not weaken TLS or expose the database publicly to make the tool connect.

Input is one bounded JSON object on stdin with action, request_id, tenant_id,
user_id, binding_id, issuer, subject, address, review_reference and expires_at.
STAGE requires all identity/binding metadata; revocations prohibit enrollment
fields and require only their exact target. Fields are opaque identifiers (128
characters maximum), except canonical checksum EOA address and UTC deadline.
Only nonsecret references belong in input. No signature, key, token, identity
document, free-form evidence or financial permission field is accepted. Output
is a fixed success/failure marker; driver errors and request contents are omitted.
Database/operator logging must likewise exclude parameters and credential files.

## Tests and remaining gates

Safe local unit suites cover request shape, disabled activation, SIWE signature
and challenge expiry rules, browser proof/rotation/revocation/consent, OAuth PKCE
and the two-tool read-only registry. The new PostgreSQL integration tests cover
pending-only creation, grants, replay, expiry, cross-tenant/user denial, duplicate
wallets, terminal revocation and invalidation of SIWE authentication.

The PostgreSQL package TestMain starts a Docker test database. Do not run that
suite under a no-container authorization. Compile-only validation is not execution
evidence. Owner-approved future isolated execution must cover the full package,
including existing replay/concurrent redemption, lock-wait expiry, tenant isolation
and token revocation regressions. No existing E2E container is needed or permitted.

Before Claude: resolve and implement genuine enrollment verification/activation;
validate DB migration/functions/grants under real non-owner credentials; review
durable storage, separate immutable images, backup/recovery and operator DB TLS;
register the actual public OAuth client and exact callback with mcp:read and
https://mcp.wizpay.xyz/mcp; retain Authorization Code + S256 and token method none;
then separately approve canonical HTTPS ingress and the precise wallet-signing
and read-only test. Only get_intent/get_approval may be exposed. No refresh token,
financial approval, autonomous action or transaction execution is introduced.
Account entitlement and actual Claude interoperability remain unverified.
