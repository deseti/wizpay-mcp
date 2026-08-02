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

## Explicit non-goals

No microservices, production database/Redis/River behavior, chain/provider calls, signing, broadcasting, OAuth server, wallet creation, approval execution, fee logic, treasury routing, complex UI, or autonomous spending exists in Phase 0.

