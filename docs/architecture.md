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

## Explicit non-goals

No microservices, production database/Redis/River behavior, chain/provider calls, signing, broadcasting, OAuth server, wallet creation, approval UI, financial execution, fee logic, treasury routing, complex UI, or autonomous spending exists through Phase 3.
