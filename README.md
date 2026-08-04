# WizPay MCP

WizPay MCP is an independent MCP-native payment orchestration service. Phase 11 assembles the provider execution boundary — a provider-neutral wiring layer, the Circle User-Controlled Wallet adapter, the Arc receipt verifier, and typed Payroll/Swap contract deployment primitives — without implementing the domain planning that would let it move funds.

## Current implementation status

Phases 0–7 established the runtime, identity/wallet, intent/approval, policy, execution-control, MCP tool, and tenant-isolated PostgreSQL persistence foundations. Phase 8 adds verified-principal normalization, persisted identity eligibility checks, typed capability authorization, trusted request context, and canonical storage.Scope mapping. Phase 9 adds PostgreSQL-backed execution leases/fencing, deterministic resume, provider-neutral adapter/verifier boundaries, and verification-gated completion. Phase 10 registers Payroll, Swap, Bridge, and ANS as immutable versioned metadata and provides deterministic, provider-neutral availability decisions. Phase 11 assembles those boundaries into a concrete provider plane: a `providers/wiring` layer composes the Circle User-Controlled Wallet adapter and the Arc Testnet receipt verifier, wires them into the execution worker and into capability availability, and does so fail-closed.

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

- **Circle User-Controlled Wallet adapter** initiates transfers only the user can authorize and reconciles their outcome. It never signs, never holds a private key, seed phrase, or signing share, never completes a challenge on the user's behalf, and never accepts a wallet identifier from runtime input — the wallet comes from the approved plan alone.
- **Arc receipt verifier** is read-only. It reads transaction receipts and the chain head over a single Arc Testnet JSON-RPC endpoint (`https://rpc.testnet.arc.io`, chain ID `5042002`), confirms the endpoint's chain identity before trusting any receipt, and is the only component permitted to assert on-chain success or failure. An absent receipt is reported as unknown, never as failure. Arc uses deterministic BFT finality; default required confirmations is `1`.
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

Verified Arc Testnet deployments are registered in the static in-process registry `internal/contracts` at MCP `RegistryVersion` `1` (artifact metadata only — **not** a Solidity semantic version):

| Role | Contract | Address | Chain ID |
|---|---|---|---:|
| Payroll | WizPay | `0x87ACE45582f45cC81AC1E627E875AE84cbd75946` | `5042002` |
| Swap | WizPaySwapExecutor | `0x17685466759f9Cde06f0DCbB5464164ABe541eFA` | `5042002` |

- Full verified ABIs: `contracts/abi/WizPay.json`, `contracts/abi/WizPaySwapExecutor.json` (reference-only).
- Runtime uses minimal allowlisted ABI fragments in `internal/contracts/payroll` and `internal/contracts/swap`.
- Admin functions are intentionally excluded from the runtime execution surface.
- No generic arbitrary contract executor exists; destination addresses come only from the registry.
- No separate FX Engine deployment is assumed or registered.
- Bridge, CCTP, and ANS remain untouched.
- Contract artifacts do not enable Phase 10 capability availability. Phase 12 remains the Financial Capability Modules boundary.
- No live financial transaction was performed as part of this work.

## Explicit non-goals

No domain planner, wallet creation, signing, broadcasting of live contract transactions, approval UI, autonomous spending, or treasury routing exists in Phase 11. The Circle adapter, Arc verifier, and contract encode/decode primitives are assembled but cannot drive an execution without a planner, which Phase 12 — the actual financial capability implementation boundary — will supply. No live financial transaction can occur from this phase, and no mainnet configuration is provided. The worker loop remains provider-neutral and idles whenever the plane is not fully configured.

See docs/architecture.md and docs/persistence.md for boundaries.
