# WizPay MCP

WizPay MCP is an independent, MCP-native payment orchestration service. It is not a proxy for WizPay Core or Nano WizPay. Static, verified facts such as ABIs, deployments, event signatures, token metadata, network parameters, and documented on-chain rules may be reused; runtime business logic and service coupling may not.

## Current implementation status

Phase 0 established the behavioral and security contracts. Phases 1–6 added the runtime, identity/wallet, intent/approval, policy, execution-control, and safe MCP tool boundaries. Phase 7 adds tenant-isolated PostgreSQL persistence using pgx/v5, sqlc, forward migrations, optimistic concurrency, atomic control-plane transactions, verification evidence, and database-enforced append-only audit.

The six available definitions remain `wizpay.create_intent`, `wizpay.get_intent`, `wizpay.request_approval`, `wizpay.get_approval`, `wizpay.evaluate_policy`, and `wizpay.prepare_execution`. The application bootstrap still registers no tools because no authenticated application-service implementations exist. Phase 7 adds no authentication provider, wallet creation/control, signing, broadcasting, payment execution, workers, schedulers, queues, blockchain client, adapter implementation, compliance integration, risk scoring, or approval UI.

PostgreSQL is now the only persistence source of truth. Redis, River, go-ethereum, provider runtimes, and the future React approval application remain inactive.

Future public endpoint: `https://mcp.wizpay.xyz/mcp` (not deployed).

## Run the Phase 7 foundation

Requirements: Go 1.25 or newer.

```bash
cp .env.example .env
# Replace the local-only PostgreSQL password placeholders in .env.
docker compose up -d postgres
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
- [Phase 7 PostgreSQL persistence](docs/persistence.md)
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
