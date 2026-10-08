# Architecture and dependency boundaries

## System boundary

WizPay MCP is an independent modular monolith. It owns its public contracts, identities, immutable intents, approvals, policies, execution records, verification decisions, recovery, and audit trail. It must not delegate those responsibilities to WizPay Core or Nano WizPay runtime APIs.

Allowed reuse is restricted to independently verified static artifacts: ABIs, deployment addresses, event signatures, token/network configuration, and documented on-chain rules.

## Modules

| Area | Responsibility | Forbidden dependency |
|---|---|---|
| `internal/mcp` | Streamable HTTP MCP transport, authentication context, schema validation, response mapping | provider/RPC clients |
| `internal/auth` | application identity and authenticated principal | wallet signing material |
| `internal/wallet` | wallet binding metadata and mismatch checks | user authorization secrets |
| `internal/intents` | canonicalization, digest contract, immutable intent records | transport and provider payloads |
| `internal/approvals` | approval artifact and consumption rules | financial execution implementation |
| `internal/policies` | advisory/deny policy evaluation; never MVP approval bypass | provider clients |
| domain modules | domain validation, plans, execution ports, verification rules | other domains' executors |
| `internal/chain` | narrow chain read/submit/verify adapter interfaces | MCP transport |
| `internal/jobs` | durable-job interfaces and retry orchestration | changing execution identity |
| `internal/storage` | repository interfaces | network/provider calls |
| `internal/audit` | append-oriented, redacted security events | secrets/raw authorization artifacts |
| `web/approval-ui` | login, wallet binding, preview, approval/rejection, policy management, revocation | signing-secret custody and backend authority |

## Dependency direction

```text
MCP transport
  -> domain application service
     -> intents / approvals / policies
        -> domain execution + verification ports
           -> Circle / Arc / approved provider adapters

storage implementations -> domain repository ports
job implementations     -> domain recovery ports
audit sink               <- security-sensitive actions from every layer
```

Dependencies point inward to domain contracts. Provider adapters implement ports; they do not define domain success. Storage repositories never make network calls. React/UI code is outside and cannot be imported by Go domain code.

## Money-moving vertical slice

Every money-moving MCP tool maps to the same conceptual gates, implemented within its own domain:

```text
immutable intent
  -> policy evaluation (deny/advisory only in MVP)
  -> explicit approval bound to digest
  -> domain executor boundary
  -> submission observation
  -> receipt confirmation and domain verification
  -> completion or recovery using the same execution identity
```

No generic arbitrary-call executor is permitted. Shared code is limited to values and protocols that have identical invariants across domains (identifiers, amounts, digests, audit envelopes, and execution leases).

## Infrastructure boundaries (future, not Phase 0 runtime)

- PostgreSQL is authoritative for intents, approvals, execution identities, lifecycle state, and audit references.
- Redis may cache data and provide locks/rate limits, but Redis loss cannot erase or authorize financial state.
- River jobs carry record identifiers and attempt metadata, never mutable payment payloads.
- go-ethereum adapters operate only on allowlisted chains/contracts/functions defined by verified inventories.
- Circle adapters use user-controlled wallet flows only. WizPay MCP cannot obtain unilateral signing capability.

## Phase 1 runtime foundation

The implemented process dependency order is:

```text
environment configuration
  -> structured logger
  -> empty official-SDK MCP server
  -> Streamable HTTP transport
  -> HTTP routes and lifecycle
```

`cmd/server` performs bootstrap and signal handling only. `internal/app` owns dependency order, the HTTP server, readiness state, context cancellation, and graceful shutdown. `internal/mcp` owns official SDK initialization and transport binding. `internal/mcp/tools` exposes a registration interface but Phase 1 supplies no tools. `internal/config`, `internal/logging`, and `internal/errors` remain provider- and domain-neutral.

The HTTP surface is `/mcp`, `/health`, and `/readiness`. Streamable HTTP is configured as stateless with JSON responses. No authentication, provider, chain, persistence, approval, wallet, job, or domain runtime is wired.

## Phase 2 identity and wallet foundation

Phase 2 adds domain contracts without runtime wiring:

```text
resolved identity metadata
  + provider-neutral wallet binding metadata
  -> future Authorizer interface
  -> future intent and approval application layer
```

`internal/auth` owns identity lifecycle, transport-neutral request context, and the future authorization interface. `internal/wallet` owns validated wallet metadata, `PENDING -> ACTIVE -> REVOKED` lifecycle rules, mismatch checks, and the future provider interface. A pending binding may also be revoked directly; the absence of a binding record represents `UNBOUND`, and every revoked binding is terminal. `internal/storage` contains repository interfaces only and performs no I/O.

These domains are not connected to MCP handlers or HTTP middleware. They cannot authenticate, query a wallet provider, hold credentials, sign, submit, approve, or execute anything.

## Phase 3 intent and approval foundation

Phase 3 implements the pre-execution domain boundary without runtime wiring:

```text
typed DRAFT intent
  -> CREATED (material fields frozen; RFC 8785 digest assigned)
  -> APPROVAL_REQUIRED
  -> APPROVED (exact intent/digest/user/wallet-binding artifact)
  -> READY_FOR_EXECUTION (handoff boundary only)
```

`internal/intents` owns a closed financial union for `PAYROLL`, `SWAP`, `BRIDGE`, and `ANS_REGISTRATION`; exact decimal/base-unit amounts; ownership, route, and constraint values; lifecycle rules; immutable revisions after `CREATED`; and a deterministic logical-operation key. `READY_FOR_EXECUTION` is a handoff boundary and remains cancellable or expirable because no execution state or implementation exists yet. It does not mean submitted, settled, or completed. `EXPIRED` and `CANCELLED` are terminal.

`internal/approvals` owns explicit approval artifacts in `PENDING`, `APPROVED`, `REJECTED`, `EXPIRED`, or `CONSUMED`. An artifact derives and retains the exact intent ID/version/digest, user ID, wallet binding ID/version, wallet ID/address, and chain ID. Consumption reserves only the deterministic logical-operation identity; it performs no financial action.

`internal/storage` contains interfaces for future intent and approval persistence with optimistic lifecycle updates and exact replay semantics. `internal/audit` contains event names and typed reference metadata only. Neither package has an implementation or performs I/O.

## Phase 4 policy engine foundation

Phase 4 adds a pure authorization boundary after explicit intent approval:

```text
active identity + active wallet binding + approved intent
  -> exact policy scope and version reference
  -> typed deterministic rules
  -> ALLOW | DENY | REQUIRE_REVIEW
```

`internal/policies` owns immutable policy values, `DRAFT -> ACTIVE -> DISABLED` lifecycle behavior, expiry, typed rules for spending limits, operations, chains, tokens, recipients, and intent lifetime, plus deterministic evaluation. `DISABLED` and `EXPIRED` are terminal. Denial dominates review, and review dominates allow when multiple rules apply. Rule inputs and findings are canonically ordered so repeated evaluation with identical values and time produces identical output.

The pre-execution entry point accepts only an `APPROVED` intent and exact matching active identity and wallet-binding context. A separate pre-approval entry point accepts only `CREATED`; its `ALLOW` result means only that explicit approval may be requested and never performs or bypasses approval. The intent's frozen policy reference must match the policy ID and version. Evaluation reads only supplied in-memory values; it has no transport, persistence, provider, compliance, risk-model, or blockchain dependency. `READY_FOR_EXECUTION` remains a future application-layer transition and is not produced by the policy package.

## Phase 5 execution adapter foundation

Phase 5 defines the execution handoff without implementing it:

```text
consumed exact approval + ALLOW pre-execution policy result
  -> reference-only execution request
  -> one deterministic execution ID per Phase 3 operation key
  -> provider-neutral adapter interface (no implementation)
```

`internal/execution` owns request validation, execution identity, lifecycle, recovery eligibility, provider-neutral result observations, and the future adapter interface. Requests contain only intent, approval, policy-evaluation, operation, and execution references. They contain no replacement financial parameters, transaction payloads, signatures, credentials, receipts, hashes, or raw provider responses.

The ordinary lifecycle preserves the Phase 0 evidence boundaries: `CREATED -> AUTHORIZED -> QUEUED -> EXECUTING -> SUBMITTED -> CONFIRMING -> CONFIRMED -> VERIFIED -> COMPLETED`. Cancellation is allowed only before queueing. Ambiguity enters `RECOVERY_REQUIRED` with a stable safe reason and deterministic same-execution checkpoint. A proven `FAILED` state can enter recovery only when its failure contract explicitly marks it recoverable; terminal failures remain terminal. No transition invokes an adapter, retries work, or schedules a job.

`internal/storage` adds interfaces for atomic create/load-by-operation-key and optimistic lifecycle updates. Future persistence must consume approval and create the one execution atomically. Phase 5 supplies no persistence or I/O.

## Phase 6 MCP tool layer foundation

Phase 6 adds a transport boundary over the existing domain contracts without adding application-service implementations:

```text
typed MCP input + semantic validation
  -> narrow domain service interface
  -> safe typed result or redacted public error
```

`internal/mcp/tools` owns the registry, unique metadata, inferred draft-2020-12 input/output schemas, semantic reference and discriminated-union validation, official SDK registration, handler adaptation, and safe response mapping. The foundation registry contains exactly `wizpay.create_intent`, `wizpay.get_intent`, `wizpay.request_approval`, `wizpay.get_approval`, `wizpay.evaluate_policy`, and `wizpay.prepare_execution`. It rejects incomplete and duplicate definitions.

`internal/services` contains transport-neutral orchestration interfaces only. Implementations must later resolve authenticated identity and wallet authority, enforce ownership, and coordinate persistence and domain objects. No MCP input can supply identity ownership metadata. Execution preparation accepts only intent, approval, and policy references; it cannot accept replacement financial data and has no method for invoking an adapter.

Errors pass through the Phase 0 public error mapper, add the caller's request correlation ID, and never serialize unknown causes. The main application remains intentionally unwired until authenticated service implementations exist, so the live `/mcp` route still advertises zero tools.

## Phase 7 persistence foundation

Phase 7 makes PostgreSQL the sole durable source of truth while preserving inward dependency direction:

```text
application/domain services -> tenant-scoped repository interfaces
PostgreSQL + pgx/sqlc       -> repository implementations
```

All tenant-owned SQL predicates and composite foreign keys include `tenant_id`. Domain packages remain independent of pgx, sqlc, PostgreSQL, and generated types. Explicit mappers reconstruct persisted values through validated domain restoration constructors, including recomputation checks for intent digests and execution request keys.

The database preserves current wallet projections plus immutable wallet-version evidence, intent material, exact composite approval and policy-evaluation bindings, one execution request per operation identity, strict lifecycle revisions and immutable execution-revision snapshots, revision-bound verification observations, and append-only audit records. Serializable multi-record operations and database constraints enforce atomicity, optimistic concurrency, and full immutable retry identity. A migration-owner connection is separate from the restricted application connection; audit update/delete and trigger administration are denied by privileges, with immutability triggers and the insert/read-only repository surface as additional defenses.

The process validates database configuration, opens and pings a bounded pgx pool, applies embedded forward migrations, includes PostgreSQL in readiness checks, and closes the pool on shutdown. It still registers no MCP tools because authentication and application-service implementations remain future phases. See [Phase 7 PostgreSQL persistence](persistence.md).

## Phase 9 execution runtime

Phase 9 provides the provider-neutral execution-control runtime. It resumes immutable prepared execution requests, persists one execution identity per operation identity, claims work with PostgreSQL leases and fencing tokens, and invokes only the existing `execution.Adapter` boundary. A durable submission-start marker makes restart behavior deterministic: a marked pre-call execution reconciles through `GetStatus` rather than blindly submitting again. A separate verifier boundary is required before any final success; verified evidence and the `VERIFIED` lifecycle transition are persisted atomically.

The runtime has no Circle, Arc, wallet, chain, signer, receipt, or provider implementation. Its worker loop is a repository-backed polling boundary and remains inert until explicit provider-neutral adapter and verifier implementations are supplied by a later phase.

## Explicit non-goals

No microservices, Redis/River behavior, chain/provider calls, signing, broadcasting, OAuth server, wallet creation, approval UI, financial execution, compliance API, AI/ML risk scoring, fee logic, treasury routing, complex UI, or autonomous spending exists through Phase 9.

## Phase 13 — final roadmap phase: bounded autonomous runtime

Phase 13 is the final numbered roadmap phase; there is no Phase 14. The
provider-neutral `internal/autonomy` domain and scoped PostgreSQL repositories
add versioned typed schedules,
deterministic UTC occurrences, bounded missed-run handling, lease/fencing
ports, principal/delegation context, user-authorized autonomous grants,
transactional spend reservations, simulation, safe reason codes, emergency
stop, and autonomous audit vocabulary above the existing Phase 12 gates.
PAYROLL and SWAP remain the only executable financial modules. The default
`WIZPAY_AUTONOMY_ENABLED=false` rollout control and existing fail-closed
provider assembly mean a schedule cannot enable unavailable execution. Once
submission may have occurred, the occurrence remains reconciliation-only.
See [Phase 13 autonomous runtime](phase-13-autonomous-runtime.md). Future
work is maintenance/security review/release operations, not another numbered
phase; production launch is a separate explicit decision requiring independent
security review.

## Phase 10 capability registry

Phase 10 adds `internal/capabilities` as a control-plane authority for typed, immutable, versioned capability metadata. The deterministic in-process registry supports exact-version lookup, latest-enabled lookup, canonical descriptor identity, and provider-neutral availability decisions for Payroll, Swap, Bridge, and ANS. Definitions reuse existing intent and permission types and declare approval, policy, execution, chain/network/token/route, and abstract provider-feature requirements.

All initial definitions are disabled because repository-backed provider adapters and executable routes do not yet exist. Availability never performs I/O and does not imply authentication, authorization, approval, policy allow, execution preparation, or execution success. Phase 11 remains provider execution integration; Phase 12 remains actual financial capability implementation. The six Phase 6 MCP tools and the Phase 9 runtime are unchanged.

## Phase 11 provider execution boundary

Phase 11 assembles the provider execution plane without granting it the ability to move funds. `internal/providers` holds the provider-neutral core (registry, plan, reference, classification taxonomy, adapter/verifier ports); `internal/providers/circle` holds the Circle User-Controlled Wallet boundary; `internal/providers/arc` holds the read-only Arc receipt boundary; and `internal/providers/wiring` is the only package that composes them. The dependency rule is strict: the Circle and Arc boundaries never import one another, and the neutral core imports neither.

**Custody and read-only guarantees.** The Circle adapter code retains its user-controlled authorization, idempotency, and reconciliation controls, but Track A does not construct it for Arc Mainnet: that authorization/execution path is `UNVERIFIED / REQUIRES OWNER DECISION`. Populating Circle environment fields cannot produce an adapter, verifier, provider features, or worker. The Arc boundary exposes only bounded reads over the single Mainnet JSON-RPC endpoint (`https://rpc.mainnet.arc.io`, chain ID `5042`), with no general-purpose RPC passthrough and no fallback RPC list; it confirms chain identity before receipt or deployment evidence is trusted. Deployment attestation resolves static registry IDs to addresses and permits only bytecode reads and a fixed getter set. Arc Testnet (`5042002`) is rejected by Mainnet configuration validation.

**Arc finality.** The default confirmation depth is `1` (never configurable below 1). Observation-consistency machinery remains as a defensive guard against contradictory RPC observations (disappearing receipts, block hash/number mismatch, confirmation/head regression); those signals stay reconciliation-only.

**Submission is not success.** The classification taxonomy enforces that no provider observation asserts verified success. Every post-submission Circle transaction state — including `CONFIRMED` and `COMPLETE` — maps to submitted-pending; only an Arc receipt at the configured confirmation depth, through the verifier, yields the generic chain-level `VERIFIED` transition. Transaction hash alone is never success. Phase 12 Payroll/Swap domain event verification remains a separate gate after generic receipt success. Once a request may have left the process, inconclusive transport and unknown states degrade to ambiguous (reconciliation-only), and reconciliation recovers the persisted provider reference rather than resubmitting, so a submission is never blindly repeated.

**Fail-closed assembly.** `wiring.Build` registers Circle as unconfigured metadata but unconditionally refuses Arc Mainnet adapter construction in Track A. `wiring.BuildWorker` therefore returns no worker. `wiring.Availability` exposes no provider features and discards caller-supplied features. Payroll and Swap capability defaults remain disabled; their domain planners refuse Mainnet plans, and their verifiers preserve intent/sender/chain/contract binding checks before returning the explicit Track A disabled outcome.

### Phase 11 contract deployment artifacts (Payroll + Swap corrective)

`internal/contracts` holds a static, typed, versioned deployment registry for the reviewed Arc Mainnet Payroll and Swap contracts. `RegistryVersion` is MCP-side artifact metadata only; it is **not** an invented Solidity semantic version (the deployments do not expose one).

| Contract ID | Name | Address | Chain ID |
|---|---|---|---:|
| `WIZPAY_PAYROLL` | WizPayPayrollMainnet | `0x77AC7Cb6507D404b5530fC03e3D39BAaEdE10C34` | `5042` |
| `WIZPAY_SWAP_EXECUTOR` | WizPaySwapExecutorMainnet | `0x7A051F17B237750EF9D4E63fb75381B9F8755774` | `5042` |

Full verified ABIs live under `contracts/abi/` as reference-only sources of truth. Runtime packages `internal/contracts/payroll` and `internal/contracts/swap` embed only minimal allowlisted ABI fragments for approved money-moving functions, read functions, and verification events. Admin functions present in the full ABI are intentionally excluded. There is no generic arbitrary-call executor (`CallContract`, raw calldata APIs, free-form method selectors, or caller-supplied destination addresses). No separate FX Engine deployment is registered or assumed. Bridge, CCTP, and ANS remain untouched.

These primitives model and encode only the reviewed Mainnet ABI surface. Track A does not plan, approve, submit, sign, or verify Payroll/Swap execution. Capability availability remains separate: registering contract artifacts does not enable Payroll/Swap capabilities.

### Phase 12 Step 3 — typed UCW contract execution (Payroll + Swap)

Phase 12 Step 3 introduced the provider-neutral sealed `CONTRACT_EXECUTION` representation. Track A retains that sealed representation but prevents Payroll/Swap planners from producing executable plans. The Mainnet contract layer accepts neither arbitrary targets nor arbitrary calldata, router, recipient, or token choices. ERC20 approval sequencing and native-value execution are intentionally not implemented.

### Phase 12 Step 4 — Arc receipt logs + Payroll/Swap domain event verification

Phase 12 Step 4 adds **narrow Arc receipt-log extraction** and **pure domain event verifiers** so a generic successful transaction receipt is not sufficient to mark Payroll/Swap financial success.

**Arc receipt logs.** `eth_getTransactionReceipt` on the already-known transaction may yield bounded normalized logs on `providers.Receipt` (address, topics, data, optional log index / tx hash). Bounds: max logs, max topics per log, max data bytes, valid EVM addresses/hashes, deterministic receipt order. There is no eth_getLogs search API and no raw JSON-RPC passthrough. Malformed or oversized log material fails closed. Observation integrity may fingerprint logs (hex SHA-256) and persist only that fingerprint on the adapter reference (`lf=`); full logs are never stored in the reference. Log mutation under the same block identity is reconciliation-only (`LOGS_CHANGED`).

**Provider-neutral evidence.** `providers.Receipt` remains the chain evidence carrier. Logs are observation material for domain packages; they are not financial success. Generic chain-level `VERIFIED` (Arc SUCCESS at min confirmations, default 1) is still only inclusion/finality evidence.

**Domain verifiers.** Pure deterministic gates with no RPC, Circle, or wall-clock:

| Package | Input | Canonical event | Financial complete when |
|---|---|---|---|
| `internal/payroll` | frozen intent + sealed plan + receipt | Mainnet Payroll events | **never in Track A** — verifier fails closed |
| `internal/swap` | frozen intent + sealed plan + receipt | `WizPayMainnetSwapExecuted` | **never in Track A** — verifier fails closed |

Batch `BatchPaymentRouted` proves aggregates (sender, tokenIn, totals, recipientCount, referenceId, and shared tokenOut for single-token-out). It does **not** emit per-recipient settlement; the verifier documents unprovable fields and refuses to claim full financial completion. Ambiguity (zero matches, multiple conflicting matches, wrong contract, malformed topics/data) fails closed to unverified/failed. Trusted static event definitions only — no arbitrary signature/ABI/decoder surface.

**Execution integration.** Domain verifiers are the capability-specific gate after generic receipt success. Runtime/worker orchestration that marks Payroll/Swap financial completion must require **both** chain inclusion/finality **and** domain event verification. Token-transfer chain verification is unchanged. Worker wiring, MCP tools, capability enablement, allowances, Bridge/CCTP/ANS remain out of scope for this step.

**Provider-plane hardening (Payroll + Swap).** The plane also provides: (1) defensive receipt observation comparison (missing receipt after present, block hash/number change, confirmation depth decrease, log fingerprint change) that never triggers resubmission—on Arc this protects against RPC/observation inconsistency rather than expected consensus reorgs; (2) bounded non-financial health probes for configured Circle and Arc dependencies; (3) provider-neutral circuit breakers on outbound infrastructure calls; (4) optional sandbox/testnet integration tests that skip unless explicitly enabled. Process liveness (`/health`) does not depend on external providers. See [Phase 11 security and recovery review](phase-11-security-recovery-review.md).

## Phase 8 authentication and authorization foundation

Phase 8 protects the control-plane boundary with provider-neutral verified principals, persisted ACTIVE identity resolution, typed capability permissions, private typed context keys, and one canonical trusted-context-to-`storage.Scope` mapping. Authentication, capability authorization, financial approval, policy evaluation, and execution permission remain separate gates. The RSA JWT adapter is a narrow local-key verifier behind `auth.TokenVerifier`; it performs no discovery, provisioning, refresh, session storage, or provider execution. `/mcp` can be protected while `/health` and `/readiness` remain unauthenticated. The bootstrap continues to register zero live tools until authenticated application services exist. See [Phase 8 authentication and authorization](authentication-authorization.md).

Phase 8 added no execution runtime or provider behavior. Phase 9 now supplies only the provider-neutral runtime described above; Phase 10 adds only the control-plane capability registry. Provider/chain integration, Redis, River, wallet creation, signing, broadcasting, real receipt polling, approval UI, and domain-specific financial execution remain absent.

## WP2 OAuth foundation

`internal/oauth` owns the constrained Authorization Code/S256 protocol and trusted
browser-consent ports; `internal/http/oauth` owns metadata, protocol routing and
public-origin boundaries. PostgreSQL implements digest-only code/token persistence,
atomic redemption, consent and revocation. OAuth verification feeds the existing
`auth.TokenVerifier` and `requestauth` principal/identity pipeline. Only read authority
is supported; no production browser port or financial authority is wired. See
[WP2 OAuth foundation](wp2-oauth-foundation.md) for endpoints, pre-registration,
compatibility limits and deployment prerequisites.
