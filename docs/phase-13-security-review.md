# Phase 13 security-review checklist

This is a preparation artifact for an independent reviewer. It is not an
independent security review and does not authorize production launch.

## Review scope

- PostgreSQL tenant predicates, composite keys, unique occurrence identity,
  serializable reservations, lease/fencing updates, and restore behavior.
- Authenticated MCP schedule controls and permission separation between
  `autonomy:read` and `autonomy:control`.
- Principal, acting agent, delegation version/expiry/revocation, grant scope,
  wallet binding, and capability intersection.
- Pause/revoke/emergency-stop checks immediately before new dispatch.
- Existing approval binding, exact intent digest, approval expiry/rejection,
  and same-operation resume.
- Provider ambiguity and submission fencing; confirm that retries reconcile and
  never blindly resubmit.
- Recurrence timezone/DST handling, bounded missed runs, frequency and input
  bounds, and integer-only accounting.
- Logs, audit records, responses, migration contents, and changed-line secret
  scan.

## Self-audit findings

1. No private keys, seed phrases, signing shares, credentials, signed data, or
   raw provider payloads are added or persisted.
2. No arbitrary calldata, shell, script, generic transaction, Bridge, CCTP,
   or ANS autonomous surface is added.
3. Autonomous execution is disabled unless `WIZPAY_AUTONOMY_ENABLED=true` is
   explicitly supplied. Even then, the current worker processor blocks safely
   when typed Payroll/Swap planner and execution assembly are unavailable.
4. Occurrence and spend reservation uniqueness is enforced in PostgreSQL;
   cap checks run in serializable transactions.
5. Schedule status changes create a new version; historical rows are not
   rewritten.
6. Authentication derives tenant and actor from trusted request context; MCP
   fields cannot create a different authority context.
7. Security-sensitive service controls append the required audit record before
   applying the mutation and fail closed when the audit repository is absent or
   returns an error. The current repository interfaces do not provide one
   atomic schedule/audit transaction, so an audit-first record can remain as a
   harmless control-attempt record if the later mutation fails; a successful
   mutation cannot silently lack its required audit attempt.

## Required independent-review evidence

The separate reviewer must record test evidence, severity for each finding,
and explicit disposition. Production-ready status requires no unresolved
critical/high findings, verified backup/restore behavior, and confirmation
that production launch remains a separate human decision.
