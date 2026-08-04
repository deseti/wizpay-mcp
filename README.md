# WizPay MCP

WizPay MCP is an independent MCP-native payment orchestration service. Phase 10 adds a typed, versioned capability registry without implementing provider or blockchain execution.

## Current implementation status

Phases 0–7 established the runtime, identity/wallet, intent/approval, policy, execution-control, MCP tool, and tenant-isolated PostgreSQL persistence foundations. Phase 8 adds verified-principal normalization, persisted identity eligibility checks, typed capability authorization, trusted request context, and canonical storage.Scope mapping. Phase 9 adds PostgreSQL-backed execution leases/fencing, deterministic resume, provider-neutral adapter/verifier boundaries, and verification-gated completion. Phase 10 registers Payroll, Swap, Bridge, and ANS as immutable versioned metadata and provides deterministic, provider-neutral availability decisions.

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

## Explicit non-goals

No Circle/Arc integration, wallet creation, signing, broadcasting, smart-contract calls, receipt polling against a real provider, River/Redis dependency, payroll/swap/bridge/ANS execution, approval UI, autonomous spending, or treasury routing exists in Phase 10. Phase 11 remains the provider execution integration boundary and Phase 12 remains the actual financial capability implementation boundary. The worker loop is provider-neutral and requires injected adapter/verifier implementations; no real financial transaction can occur from this phase.

See docs/architecture.md and docs/persistence.md for boundaries.
