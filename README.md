# WizPay MCP

WizPay MCP is an independent MCP-native payment orchestration service. Phase 11 assembles the provider execution boundary — a provider-neutral wiring layer, the Circle User-Controlled Wallet adapter, the Arc receipt verifier, and typed Payroll/Swap contract deployment primitives — without implementing the domain planning that would let it move funds.

## Current implementation status

Phases 0–13 established the runtime, identity/wallet, intent/approval, policy, execution-control, MCP tool, provider, financial-module, autonomous-runtime, and tenant-isolated PostgreSQL foundations. Track A migrates the chain identity and sealed Payroll/Swap contract metadata to Arc Mainnet while keeping every money-moving capability disabled and fail-closed.

Authentication is distinct from authorization, financial approval, and execution permission. Raw bearer credentials are transport input only: they are never domain/application input, logged, audited, or persisted. Tenant and actor identity are derived exclusively from verified claims plus persisted identity resolution; MCP tool arguments cannot override them.

The application still does not fake full product readiness: no authenticated application-service implementations are wired, so the live `/mcp` route advertises no tools. Health and readiness remain unauthenticated.

## Run the foundation

Requirements: Go 1.25 or newer.

```bash
cp .env.example .env
docker compose up -d postgres
set -a
. ./.env
set +a
go run ./cmd/server
```

When `AUTH_REQUIRED=true`, startup requires issuer, audience, and an RSA public-key PEM path and protects `/mcp` with bearer verification. Development examples keep authentication disabled until real deployment configuration is supplied. No private keys, client secrets, bearer tokens, Circle credentials, or Arc credentials belong in this repository.

Routes:

- `POST /mcp` — official stateless Streamable HTTP MCP transport; currently no live tools.
- `GET /health` — unauthenticated liveness.
- `GET /readiness` — unauthenticated readiness.

## Security baseline

- Verified credentials produce only normalized typed claims.
- JWT validation requires configured issuer/audience, required timing, and RS256; failures are safe and fail closed.
- Persisted identities must be ACTIVE and match tenant, actor, and provider relationship.
- Typed permissions gate application capabilities; they never imply approval or execution.
- Every MVP money-moving intent still requires explicit approval bound to an immutable digest.
- WizPay MCP never stores private keys, seed phrases, signing shares, or equivalent authorization secrets.

## Capability registry

The in-process registry describes capability-to-intent mappings, required permissions and approval/policy/execution gates, supported constraints, and abstract provider feature requirements. Initial definitions are disabled because no provider adapters or verified execution routes are registered. Capability availability is metadata only: it does not imply authorization, approval, policy allow, execution readiness, or execution success.

## Provider execution boundary (Phase 11)

The `internal/providers/wiring` package assembles the provider execution plane from configuration. It keeps the provider-neutral core, the Circle boundary, and the Arc boundary from importing one another, and assembly is declarative and fail-closed:

- **Circle boundary** retains its user-controlled adapter and reconciliation safeguards, but Arc Mainnet authorization/execution is `UNVERIFIED / REQUIRES OWNER DECISION`. Track A never constructs the adapter, generic verifier, domain verifier, or worker, even when Circle environment fields are populated. It never signs or holds a private key, seed phrase, or signing share.
- **Arc receipt verifier** is read-only. It reads transaction receipts and the chain head over the single Arc Mainnet JSON-RPC endpoint (`https://rpc.mainnet.arc.io`, chain ID `5042`), confirms the endpoint's chain identity before trusting any receipt, and is the only component permitted to assert on-chain success or failure. Arc Testnet configuration is rejected. An absent receipt is reported as unknown, never as failure. The default required confirmations is `1`.
- **Provider submission is never verified success.** No Circle transaction state — including `CONFIRMED` and `COMPLETE` — maps to verified success; every post-submission state maps to submitted-pending so the runtime advances to on-chain verification. Only an Arc receipt at the configured confirmation depth yields generic chain-level verification. Phase 12 domain event verification remains separate.
- **Reconcile, never blindly resubmit.** Once a request may have left the process, an inconclusive response is classified as ambiguous (reconciliation-only), and reconciliation recovers the persisted provider reference rather than issuing a second submission.

Two integrations connect the plane to the rest of the system, both fail-closed:

- **Worker (Phase 9).** `wiring.BuildWorker` constructs the execution worker only when the plane carries both a provider adapter and a chain-backed verifier. Because Phase 11 supplies no domain planner — turning an approved intent into a concrete transfer is Phase 12 capability logic — the adapter is always nil, so the worker reports unconfigured and the process idles. No execution, and therefore no financial transaction, can be driven from this phase.
- **Capability availability (Phase 10).** `wiring.Availability` resolves a capability with the provider features a configured provider actually supplies on the requested chain and network, discarding any features a caller placed on the request. A caller can never assert a feature into existence, so every execution-requiring capability stays unavailable until a real provider is configured.

Both `cmd/server` and `cmd/worker` own no provider secrets in the repository; Circle and Arc credentials are supplied only through the environment. See `docs/architecture.md` for the full boundary description.

## Provider-plane hardening (Phase 11 corrective)

Additional fail-closed controls for the Payroll + Swap provider plane:

- **Observation-integrity verification** — successive Arc receipt observations compare block hash, block number, confirmation depth, and presence; inconsistency stays reconciliation-only and never resubmits. On Arc this is a defensive RPC/observation guard (committed blocks are not expected to reorg under deterministic BFT finality).
- **Provider health probes** — non-financial Circle reachability and Arc chain-identity/block-height checks with bounded timeouts; process `/health` liveness does not depend on external providers.
- **Circuit breakers** — CLOSED / OPEN / HALF_OPEN breakers on outbound Circle and Arc infrastructure calls; validation and missing user authorization do not open the breaker.
- **Optional sandbox/testnet harness** — offline by default; see `docs/phase-11-security-recovery-review.md` for env flags and explicit non-goals.

## Contract deployment artifacts (Payroll + Swap)

Reviewed Arc Mainnet deployments are registered in the static in-process registry `internal/contracts` at MCP `RegistryVersion` `1` (artifact metadata only — **not** a Solidity semantic version):

| Role | Contract | Address | Chain ID |
|---|---|---|---:|
| Payroll | WizPayPayrollMainnet | `0x77AC7Cb6507D404b5530fC03e3D39BAaEdE10C34` | `5042` |
| Swap | WizPaySwapExecutorMainnet | `0x7A051F17B237750EF9D4E63fb75381B9F8755774` | `5042` |

- Full reviewed ABIs: `contracts/abi/WizPayPayrollMainnet.json`, `contracts/abi/WizPaySwapExecutorMainnet.json` (reference-only).
- Runtime uses minimal allowlisted ABI fragments in `internal/contracts/payroll` and `internal/contracts/swap`.
- Admin functions are intentionally excluded from the runtime execution surface.
- Optional Arc attestation reads bytecode plus fixed registry-only getters for owner, fee recipient/rate, paused state, canonical tokens, router/Permit2, and exposed pool configuration. These observations never enable execution.
- No generic arbitrary contract executor exists; destination addresses come only from the registry.
- No separate FX Engine deployment is assumed or registered.
- Bridge, CCTP, and ANS remain untouched.
- Contract artifacts do not enable capability availability. Track A planners and domain verifiers explicitly refuse execution.
- No live financial transaction was performed as part of this work.

## Explicit non-goals

Track A adds no wallet creation, signing, live transaction broadcasting, ERC20 approval sequencing, native-value execution, autonomous spending, or treasury routing. Payroll and Swap remain disabled; their planners and domain verifiers fail closed. The Mainnet ABI encoders are typed descriptor primitives only and cannot by themselves activate execution.

See docs/architecture.md and docs/persistence.md for boundaries.
