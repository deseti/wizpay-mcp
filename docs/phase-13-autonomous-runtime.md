# Phase 13 — Autonomous agent runtime

Phase 13 is the final numbered phase in the WizPay MCP roadmap. There is no
Phase 14. Work after this phase is maintenance, security review, and release
operations, not another numbered implementation phase.

## Safety boundary

Autonomy is a control-plane orchestration layer above immutable intents,
approvals, policies, capability availability, execution idempotency, provider
reconciliation, and domain verification. Circle User-Controlled Wallet remains
the custody model. WizPay stores no private keys, seed phrases, signing
shares, recovery secrets, or model-visible Circle credentials.

The executable scope is PAYROLL and SWAP only. Bridge, CCTP, ANS, arbitrary
calldata, generic transaction execution, treasury routing, and unilateral
WizPay signing are not introduced. Provider capability is fail-closed; a
schedule never enables an unavailable provider. Production launch remains a
separate explicit human decision and requires independent security review with
no unresolved critical/high findings.

## Runtime model

`internal/autonomy` owns schedules, immutable schedule versions, occurrences,
principal/delegation context, autonomous grants, emergency-stop state, bounded
recurrence, deterministic occurrence keys, scheduler ports, simulation, and
spend reservations. Schedules contain a typed intent kind and a digest of typed
intent-template material. They cannot contain executable calldata or scripts.
Every occurrence is keyed by `schedule ID + schedule version + UTC scheduled
instant`; the database uniqueness constraint is the second line of defense
against duplicate operations.

Supported recurrence is ONCE, DAILY, WEEKLY, and MONTHLY with an explicit IANA
timezone. Occurrences are materialized in UTC; identity never reads the wall
clock. Schedule edits create a new version. Missed runs are explicit: SKIP
creates none, while RUN_LATEST creates only the newest due occurrence, never an
unlimited backlog.

Workers claim due occurrences with a lease and fencing token. The only
supported concurrency policy is FORBID_OVERLAP. Retries reuse the same
occurrence and execution identity. After external submission may have begun,
the runtime is reconciliation-only and never blindly resubmits.

## Grants and delegation

An active, user-authorized grant is required for every autonomous occurrence.
Grants bind principal, wallet binding, intent kind, optional schedule, expiry,
pause/revocation, per-action and aggregate caps, rolling-window caps, and
recipient/token/chain allowlists. All accounting uses decimal integer base
units. Reservations are keyed by grant and occurrence, made atomically, and
may be released before legitimate commitment. Repeated processing is
idempotent.

Each step carries tenant, human principal, acting agent, delegation version,
effective capability, wallet binding, and grant. Delegations expire, can be
revoked, are capability scoped, and are non-transitive by default. Effective
authority is the intersection of principal authorization, delegation, grant,
and capability requirements.

The final pre-dispatch gate rechecks runtime enablement, emergency stop,
schedule status, grant status/expiry, wallet binding, and delegation. Pause,
revocation, and emergency stop prevent new dispatches. An already-submitted
operation remains in reconciliation.

Schedule and occurrence records bind exact grant and delegation versions.
Revoked schedules are terminal. Worker claims use the exact row selected under
`FOR UPDATE SKIP LOCKED`, and dispatch fencing requires the current lease owner,
lease expiry, and monotonic fence. Security-sensitive controls fail closed if
their required audit write fails.

## Simulation, audit, and safe status

Simulation evaluates schedule, delegation, grant, allowlists, caps, step-up,
and runtime availability without reserving spend, changing lifecycle state,
creating approvals/challenges, or calling providers. Status explanations use
stable reason codes and never expose provider bodies, credentials, signatures,
or unrestricted internal errors. Autonomous audit events preserve principal →
agent/delegation → schedule/version → occurrence → intent → grant/approval →
execution → verified-evidence attribution.

The application contract is split into read operations (list/get schedules,
simulate, status, utilization) and controls (create/version, pause/resume/
revoke, emergency stop). Controls use the existing authenticated authorization
context and never provide signing or arbitrary execution APIs. The default
process control is `WIZPAY_AUTONOMY_ENABLED=false`.

## Production hardening

Initial controls bound schedules per tenant/principal, active schedules,
recipients, frequency, policy limits, and input sizes. Rate-limit integration
belongs at the authenticated application boundary. Safe anomaly signals include
denial and step-up spikes, failure/recovery rates, spend velocity, due latency,
reconciliation backlog, and verification delay. Actions are alert, block, or
step-up only; anomaly detection never expands authority.

Initial SLOs: 99% of enabled due occurrences claimed within 60 seconds (page at
5 minutes); ordinary processing reaches terminal or reconciliation state
within 10 minutes (page at 15 minutes); any emergency-stop/revocation
enforcement discrepancy pages; recovery/provider-reconciliation queues older
than 15 minutes page; verification older than 30 minutes pages; and a worker
heartbeat older than two lease durations pages.

PostgreSQL backups must include autonomy tables, constraints, and audit rows;
secret material is excluded. Restore verification runs migrations/checksums,
restores a staging copy, confirms unique occurrence keys and reservation
states, and runs scheduler recovery before traffic resumes. Target assumptions
are RPO 5 minutes and RTO 30 minutes, subject to the database contract. After
restore, pause autonomy, reconcile claimed/submitted occurrences, verify
occurrence/execution identity mapping, then resume only under controlled
rollout.

Runbooks cover pause-all, safe resume, provider/RPC outage, stuck
reconciliation, policy incident, suspected duplicate, database restore,
credential-compromise response, and staged rollout. Chaos is limited to
deterministic local fakes (worker crash, lease expiry, restore, provider
ambiguity, and reordering); it never targets production or performs a financial
transaction. Load/soak workloads are bounded local PostgreSQL fixtures.

## Completion status

The concrete PostgreSQL repositories, authenticated schedule-control MCP
surfaces, step-up coordinator, durable fail-closed worker wiring, migration
integration tests, and deterministic validation support are implemented.
Autonomous execution remains blocked when typed Payroll/Swap planning and the
existing execution assembly are unavailable; enabling the flag does not
override that boundary. Final acceptance still requires the full validation
suite and an independent security review with no unresolved critical/high
issues. Phase 13 remains incomplete until all implementation and validation
gates, including that independent review, pass. Those are release gates, not a
new roadmap phase.
