# Phase 11 security and recovery review (Payroll + Swap scope)

**Status:** Review closure for the Phase 11 provider-plane corrective (Payroll + Swap only).  
**Repository HEAD basis:** post–`504305e` corrective work (reorg, health, circuit breaker, integration harness).  
**Live financial transactions:** not performed as part of this review.

## Scope covered

| Component | In scope |
|---|---|
| Circle User-Controlled Wallet boundary | Yes — initiate user challenge + reconcile only |
| Arc read-only verification boundary | Yes — receipt/head, chain ID `5042002` |
| Payroll contract primitives | Yes — encode/decode + registry allowlists |
| Swap contract primitives | Yes — encode/decode + registry allowlists |
| Provider health checks | Yes — non-financial probes |
| Circuit breakers | Yes — outbound Circle/Arc infrastructure |
| Reorg-aware observation handling | Yes — process-local receipt identity tracking |
| Sandbox/testnet integration harness | Yes — optional, offline by default |

## Explicitly deferred (project decision)

| Item | Status |
|---|---|
| Bridge | Deferred — not implemented |
| CCTP | Deferred — not implemented |
| ANS | Deferred — not implemented |
| Phase 12 business logic (payroll planning, swap quotes/routing, domain settlement success) | Deferred — not started |
| Capability enablement (Phase 10 remains disabled) | Deferred — availability separate |
| Mainnet | Deferred — not configured |

## Security assertions

| Assertion | Evidence |
|---|---|
| No WizPay private-key custody | No private-key types or local signers in provider/contract packages |
| No seed-phrase custody | Forbidden by architecture; not introduced |
| No MPC signing-share custody | Circle UCW only; shares never stored |
| No unilateral WizPay signing | Challenges require user authorization; backend never completes challenges |
| User authorization required | Missing user token → `USER_AUTHORIZATION_REQUIRED` |
| Provider submission ≠ verified success | Classification maps CONFIRMED/COMPLETE → submitted-pending; only Arc receipt at depth may verify |
| Reorg/ambiguity never triggers blind resubmission | Reorg → verification PENDING; submission-start marker forces `GetStatus` reconcile |
| No generic arbitrary contract execution | Sealed `EncodedCall`; typed Payroll/Swap encoders only |
| Admin ABI surface excluded | Canonical allowlists; admin functions not registered |
| Provider secrets redacted | `APIKey` / `UserAuthorization` redaction; health details contain no secrets |
| Capability availability remains disabled/separate | Defaults disabled; wiring claims only UCW + token transfer |

## Recovery assertions

| Assertion | Evidence |
|---|---|
| Durable submission-start marker | Phase 9 runtime `MarkSubmissionStarted` before `Execute` |
| Reconciliation after possible submission | `GetStatus` path; ambiguous classification is reconciliation-only |
| No duplicate execution after ambiguity | Same execution ID; no second `Execute` once marked |
| Reorg-aware observation handling | `ObservationTracker` plus durable receipt observation fields on adapter references (`bh`/`bn`/`cf`/`rp`) survive worker restart; present→missing retains last inclusion identity |
| Circuit breaker does not violate idempotency/recovery | Open breaker returns transient/open error; never invents a new execution or resubmits after ambiguity |

## Integration harness vs live evidence

| Item | Status |
|---|---|
| Offline unit/fake tests for health, breaker, reorg | Implemented and run in default CI |
| Optional Arc Testnet read-only harness (`WIZPAY_ARC_INTEGRATION=1`) | Implemented; **not executed** in this closure unless operators opt in |
| Optional Circle non-financial harness (`WIZPAY_CIRCLE_INTEGRATION=1` + API key) | Implemented; **not executed** in this closure (no credentials used) |
| Live production or money-moving tests | **Not performed** |

## How to run tests

```bash
# Default offline suite (required)
go test -count=1 ./...
go test -race -count=1 ./...

# Optional Arc Testnet read-only
WIZPAY_ARC_INTEGRATION=1 go test -count=1 ./internal/providers/arc -run TestArcTestnetIntegration

# Optional Circle non-financial (secrets via env only; never commit)
WIZPAY_CIRCLE_INTEGRATION=1 WIZPAY_CIRCLE_API_KEY=... go test -count=1 ./internal/providers/circle -run TestCircleSandboxIntegration
```

## Residual notes (not Phase 12)

- Domain planners remain nil in production worker assembly; the plane stays inert for money movement until Phase 12 supplies planners.
- Formal owner sign-off on UNVERIFIED Circle Arc Testnet product decisions in `docs/official-sources.md` remains an operator governance item, not a code gap in this corrective.

## Closure

For the **Payroll + Swap Phase 11 provider-plane hardening** items in this review (reorg, health, circuit breaker, integration harness, security/recovery documentation), the implementation is complete and validated offline. Bridge/CCTP/ANS and Phase 12 remain out of scope by decision.
