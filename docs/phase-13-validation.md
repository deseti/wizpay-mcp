# Phase 13 validation support

All commands below use deterministic in-memory stores or the repository's
ephemeral PostgreSQL integration container. They do not contact providers,
RPC endpoints, or production systems.

## Load and concurrency

```sh
go test -run 'TestConcurrentClaimsHaveOneWinner|TestGrantCapsAndIdempotentReservation' -count=20 ./internal/autonomy
go test -run TestAutonomyPersistenceClaimAndReservation -count=10 ./internal/storage/postgres
go test -race -run 'Autonomy|Claims|Reservation' ./internal/...
```

These tests exercise due scans, claim races, serializable spend reservations,
duplicate occurrence insertion, and emergency-stop persistence. The PostgreSQL
test uses an ephemeral test container and bounded fixture counts.

## Soak and restart/recovery

```sh
go test -count=100 ./internal/autonomy
go test -count=20 ./internal/storage/postgres -run 'TestAutonomyPersistenceClaimAndReservation|TestBackupRestore'
```

Restart behavior is represented by creating a second worker against the same
durable occurrence store after lease expiry. The occurrence key and fence are
preserved; retry processing never creates a new occurrence identity.

## Safe chaos/recovery cases

The safe chaos matrix uses fakes only:

| Injection | Expected result |
|---|---|
| worker stops after claim | lease expires; another worker claims the same occurrence with a higher fence |
| failure before dispatch gate | occurrence remains blocked/pending; no provider call |
| possible submission | reconciliation-only; no resubmission |
| temporary database error | worker returns/retries; financial identity is unchanged |
| emergency stop while queued | pre-dispatch gate returns `EMERGENCY_STOP` |
| delegation/grant revocation | pre-dispatch gate returns a stable denial reason |

No destructive production chaos command is provided. Provider/RPC behavior is
covered by existing deterministic execution-runtime and provider ambiguity
tests.
