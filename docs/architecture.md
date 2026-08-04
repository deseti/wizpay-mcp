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

## Phase 10 capability registry

Phase 10 adds `internal/capabilities` as a control-plane authority for typed, immutable, versioned capability metadata. The deterministic in-process registry supports exact-version lookup, latest-enabled lookup, canonical descriptor identity, and provider-neutral availability decisions for Payroll, Swap, Bridge, and ANS. Definitions reuse existing intent and permission types and declare approval, policy, execution, chain/network/token/route, and abstract provider-feature requirements.

All initial definitions are disabled because repository-backed provider adapters and executable routes do not yet exist. Availability never performs I/O and does not imply authentication, authorization, approval, policy allow, execution preparation, or execution success. Phase 11 remains provider execution integration; Phase 12 remains actual financial capability implementation. The six Phase 6 MCP tools and the Phase 9 runtime are unchanged.

## Phase 11 provider execution boundary

Phase 11 assembles the provider execution plane without granting it the ability to move funds. `internal/providers` holds the provider-neutral core (registry, plan, reference, classification taxonomy, adapter/verifier ports); `internal/providers/circle` holds the Circle User-Controlled Wallet boundary; `internal/providers/arc` holds the read-only Arc receipt boundary; and `internal/providers/wiring` is the only package that composes them. The dependency rule is strict: the Circle and Arc boundaries never import one another, and the neutral core imports neither.

**Custody and read-only guarantees.** The Circle adapter initiates transfers that only the user can authorize and reconciles their outcome. It never holds a private key, seed phrase, or signing share, never signs, never completes a challenge on the user's behalf, never creates users or wallets, and never accepts a wallet identifier from runtime input — the wallet comes from the approved plan alone. The Arc boundary exposes only transaction-receipt and block-height reads over a single Testnet JSON-RPC endpoint (chain ID `5042002`), with no general-purpose RPC passthrough; it confirms the endpoint's chain identity once before any receipt is trusted.

**Submission is not success.** The classification taxonomy enforces that no provider observation asserts verified success. Every post-submission Circle transaction state — including `CONFIRMED` and `COMPLETE` — maps to submitted-pending; only an Arc receipt at the configured confirmation depth, through the verifier, yields the `VERIFIED` transition. Once a request may have left the process, inconclusive transport and unknown states degrade to ambiguous (reconciliation-only), and reconciliation recovers the persisted provider reference rather than resubmitting, so a submission is never blindly repeated.

**Fail-closed assembly.** `wiring.Build` registers every provider so the registry can explain unavailability, but constructs an adapter only for a fully configured provider. `wiring.BuildWorker` returns the Phase 9 execution worker only when the plane carries both an adapter and a chain-backed verifier; nil fields are checked before the interface assignment so a typed-nil can never masquerade as a present adapter. Because Phase 11 supplies no domain planner, the adapter is always nil, the worker reports unconfigured, and `cmd/worker` idles until shutdown. `wiring.Availability` resolves capabilities using only the provider features the plane actually supplies on the requested chain and network, discarding caller-supplied features, so no caller can assert a capability into availability. No live financial transaction can occur from this phase and no mainnet configuration exists.


## Phase 8 authentication and authorization foundation

Phase 8 protects the control-plane boundary with provider-neutral verified principals, persisted ACTIVE identity resolution, typed capability permissions, private typed context keys, and one canonical trusted-context-to-`storage.Scope` mapping. Authentication, capability authorization, financial approval, policy evaluation, and execution permission remain separate gates. The RSA JWT adapter is a narrow local-key verifier behind `auth.TokenVerifier`; it performs no discovery, provisioning, refresh, session storage, or provider execution. `/mcp` can be protected while `/health` and `/readiness` remain unauthenticated. The bootstrap continues to register zero live tools until authenticated application services exist. See [Phase 8 authentication and authorization](authentication-authorization.md).

Phase 8 added no execution runtime or provider behavior. Phase 9 now supplies only the provider-neutral runtime described above; Phase 10 adds only the control-plane capability registry. Provider/chain integration, Redis, River, wallet creation, signing, broadcasting, real receipt polling, approval UI, and domain-specific financial execution remain absent.
