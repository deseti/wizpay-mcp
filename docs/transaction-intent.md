# Canonical immutable transaction intent

The normative structural schema is [transaction-intent.schema.json](schemas/transaction-intent.schema.json). This document defines canonicalization and semantics.

## Typed envelope

Every intent has:

- `intent_id`: server-generated immutable opaque ID.
- `intent_version`: schema/canonicalization version, initially `1`; it never changes in place.
- `owner`: internal authenticated `user_id` and identity-provider reference hash/opaque ID.
- `wallet`: binding ID, Circle user reference, Circle wallet ID, exact address, chain ID, and binding version.
- `operation_type`: one of `PAYROLL`, `SWAP`, `BRIDGE`, `ANS_REGISTRATION`, `WITHDRAWAL`.
- `financial`: a discriminated union keyed by `operation_type`, never arbitrary JSON.
- `route`: allowlisted route type and stable route identifier, with typed contract/provider references and optional quote/plan ID.
- `constraints`: deadline and domain-specific bounds such as minimum output, maximum slippage, or destination chain.
- `nonce`: server-issued monotonic owner/wallet scope string, plus a client request ID for intent idempotency.
- `created_at` and `expires_at`: UTC RFC 3339 timestamps.
- `intent_digest`: algorithm plus digest of the approval payload.
- `execution_state`: lifecycle projection only; not part of the approval digest and not permission to mutate the intent.

Amounts always contain exact human decimal and base-unit strings plus decimals. Token authority comes from chain ID, standard, and address/native designation, not symbol. Recipients are ordered typed records. Payroll total base units must equal the exact sum of recipient base units.

## Domain payloads

| Type | Required normalized fields |
|---|---|
| PAYROLL | input token; ordered recipient list; exact per-recipient amounts; exact total; batch memo hash if present |
| SWAP | input/output tokens; exact input; quoted/expected output; minimum output; max slippage bps; quote ID and expiry |
| BRIDGE | source/destination chain IDs; source token; exact source/destination amounts; destination recipient; bridge plan/message route and expiry |
| ANS_REGISTRATION | chain ID; normalized name; normalization rules version; term; controller/recipient; exact cost token/amount; registration route |
| WITHDRAWAL | token; exact amount; exact recipient; optional safe memo; withdrawal route |

## Freeze boundary

The record may be built in `draft`. Before the state becomes `intent_created`, the service validates and normalizes all approval-bound fields. At `intent_created`, every field listed below is immutable:

- intent ID/version, owner identity, wallet identity/address/binding version;
- operation type and every domain financial field;
- source and destination chain IDs;
- tokens, amounts, ordered recipients, controllers, and names;
- provider/contract route, quote/plan ID, fees represented in the plan;
- slippage/minimum output and every material constraint;
- created time, deadline/expiry, nonce, and client request ID;
- canonicalization version and digest algorithm.

Policy results, approval records, execution attempts, receipts, audit events, and lifecycle transitions are separate append-oriented records. They may advance, but cannot edit the intent.

Any material change—including a refreshed quote, new recipient, new wallet binding, changed deadline, or corrected amount—creates a new intent ID, new digest, and new approval.

## Canonical digest contract

1. Select the complete immutable approval payload defined by schema version.
2. Encode as RFC 8785 JSON Canonicalization Scheme bytes (UTF-8). No floats, duplicate keys, insignificant fields, or provider raw JSON are allowed.
3. Domain-separate with the UTF-8 prefix `WIZPAY_MCP_INTENT_V1\n`.
4. Compute SHA-256 and render `sha256:<64 lowercase hex characters>`.
5. Store the canonicalization version and digest with both intent and approval.

The digest is a binding/interface boundary in Phase 0, not a user signature. A later cryptographic approval mechanism must sign or otherwise authorize this exact digest without weakening user-controlled wallet authorization.

## State projection

`execution_state` is derived from the lifecycle record and excluded from the digest so state can advance without changing approved content. It cannot contain route overrides, amounts, or replacement recipients.

