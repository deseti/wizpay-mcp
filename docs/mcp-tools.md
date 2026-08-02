# MVP MCP tool inventory and public contracts

## Contract conventions

- Tool names are stable, lowercase snake case, and provider-neutral.
- `READ_ONLY` means no external financial effect. An intent-creation tool may persist a record but cannot sign, submit, reserve, or move funds.
- `MONEY_MOVING` tools accept only `intent_id`, `approval_id`, and `idempotency_key` plus request metadata. Financial parameters cannot be restated or overridden at execution time.
- All objects reject unknown fields. IDs are opaque strings with documented prefixes; UUID idempotency keys are client-generated.
- `chain_id` is a decimal string, never an alias. EVM addresses are `0x` plus 40 hexadecimal characters and are validated/checksummed internally.
- `amount` uses `{decimal, base_units, decimals}`. `decimal` is a canonical human-readable decimal string; `base_units` is an unsigned integer string. Both must represent the same value exactly. Floating-point JSON numbers are forbidden.
- Tokens use `{chain_id, standard, address, symbol, decimals}`. Native assets use `standard: NATIVE` and omit `address`; ERC-20 assets require `address`. Symbols are display metadata, never authority.
- A recipient is `{recipient_id?, address, amount, memo?}`; each address and amount is explicit. No comma-delimited recipients.
- Every response includes `request_id`. Stateful objects include a server-issued ID and version.
- Formal inputs are in [public-tools.schema.json](schemas/public-tools.schema.json), successful outputs in [public-responses.schema.json](schemas/public-responses.schema.json), and errors in [error.schema.json](schemas/error.schema.json). The tables below define semantics.

## Tool inventory

| Tool | Purpose | Domain | Class | Expected input | Expected output | Approval | Intent type | Principal errors |
|---|---|---|---|---|---|---|---|---|
| `wallet_get_balances` | Read verified/observed token balances for the bound wallet | wallet/balance | READ_ONLY | `wallet_binding_id`, `chain_id`, optional token list (max 50) | wallet identity, per-token base-unit/decimal balances, observation block/time, verification status | none | none | authentication_required, wallet_not_bound, wallet_mismatch, unsupported_chain, provider_unavailable |
| `deposit_get_status` | Read status of a known inbound deposit | wallet/balance | READ_ONLY | `wallet_binding_id`, `chain_id`, transaction hash | observed transfer(s), confirmations/finality, verification status | none | none | wallet_mismatch, receipt_not_confirmed, receipt_verification_failed, provider_unavailable |
| `withdrawal_preview` | Validate a prospective withdrawal without creating an intent | wallet/balance | READ_ONLY | wallet binding, chain, token, amount, recipient | fees if known, balance sufficiency, warnings, expiry | none | none | validation_error, insufficient_balance, unsupported_chain, unsupported_token, route_unavailable |
| `withdrawal_create_intent` | Freeze a withdrawal plan for approval | wallet/balance | READ_ONLY | preview fields plus `client_request_id`, deadline | immutable intent summary/digest, policy result, `approval_required` | later execution | `WITHDRAWAL` | validation_error, wallet_mismatch, policy_denied, insufficient_balance, execution_conflict |
| `withdrawal_execute` | Resume/submit the approved withdrawal execution | wallet/balance | MONEY_MOVING | `intent_id`, `approval_id`, UUID `idempotency_key` | execution ID/state, existing-or-new submission observation | mandatory | `WITHDRAWAL` | approval_required, approval_expired, approval_rejected, execution_conflict, execution_pending, execution_failed |
| `payroll_preview` | Validate a bounded batch and calculate totals without creating an intent | payroll | READ_ONLY | wallet binding, chain, token, 1–500 recipients, deadline | normalized recipients, totals, known fees, warnings, preview expiry | none | none | validation_error, insufficient_balance, unsupported_token, route_unavailable |
| `payroll_create_intent` | Freeze a payroll batch and route | payroll | READ_ONLY | preview fields, client request ID, explicit route | immutable intent summary/digest, policy result, `approval_required` | later execution | `PAYROLL` | validation_error, wallet_mismatch, policy_denied, quote_expired, execution_conflict |
| `payroll_execute` | Resume/submit one approved payroll execution | payroll | MONEY_MOVING | references and idempotency key only | execution ID/state and batch verification progress | mandatory | `PAYROLL` | approval_required, approval_expired, execution_conflict, execution_pending, receipt_verification_failed |
| `swap_get_quote` | Obtain a bounded, non-executable swap quote | swap | READ_ONLY | wallet binding, chain, input/output tokens, exact input amount, max slippage bps, deadline | quote ID, amounts, route identifier, provider-neutral fee breakdown, expiry | none | none | unsupported_token, quote_expired, route_unavailable, provider_unavailable |
| `swap_create_intent` | Freeze quote, minimum output, and route for approval | swap | READ_ONLY | `quote_id`, wallet binding, client request ID, deadline | immutable intent summary/digest, policy result, `approval_required` | later execution | `SWAP` | quote_expired, wallet_mismatch, policy_denied, execution_conflict |
| `swap_execute` | Resume/submit one approved swap | swap | MONEY_MOVING | references and idempotency key only | execution ID/state, submission/settlement verification status | mandatory | `SWAP` | approval_required, quote_expired, execution_conflict, execution_pending, receipt_verification_failed |
| `bridge_get_quote` | Obtain a bounded bridge quote/plan | bridge | READ_ONLY | wallet binding, source/destination chain IDs, token, amount, destination recipient, deadline | quote/plan ID, route, fees, destination amount, expiry | none | none | unsupported_chain, unsupported_token, route_unavailable, provider_unavailable |
| `bridge_create_intent` | Freeze source and destination semantics for approval | bridge | READ_ONLY | `quote_id`, wallet binding, client request ID, deadline | immutable intent summary/digest, policy result, `approval_required` | later execution | `BRIDGE` | quote_expired, wallet_mismatch, policy_denied, execution_conflict |
| `bridge_execute` | Resume/submit one approved bridge execution | bridge | MONEY_MOVING | references and idempotency key only | execution ID/state, source and destination verification progress | mandatory | `BRIDGE` | approval_required, execution_pending, execution_failed, receipt_not_confirmed, receipt_verification_failed |
| `ans_check_name` | Check normalized ANS name availability and constraints | ANS | READ_ONLY | chain ID, Unicode input name | normalized name, availability observation, rules version/time | none | none | validation_error, unsupported_chain, provider_unavailable |
| `ans_get_record` | Read verified ANS ownership/records | ANS | READ_ONLY | chain ID, normalized name or node identifier | owner, resolver records, observation block/time, verification status | none | none | validation_error, unsupported_chain, receipt_verification_failed |
| `ans_create_registration_intent` | Freeze name, term, recipient/controller, cost, and route | ANS | READ_ONLY | wallet binding, chain ID, normalized name, term, controller address, route/quote, deadline | immutable intent summary/digest, policy result, `approval_required` | later execution | `ANS_REGISTRATION` | validation_error, wallet_mismatch, quote_expired, route_unavailable, execution_conflict |
| `ans_register` | Resume/submit one approved registration | ANS | MONEY_MOVING | references and idempotency key only | execution ID/state and ownership verification progress | mandatory | `ANS_REGISTRATION` | approval_required, execution_conflict, execution_pending, receipt_verification_failed |
| `transaction_get_status` | Read a known intent/execution without advancing it | transaction/status | READ_ONLY | exactly one of `intent_id` or `execution_id` | lifecycle state, safe timestamps, redacted submission and verification evidence, recovery guidance | none beyond ownership | none | authentication_required, authorization_required, validation_error, internal_error |

## Intent creation output

Every `*_create_intent` tool returns the same envelope:

```json
{
  "request_id": "req_...",
  "intent_id": "int_...",
  "intent_version": 1,
  "intent_digest": "sha256:<lowercase-hex>",
  "state": "approval_required",
  "expires_at": "RFC3339 timestamp",
  "policy_evaluation": {"decision": "ALLOW_WITH_APPROVAL", "policy_version_ids": []},
  "approval": {"required": true, "approval_request_id": "aprq_..."}
}
```

Returning this object does not authorize or initiate financial execution.

## Money-moving output and gate mapping

| Tool | Intent | Approval/policy | Executor boundary | Verification | Recovery identity |
|---|---|---|---|---|---|
| `withdrawal_execute` | WITHDRAWAL | digest-matched explicit approval; policy cannot bypass | wallet withdrawal port | expected sender/token/amount/recipient and final receipt | `execution_id` keyed by intent |
| `payroll_execute` | PAYROLL | digest-matched explicit approval; batch policy checked | payroll executor port | every expected transfer or allowlisted contract event plus totals | same batch execution and recipient item IDs |
| `swap_execute` | SWAP | digest-matched explicit approval and unexpired quote | swap executor port | input spent, minimum output received, allowlisted route, terminal settlement | same trade/submission identifier |
| `bridge_execute` | BRIDGE | digest-matched explicit approval and unexpired route | bridge executor port | source burn/lock and destination mint/release, linked by verified protocol evidence | same bridge message/transfer identifier |
| `ans_register` | ANS_REGISTRATION | digest-matched explicit approval and current availability checks | ANS executor port | expected name ownership/controller on the intended chain | same registration/commitment identity |

An executor must fail closed if its verifier contract is not implemented and its required artifacts are not `VERIFIED`.

## Error response

Tools return the standardized envelope in [errors.md](errors.md). Raw provider payloads, credentials, stack traces, signed transactions, and authorization material are never public error details.
