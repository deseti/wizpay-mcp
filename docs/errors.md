# Standard public error model

## Envelope

```json
{
  "error": {
    "code": "approval_required",
    "message": "Explicit approval is required for this intent.",
    "request_id": "req_...",
    "retryable": false,
    "user_action_required": true,
    "terminal": false,
    "safe_details": {"intent_id": "int_..."}
  }
}
```

`message` is safe and provider-neutral. `safe_details` is allowlisted per code and may contain opaque record IDs, field paths, retry-after seconds, and current state only. It must not contain secrets, authorization credentials, raw provider payloads, signed material, stack traces, SQL, or internal network details. HTTP/MCP transport status is mapped separately and never replaces `code`.

## Taxonomy

Retryability is a default hint; callers must honor `retry_after_seconds` and query status before retrying money-moving requests. `terminal` means terminal for the current intent/execution, not that a corrected new intent is forbidden.

| Code | Meaning | Retryable | User action | Terminal |
|---|---|---:|---:|---:|
| `validation_error` | Input violates schema or semantic limits | no | yes | no |
| `authentication_required` | No valid application user session | no | yes | no |
| `authorization_required` | Principal cannot access the resource/action | no | yes | yes |
| `identity_not_found` | Resolved identity is not established/active | no | yes | no |
| `identity_suspended` | Identity is temporarily ineligible to authorize | no | yes | no |
| `identity_revoked` | Identity is permanently revoked | no | yes | yes |
| `intent_not_found` | Referenced intent does not exist or is not visible | no | yes | yes |
| `intent_expired` | Intent expired before reaching the next pre-execution gate | no | yes (new intent) | yes |
| `intent_mutated` | Material fields differ from the frozen intent digest | no | yes (new intent/approval) | yes |
| `approval_required` | Matching explicit approval does not exist | no | yes | no |
| `approval_not_found` | Referenced approval does not exist or is not visible | no | yes | yes |
| `approval_expired` | Approval or its intent expired before consumption | no | yes (new intent/approval) | yes |
| `approval_rejected` | User rejected the approval request | no | yes (new intent) | yes |
| `approval_already_consumed` | Approval is already reserved for its logical operation | no | no | yes |
| `policy_not_found` | Referenced policy does not exist or is not visible | no | yes | yes |
| `policy_invalid` | Policy structure, rule, scope, or lifecycle data is invalid | no | yes | yes |
| `policy_denied` | Active policy forbids the proposed intent | no | yes | yes |
| `policy_expired` | Policy is past its authorization window | no | yes | yes |
| `policy_disabled` | Policy is not active and cannot authorize | no | yes | yes |
| `review_required` | Policy requires an explicit review decision | no | yes | no |
| `wallet_not_bound` | User has no active verified binding | no | yes | no |
| `wallet_mismatch` | User, wallet ID/address, chain, or binding differs | no | yes | yes |
| `wallet_revoked` | Wallet binding is terminally revoked | no | yes | yes |
| `unsupported_chain` | Chain is not allowlisted | no | yes | yes |
| `unsupported_token` | Token identity/configuration is not allowlisted | no | yes | yes |
| `insufficient_balance` | Observed spendable balance is too low | conditional | yes | no |
| `quote_expired` | Quote/route deadline elapsed | no | yes (new quote/intent) | yes |
| `route_unavailable` | No policy-compliant verified route exists | conditional | no | no |
| `execution_conflict` | Idempotency/intent references conflict or another execution owns the intent | no | yes if key misuse | yes for conflicting request |
| `execution_pending` | Same execution exists and is not terminal | yes (status only) | no | no |
| `execution_failed` | Domain execution reached a proven failure | condition-specific | maybe | condition-specific |
| `receipt_not_confirmed` | Submission observed but required confirmation/finality absent | yes (poll/status) | no | no |
| `receipt_verification_failed` | Evidence contradicts approved intent or cannot satisfy verifier | no | yes/support | yes |
| `provider_unavailable` | Required provider/RPC temporarily unavailable | yes | no | no |
| `rate_limited` | Caller exceeded a limit | yes | no | no |
| `internal_error` | Safe unexpected failure | conditional | no | no |

## Rules

- Authentication failures do not reveal whether an intent or wallet exists.
- Validation errors identify safe JSON field paths, never accepted hidden values.
- A changed idempotency key does not override an existing intent execution.
- Ambiguous provider/RPC outcomes become `execution_pending` or `recovery_required` internally, never `execution_failed` unless non-submission/failure is proven.
- A transaction hash alone never changes an error to success.
- Internal diagnostics use a restricted correlation ID linked to redacted audit records.
