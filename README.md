# WizPay MCP

WizPay MCP is an independent, MCP-native payment orchestration service. It is not a proxy for WizPay Core or Nano WizPay. Static, verified facts such as ABIs, deployments, event signatures, token metadata, network parameters, and documented on-chain rules may be reused; runtime business logic and service coupling may not.

## Current implementation status

Phase 0 established the behavioral and security contracts. Phase 1 added the minimal Go runtime foundation. Phase 2 added provider-neutral identity and wallet-binding contracts. Phase 3 added typed intents and explicit approvals. Phase 4 added deterministic policy evaluation. Phase 5 added validated execution references and a provider-neutral adapter boundary. Phase 6 adds a typed MCP tool registry, schemas, validation, safe handlers, and domain-service ports for exactly six foundation tools.

The six available definitions are `wizpay.create_intent`, `wizpay.get_intent`, `wizpay.request_approval`, `wizpay.get_approval`, `wizpay.evaluate_policy`, and `wizpay.prepare_execution`. `prepare_execution` creates only a reference-bound execution request and cannot invoke an execution adapter. The application bootstrap still registers no tools because no application-service implementations or authentication context are wired. Phase 6 contains no authentication provider, wallet creation/control, signing, broadcasting, payment execution, persistence implementation, workers, schedulers, queues, blockchain client, adapter implementation, compliance integration, risk scoring, or approval UI implementation.

The future persistence stack remains PostgreSQL as source of truth, Redis only for cache/locks/rate limits, River for durable jobs, and go-ethereum for EVM interaction. The future approval application remains React + Vite. None of those dependencies are activated through Phase 6.

Future public endpoint: `https://mcp.wizpay.xyz/mcp` (not deployed).

## Run the Phase 1 foundation

Requirements: Go 1.25 or newer.

```bash
cp .env.example .env
set -a
. ./.env
set +a
go run ./cmd/server
```

Local routes:

- `POST /mcp` — official MCP Streamable HTTP transport; the current application bootstrap registers no tools.
- `GET /health` — process liveness.
- `GET /readiness` — serving readiness.

The worker entry point is signal-aware but registers zero jobs:

```bash
go run ./cmd/worker
```

Required structured lifecycle events include `configuration_loaded`, `server_started`, and `server_shutdown`.

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
