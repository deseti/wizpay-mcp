# Idempotency, duplicate prevention, and recovery

## Core invariant

```text
same intent + any retry = same financial execution
```

Changing an idempotency key never creates a second execution for the same intent. Changing material financial data creates a new intent and approval, not a retry.

## Idempotency layers

| Layer | Stable key/identity | Required behavior |
|---|---|---|
| intent creation | `(owner_id, client_request_id)` | exact replay returns same intent; different canonical payload returns `execution_conflict` |
| approval | `(intent_id, intent_digest)` | one effective decision; contradictory/replayed artifacts fail closed |
| execution | unique `intent_id -> execution_id` plus client idempotency key | atomically create once; all retries load it |
| provider | deterministic provider key derived from execution ID and route version | reuse the same key/request; persist provider operation ID immediately |
| worker | execution ID + attempt number under lease/fencing token | duplicate delivery reconciles; never invents execution |
| receipt polling | execution/submission ID + observation cursor | GET/read only; observations are append/upsert idempotent |
| recovery | same execution, provider operation, tx hash/message ID | reconcile external truth; never restart as new payment |

PostgreSQL will be authoritative. Unique constraints and compare-and-swap transitions are required. Redis locks are advisory acceleration only; loss/duplication cannot authorize submission. River delivery is at-least-once and must be harmless under duplicate delivery.

## Submission protocol

1. Load immutable intent, exact policy evaluation, approval, and wallet binding under authoritative checks.
2. Atomically create/load the single execution and a numbered submission attempt with deterministic provider idempotency key.
3. Acquire a bounded lease with fencing token; stale workers cannot persist later state.
4. Reconcile provider/chain state before any submission attempt.
5. Submit only if non-submission is proven and the adapter supports safe deterministic replay.
6. Persist provider operation ID/transaction hash and response fingerprint; transition to observation states.
7. Poll/read and verify. Never re-submit merely because a response was lost.

Adapters that cannot provide deterministic idempotency or reliable reconciliation are ineligible until an owner-approved compensating design exists.

## Crash matrix

| Crash point | Required restart behavior | Forbidden behavior |
|---|---|---|
| before provider submission | load attempt; prove no submission; resume same execution under a new lease | create another execution |
| after request may have left process but before response/persistence | mark/reenter `recovery_required`; query by deterministic provider key, wallet/nonce, route, and bounded time; manual reconcile if inconclusive | blind resubmission or mark failed |
| after provider submission/operation ID but before local persistence | use provider idempotency lookup and submission-attempt fingerprint; persist recovered ID | new provider request with a new key |
| after transaction hash but before receipt | persist/rediscover hash; poll receipt and chain finality; do not submit | treat hash as success or replace transaction silently |
| after receipt but before verification | reload receipt from trusted sources; run domain verifier against intent | infer success from provider status |
| after verification but before local completion | idempotently persist verification evidence and move `verified -> completed` | repeat financial execution |
| worker duplicate delivery | one worker wins lease/fence; others return current state | parallel submissions |

## Ambiguity and recovery

Ambiguity is a safety state, not failure. `recovery_required` records safe reason code, last known execution/submission identifiers, next reconciliation method, and operator escalation. Recovery can advance only the same execution. If reliable evidence never establishes outcome, it remains unresolved; the system must not create a compensating or replacement payment automatically.

Provider responses are fingerprinted and minimally retained. Receipt verification should use multiple trustworthy observations when the domain risk requires it, record block/confirmation/finality context, and prove expected chain, sender/wallet, recipient(s), token(s), amounts, contract/function/event, quote/plan linkage, and success semantics.

