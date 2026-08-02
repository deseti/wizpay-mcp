# Phase 0 completion map

| Deliverable | Repository artifact | Phase 0 result |
|---|---|---|
| A. MCP tool inventory | `docs/mcp-tools.md` | provider-neutral READ_ONLY/MONEY_MOVING inventory and domain gate map |
| B. input/output schemas | `docs/schemas/common.schema.json`, `public-tools.schema.json`, `public-responses.schema.json` | exact strings/base units, explicit chains/tokens/recipients, bounded arrays, unknown-field rejection |
| C. error model | `docs/errors.md`, `docs/schemas/error.schema.json` | stable taxonomy and retry/user/terminal semantics |
| D. immutable intent | `docs/transaction-intent.md`, `transaction-intent.schema.json` | typed domain union, freeze boundary, canonical digest |
| E. lifecycle | `docs/lifecycle.md` | transitions, terminal/recoverable states, authorization/execution/verification boundaries |
| F. Circle wallet binding | `docs/wallet-approval-policy.md`, `wallet-binding.schema.json` | reference-only binding, fail-closed mismatch, no signing-secret custody |
| G. approval model | `docs/wallet-approval-policy.md`, `approval.schema.json` | per-intent explicit approval, digest/wallet binding, revoke/consume semantics |
| H. policy model | `docs/wallet-approval-policy.md`, `policy.schema.json` | versioned constraints; allow never bypasses MVP approval |
| I. idempotency/recovery | `docs/idempotency-recovery.md` | intent/execution/provider/job/poll layers and crash matrix |
| J. threat model | `docs/threat-model.md` | required threats with prevention, detection, and recovery |
| K. chain/contract inventory | `docs/blockchain-inventory.md`, `contracts/*` | official facts separated from disabled/unverified ABIs and owner decisions |
| L. data/retention | `docs/data-audit.md` | five classes, record handling, forbidden-to-store signing material |
| M. audit model | `docs/data-audit.md` | append-oriented safe event envelope and event taxonomy |

Governance and module boundaries are defined in `README.md`, `AGENTS.md`, and `docs/architecture.md`. The directory scaffold contains no executable production or financial behavior.

