# WP1: identity separation and human approval security

WP1 follows the owner's external-wallet-first staged hybrid decision. It prepares
identity and approval boundaries only. OAuth, SIWE, wallet onboarding, signing,
CAW execution, and autonomous financial dispatch are not implemented or enabled.
The CAW roadmap remains historical guidance; WP1 does not rewrite it.

## Independent identities

- `auth.Identity` has a stable internal user ID and authentication issuer/subject.
  Its historical `Provider()` accessor refers to the authentication issuer.
- `wallet.Binding.Provider()` identifies the wallet provider, independently of
  authentication. Provider user references remain opaque metadata, not credentials.
- New application-created intents snapshot both `Ownership.IdentityProvider`
  (authentication issuer) and `Ownership.WalletProvider` (wallet provider).
- Ownership checks still require the exact user, binding ID/version, provider,
  provider user reference, wallet ID/address, chain, network, and active binding.
- Wallet activation remains a metadata lifecycle operation. WP1 adds no ownership
  verifier and does not activate or fabricate evidence for any wallet.
- Signing/execution authority remains a separate, unavailable provider capability.
  Neither authentication nor a binding is financial authorization.

## Historical compatibility

An empty intent wallet-provider snapshot denotes the pre-WP1 coupled representation.
Only for that representation, its recorded authentication provider is also the
expected wallet provider. No fallback to a different provider is permitted.
The canonical ownership encoder omits this new field for legacy records, preserving
historical canonical bytes, digests, operation keys, and approval references.
New explicit snapshots include the wallet provider in the immutable digest.
Restoration rejects mismatched digests; it never upgrades an old snapshot.

Migration 007 adds the default-empty column without rewriting existing values.
It replaces authentication/wallet provider equality with tenant/user ownership
foreign keys, retains exact historical wallet-version relationships, independently
checks the recorded authentication issuer, and enforces the wallet provider through
an insert/update trigger. The new provider field is immutable even on drafts.
No wallet, user, binding ID/version, status, evidence, intent digest, or approval is
reassigned or reactivated. Invalid historical relationships abort migration.

The same forward migration corrects a pre-existing migration-005 constraint error:
its original lifecycle CHECK still rejected READY_FOR_EXECUTION_CONFIRMATION,
while its expiry CHECK had been removed. Migration 007 removes that duplicate
lifecycle CHECK, retains the updated lifecycle validation, and restores
`expires_at > created_at`. Existing inconsistent expiry data aborts migration.

There is no destructive down migration. Old binaries can still read unchanged
legacy records but fail closed restoring explicit-provider intent digests.
Once independent-provider records exist, the old equality constraints cannot be
safely restored. Use reviewed forward fixes or coordinated backup restoration;
never strip the new snapshot from financial records.

## Human-only authority

MCP `approval:request` permits requesting human review, not making the decision.
Human decisions require `approval:decide:human`; wallet handoff confirmation requires
`execution:confirm:human`. Both also require a separate human authentication context.

`auth.HumanAuthenticator` is the trusted port for a future secure browser boundary.
`AuthenticateHumanContext` invokes that port and binds its result to the existing
trusted tenant/user/issuer/subject/client/token identity. It accepts no human flag.
`RequireHuman` is enforced by HTTP actions, application services, and the permission
authorizer. Replacing request authority clears the human attestation.

WP1 supplies no production HumanAuthenticator and does not wire one into server
bootstrap. Thus authenticated MCP bearer tokens cannot decide approvals or call
`authorize-execution`, even if their verified permission claims contain the human
permission strings. These HTTP actions remain unavailable in every runtime mode
until a separately reviewed browser authentication boundary is implemented.
Only `_test.go` fixtures implement trusted human authenticators in WP1.

Decision and confirmation additionally reload the tenant/user-scoped immutable
intent and current wallet binding. Stale versions, changed providers, mismatched
owners, pending/revoked bindings, mismatched digests, and expired approvals fail
closed. A confirmation record is still metadata; it cannot sign or submit.

## Acting-agent authority

Privileged schedule creation derives the acting-agent ID from verified principal
`ClientID()`. A nonempty supplied agent ID must equal it. Request metadata cannot
supply the agent; absent verified client identity fails closed before grant lookup.
Schedule status changes also require the stored acting agent to match the verified
client. Existing delegation/grant bounds, revocation, and non-transitive semantics remain.
Legacy unrelated agent IDs are not silently mapped to clients. Future OAuth must
establish registered-client identity and reviewed client/agent relationships.
Autonomous dispatch remains disabled.

## Remaining prerequisites

- OAuth authorization server, token issuance, resource/audience separation, client
  registration, consent and revocation, and verified client/agent mapping.
- Secure human browser sessions, origin/CSRF protection, session freshness and
  revocation, and human-only permission issuance. Never adapt the MCP bearer
  verifier into the HumanAuthenticator port.
- SIWE challenge verification and EOA/EIP-1271 ownership evidence/freshness.
- Complete intent/policy/approval lifecycle orchestration and full financial review.
- Atomic approval decision/audit persistence and audited handoff decisions remain
  separate follow-up work; WP1 preserves existing audit behavior.
- Separately authorized provider/signing integration and production activation.

## Validation scope

Tests cover canonical legacy bytes/digests, provider-separated policy evaluation,
MCP denial despite human permission strings, trusted human scope/permission checks,
HTTP denial, stale/unusable bindings, agent impersonation, PostgreSQL relationship
constraints, and upgrading a populated pre-WP1 schema without rewriting intent or
approval identity. Existing idempotency/recovery/Mainnet-guard tests remain in place.
No local test proves production OAuth, wallet ownership, signing, or deployment.
