# Circle UCW SEND: local validation

This harness validates a proposed EOA signing boundary offline. It does not integrate a Circle API, prove Arc Mainnet provider compatibility, or activate financial execution. Circle `Configured()` remains false; production adapter construction and provider wiring remain unavailable. No roadmap phase is added.

## Existing architecture

The harness uses `internal/wallet/binding.go`, frozen `internal/intents` values, `internal/policies` evaluation, explicit `internal/approvals`, deterministic `internal/execution` requests, and the existing `internal/send/planner.go` and transfer calldata decoder in `internal/send/verifier.go`. It does not add MCP tools, arbitrary calls, wallet custody, or production execution wiring.

`internal/send/signed_transaction.go` adds a pure, unwired EOA envelope validator. It checks the approved SEND plan and current active verified binding, decodes canonical transaction bytes, recovers the EOA sender, and requires protected chain ID 5042, the canonical token target, zero value, and exactly ERC20 `transfer(recipient, amount)` calldata. Only legacy, access-list, and dynamic-fee envelopes are accepted. Malformed, oversized, unsupported, and mismatched envelopes fail closed. This function does not replace fresh policy evaluation, approval consumption, expiry checks, or subsequent receipt verification.

## Local proof

`internal/providers/circle/ucw_send_local_fixture_test.go` contains the test-only fake authorization boundary and fixture builders. `ucw_send_local_test.go` exercises:

- Exactly one selected verified wallet binding, including owner, provider identity, wallet ID/address, binding ID/version, chain and network. Missing, ambiguous, pending, revoked, or substituted bindings fail. This is per-request selection, not a new database constraint limiting a user to one wallet globally.
- Frozen SEND fields and digest-bound policy and explicit approval. Modified frozen fields cannot retain their digest or reuse the original authorization challenge.
- Separate challenge creation, simulated user acceptance/rejection, and signed-result observation. Rejection is terminal; missing authorization/results and conflicting results fail.
- Independent EOA recovery and exact transaction checks before any possible broadcast. Chain 5042002 is a negative fixture only; production chain validation stays 5042.
- Retry with the same deterministic execution ID and provider idempotency key, one challenge creation, and reconciliation of uncertain results. There is no submission method.
- Disabled Circle Mainnet configuration, adapter construction, and execution wiring.

Fixtures use synthetic public signature scalars, not private keys or signing secrets. Sender recovery determines mock address metadata. Zero transaction gas and fees make the fixtures non-executable. The binding verification reference is mock evidence; no real Circle wallet ownership is verified. No Circle user token or unrestricted signing credential is present.

## Reproduce offline

With dependencies already cached:

```sh
GOPROXY=off GOSUMDB=off WIZPAY_ARC_INTEGRATION=0 go test -mod=readonly ./internal/providers/circle -run '^(TestLocalUCW|TestConfigured|TestLoadConfig|TestNewAdapter|TestCircleArcMainnetExecutionUnavailable)' -count=1
GOPROXY=off GOSUMDB=off WIZPAY_ARC_INTEGRATION=0 go test -mod=readonly ./internal/send ./internal/providers/wiring ./internal/wallet ./internal/intents ./internal/policies ./internal/approvals ./internal/execution/... -count=1
```

`go-ethereum` was already a direct dependency at v1.16.7. Importing `core/types` requires additional indirect module declarations and checksums, including upstream default and `ckzg` build-tag dependencies. No existing dependency version is upgraded.

## Remaining boundaries

**Local mock validation:** proves only the domain and transaction checks above. The fake is in-memory and does not prove provider persistence, concurrency, restart recovery, real user authentication, or actual Circle SDK signing. SCA/account-abstraction signatures are outside this EOA harness. ChatGPT, Claude, and Grok user flows have not been tested end-to-end.

**Real Circle API compatibility — UNVERIFIED / REQUIRES OWNER DECISION:** verify the exact supported Arc environment, EOA transaction representation, sign-only challenge/result semantics, and user-device authorization handoff using current official Circle documentation and separately authorized provider validation. WizPay must never receive signing secrets or Circle user tokens. No provider capability is inferred from this fake.

**Arc Mainnet production activation — DISABLED:** chain ID 5042 remains the production model. Any activation requires a separately authorized later action, verified compatibility, durable reconciliation, current policy/approval/binding checks, and receipt verification. A decoded signed transaction is an observation, not verified financial success. Testnet must remain an isolated harness and cannot weaken Mainnet validation.

## Validation results in the restricted workspace

The targeted Circle harness/configuration/constructor guards and SEND, wiring, wallet, intent, policy, approval, execution, and execution-runtime package tests passed. `gofmt`, `go vet -mod=readonly ./...`, and `git diff --check` passed.

The full offline `go test -mod=readonly ./...` run failed on existing environment-dependent tests: Arc `TestAttestDeploymentCodeIsCanonicalAndReadOnly` and Circle `TestCircleMainnetHealthRemainsOfflineInTrackA` could not open `httptest` listeners (`listen tcp6 [::1]:0: socket: operation not permitted`); Postgres `TestMain` could not access `unix:///var/run/docker.sock` (`permission denied`). No test or production guard was weakened to bypass these restrictions. No real Circle API or Arc financial execution was performed.
