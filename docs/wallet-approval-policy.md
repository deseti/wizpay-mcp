# Wallet binding, approval, and policy contracts

## Circle user-controlled wallet binding

Circle documents user-controlled wallets as user-authorized: the application initiates a request and the user authorizes signing after authenticating. WizPay MCP therefore stores reference metadata only and never stores a private key, seed phrase, signing share, PIN, OTP, Circle user token, encryption key, or equivalent signing credential.

The normative binding schema is [wallet-binding.schema.json](schemas/wallet-binding.schema.json). A binding includes:

| Field | Contract |
|---|---|
| `binding_id` / `version` | internal immutable identity/version |
| `user_id` | authenticated WizPay MCP owner |
| `circle_user_ref` | opaque application-to-Circle user reference; not an auth token |
| `circle_wallet_id` | Circle wallet identifier |
| `wallet_address` | exact chain address |
| `chain_id` / `blockchain` | explicit numeric chain identity plus Circle network identifier if verified |
| `account_type` | `EOA`, `SCA`, or `UNVERIFIED` only when established |
| `binding_status` | `PENDING`, `ACTIVE`, `REVOKED` |
| `verification_status` | `UNVERIFIED`, `VERIFIED`, `FAILED`, `STALE` |
| timestamps | created, verified, and revoked timestamps as applicable |
| `evidence_ref` | restricted reference to verification evidence, not raw credentials |

Activation requires a future verified Circle lookup/challenge flow and proof that user, Circle user reference, wallet ID, address, blockchain, and chain ID agree. Phase 0 does not define that runtime flow. Until it is verified and implemented, bindings remain unusable for money movement.

Mismatch is fail-closed: do not auto-rebind, infer from address alone, or substitute another wallet. Emit `wallet_mismatch`, audit the compared identifiers in redacted form, revoke/stale a compromised binding where authorized, and require fresh binding plus a new intent and approval.

## Approval artifact

The normative schema is [approval.schema.json](schemas/approval.schema.json). It binds:

- `approval_id`, `approval_version`, `approval_request_id`;
- exact `intent_id`, `intent_version`, and `intent_digest`;
- exact user ID, wallet binding ID/version, wallet ID/address, and chain ID;
- decision `APPROVED` or `REJECTED`;
- creation/decision/expiry timestamps;
- authorization-method identifier and restricted evidence reference;
- revocation and consumption status/timestamps.

MVP invariant: exactly one explicit user approval is required for every money-moving intent. Approval is not transferable, editable, or wildcarded. Approval of one payroll batch cannot approve another batch or a retry with a different execution identity.

An approved artifact may be revoked only before the execution boundary. Consumption atomically creates or attaches to the one execution identity. After consumption, revocation cannot represent rollback; status/recovery governs the same execution. Rejected, expired, revoked, or digest-mismatched approvals are terminal for that intent.

This artifact is conceptual in Phase 0 and is not itself a blockchain signature. A future approval UI must display the canonical financial summary and must not hold signing secrets.

## Policy model

The normative schema is [policy.schema.json](schemas/policy.schema.json). Policies are versioned, scoped to an owner and optional wallet binding, and may constrain:

- allowed operation types, chain IDs, and exact token identities;
- per-transaction and rolling daily base-unit limits;
- exact recipients or governed recipient-list versions;
- payroll maximum recipients and recipient restrictions;
- maximum swap slippage basis points and allowed routes/pairs;
- bridge destination chains/recipients/routes;
- ANS name/term/cost constraints;
- validity interval and policy status.

Policy evaluation is deterministic over the immutable candidate intent and recorded as `ALLOW_WITH_APPROVAL` or `DENY`, with policy/version IDs and safe reason codes. Policy evaluation must occur before approval request and again before execution to detect revocation or stricter policy, but it cannot alter the intent.

During MVP, an allow result means only that approval may be requested. It never bypasses explicit per-intent approval. Policy conflicts fail closed. Future autonomous execution requires a separate owner-approved architecture and is outside this contract.

