# Data classification, retention, and audit model

## Classes

| Class | Examples | Handling |
|---|---|---|
| `PUBLIC` | published tool names, schemas, public chain IDs/addresses | normal integrity controls |
| `INTERNAL` | route identifiers, non-sensitive configuration, aggregate metrics | authenticated staff/service access; no public error leakage |
| `CONFIDENTIAL` | user IDs, wallet metadata, addresses linked to identity, intents, approvals, receipts, provider IDs | encrypt in transit/at rest, least privilege, purpose limitation, audited access |
| `HIGHLY_SENSITIVE` | service API credentials, session material, OAuth secrets, webhook secrets, restricted approval evidence | secret manager, narrow runtime access, never logs/audit/tool output, rapid rotation |
| `FORBIDDEN_TO_STORE` | private keys, seed phrases, signing shares/key shards, PINs, OTPs, entity secrets granting unilateral user signing, raw signing authorization credentials | reject/redact immediately; never persist, log, audit, cache, or place in jobs |

## Record policy

Concrete retention durations require legal/security owner approval. Until then, production collection is disabled and the default is minimize, classify, and define a deletion trigger before implementation.

| Record | Class | Minimum content | Retention/deletion contract |
|---|---|---|---|
| application logs | INTERNAL to CONFIDENTIAL | request ID, safe event/state/reason; pseudonymous IDs | short operational window; redact at source; no raw request/provider bodies |
| audit records | CONFIDENTIAL | event envelope and redacted resource references | append-oriented security/financial retention set by owner/legal; access audited |
| intents | CONFIDENTIAL | canonical immutable payload/digest and lifecycle reference | retain for idempotency, dispute, and regulatory period; controlled deletion/tombstone policy |
| approvals | CONFIDENTIAL; evidence may be HIGHLY_SENSITIVE | decision, digest binding, timestamps, safe method/evidence reference | aligned with intent/audit; never store PIN/OTP/token/signing secret |
| wallet metadata | CONFIDENTIAL | binding IDs, Circle refs, address/chain, verification/status | active binding plus required audit period; revoke does not erase audit truth |
| provider responses | CONFIDENTIAL, possibly HIGHLY_SENSITIVE | normalized fields, status, external ID, response hash | avoid permanent raw payloads; short quarantine only if incident need is approved |
| transaction receipts | CONFIDENTIAL (chain data itself may be public) | normalized receipt/log evidence and chain observation metadata | aligned with execution/audit verification period |
| request IDs | INTERNAL; CONFIDENTIAL when linkable | random opaque correlation ID | same as associated log/audit record; never encode identity/secrets |
| policies | CONFIDENTIAL | versioned constraints and status | active plus historical versions needed to explain decisions |
| Redis cache/locks | INTERNAL/CONFIDENTIAL transient | opaque IDs, safe cached reads, lease data | TTL mandatory; no approval authority or forbidden material |
| durable jobs | CONFIDENTIAL | record IDs, operation kind, attempt metadata | delete per completed-job policy; never duplicate mutable payment payloads |

Data subject access/deletion, regulatory retention, backup expiry, geographic residency, and audit retention periods are `UNVERIFIED / REQUIRES OWNER DECISION`.

## Audit envelope

Audit events are append-oriented and immutable. Corrections append a new event referencing the prior event. Each contains:

- `audit_event_id`, `event_type`, schema version, UTC timestamp;
- request/trace ID; actor type and pseudonymous actor ID;
- resource type and opaque resource ID;
- previous/new lifecycle state when relevant;
- safe reason/result code and policy/version references;
- tamper-evidence fields such as previous-event hash and event hash (mechanism owner-approved later);
- source component and sanitized metadata allowlist.

Events never contain credentials, access/user tokens, PIN/OTP, private/signing material, full raw provider payloads, signed transactions, secrets, or unnecessary personal data.

## Required event types

| Area | Events |
|---|---|
| identity/wallet | `identity_authenticated`, `authentication_failed`, `wallet_binding_requested`, `wallet_bound`, `wallet_binding_failed`, `wallet_revoked`, `wallet_mismatch_detected` |
| intent | `intent_created`, `intent_cancelled`, `intent_expired`, `intent_conflict_detected` |
| approval | `approval_requested`, `approval_granted`, `approval_rejected`, `approval_expired`, `approval_revoked`, `approval_consumed`, `approval_mismatch_detected` |
| policy | `policy_created`, `policy_updated`, `policy_revoked`, `policy_evaluated`, `policy_denied` |
| execution | `execution_queued`, `execution_started`, `provider_submission_started`, `provider_submission_observed`, `transaction_submitted`, `execution_conflict_detected` |
| verification/recovery | `receipt_observed`, `receipt_verified`, `receipt_verification_failed`, `recovery_required`, `recovery_resumed`, `execution_completed`, `execution_failed` |
| security/operations | `rate_limit_exceeded`, `authorization_denied`, `secret_redaction_triggered`, `configuration_changed` |

Audit write failure at or after an authorization/execution boundary fails closed or places the same execution in `recovery_required`; it must not silently proceed.

