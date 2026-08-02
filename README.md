# WizPay MCP

WizPay MCP is an independent, MCP-native payment orchestration service. It is not a proxy for WizPay Core or Nano WizPay. Static, verified facts such as ABIs, deployments, event signatures, token metadata, network parameters, and documented on-chain rules may be reused; runtime business logic and service coupling may not.

## Phase 0 status

This repository currently contains behavioral contracts, security contracts, formal schemas, and a structural modular-monolith scaffold only. It contains no payment execution, wallet creation, transaction signing or broadcasting, provider integration, production persistence, background processing, or approval UI implementation.

The planned backend is Go with the official MCP Go SDK and Streamable HTTP transport. The future persistence stack is PostgreSQL as source of truth, Redis only for cache/locks/rate limits, River for durable jobs, and go-ethereum for EVM interaction. The future approval application is React + Vite. None of those runtime dependencies are activated in Phase 0.

Future public endpoint: `https://mcp.wizpay.xyz/mcp` (not deployed).

## Security baseline

- Every MVP money-moving intent requires explicit user approval.
- Approval binds to a canonical digest of an immutable intent.
- The execution call accepts references, not replacement financial parameters.
- One intent has at most one financial execution identity; retry resumes it.
- `submitted`, `confirmed`, `verified`, and `completed` are distinct.
- WizPay MCP never stores private keys, seed phrases, signing shares, or equivalent authorization secrets.
- User funds do not route through a WizPay treasury by default.
- No additional WizPay payment fee exists without a separate product decision.

## Phase 0 documents

- [Architecture and boundaries](docs/architecture.md)
- [MCP tool inventory and contracts](docs/mcp-tools.md)
- [Public error model](docs/errors.md)
- [Immutable intent and approval digest](docs/transaction-intent.md)
- [Lifecycle state machine](docs/lifecycle.md)
- [Wallet binding, approvals, and policies](docs/wallet-approval-policy.md)
- [Idempotency and recovery](docs/idempotency-recovery.md)
- [Threat model](docs/threat-model.md)
- [Chain, token, contract, and ABI inventory](docs/blockchain-inventory.md)
- [Data classification, retention, and audit](docs/data-audit.md)
- [Official Circle and Arc sources](docs/official-sources.md)
- [Phase 0 completion map](docs/phase-0-checklist.md)
- [JSON schemas](docs/schemas/)

## Later implementation flow

```text
MCP transport
  -> domain-specific application service
  -> immutable intent + policy evaluation + explicit approval
  -> domain-specific execution port
  -> Circle / Arc / approved provider adapter
  -> receipt verifier
  -> recovery and append-oriented audit
```

The first implementation vertical slice must not begin until its required chain, token, contract, ABI, wallet behavior, and verification rules are marked `VERIFIED` in the inventory.
