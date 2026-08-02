# Money-moving lifecycle contract

## States

| State | Meaning | Recoverability |
|---|---|---|
| `draft` | unapproved builder state; no immutable intent yet | replace/edit freely; no execution |
| `intent_created` | approval-bound payload validated, frozen, and digested | request approval or expire |
| `approval_required` | an approval request exists | approve, reject, cancel, or expire |
| `approved` | matching explicit approval granted and unexpired | queue exactly one execution |
| `execution_queued` | durable execution identity exists and job is queued | retry same job/execution |
| `executing` | worker owns a lease and is reconciling or preparing submission | resume/reconcile same execution |
| `submitted` | durable evidence indicates submission/provider acceptance | confirm; never resubmit blindly |
| `confirming` | receipt/settlement evidence is being observed | poll/reconcile only |
| `verified` | domain verifier proved evidence matches the immutable intent | finalize locally |
| `completed` | verified execution and local completion bookkeeping recorded | terminal success |
| `recovery_required` | outcome is ambiguous or automated retry cannot safely decide | recover same execution with operator-safe reconciliation |
| `failed` | proven terminal failure or verifier contradiction | terminal unless a domain-specific same-execution recovery path is explicitly defined |
| `cancelled` | cancelled before execution boundary | terminal; create a new intent if needed |
| `expired` | intent/approval deadline passed before execution boundary | terminal; new intent/approval required |
| `rejected` | explicit user rejection | terminal; new intent required |

## Allowed transitions

```text
draft -> intent_created
intent_created -> approval_required | cancelled | expired
approval_required -> approved | rejected | cancelled | expired
approved -> execution_queued | cancelled | expired
execution_queued -> executing | recovery_required
executing -> submitted | confirming | recovery_required | failed
submitted -> confirming | recovery_required | failed
confirming -> verified | recovery_required | failed
verified -> completed | recovery_required
recovery_required -> executing | confirming | verified | completed | failed
```

Self-transitions are forbidden. Repeated messages are idempotent no-ops that return current state; they do not append a false transition. Skipping forward is allowed only when reconciliation atomically records the missing evidence and each logical boundary—for example, a receipt first discovered after restart may record `submitted -> confirming -> verified`, never simply label a hash completed.

## Boundaries

- **Authorization boundary:** `approval_required -> approved` requires a valid, unexpired, unrevoked explicit user approval whose owner, wallet, intent ID, version, and digest match exactly. MVP policy never performs this transition.
- **Execution boundary:** durable creation of `execution_id` and `execution_queued`. After this point cancellation/expiry cannot imply that external action was stopped; reconciliation is mandatory.
- **Provider submission boundary:** a provider call or transaction broadcast may occur only while owning the execution lease and only after an atomic submission-attempt record exists.
- **Receipt verification boundary:** `confirming -> verified` requires domain-specific verified evidence. Provider success, a transaction hash, or receipt existence alone is insufficient.
- **Completion boundary:** `verified -> completed` is local bookkeeping only and cannot precede verification.

## Terminal and recoverable states

Terminal: `completed`, `cancelled`, `expired`, `rejected`, and normally `failed`. Recoverable: `execution_queued`, `executing`, `submitted`, `confirming`, `verified`, and `recovery_required`. `approved` is resumable before execution creation.

## Invalid transitions

All unlisted transitions fail closed with an internal transition-conflict record. Especially invalid:

- any pre-approval state directly to execution;
- `approved` after rejection, cancellation, or expiry;
- any execution state back to approval/draft;
- `submitted` directly to `completed` without verified evidence;
- changing execution identity during retry/recovery;
- treating policy allow as approval;
- cancelling an ambiguous/submitted execution as proof of non-execution.

