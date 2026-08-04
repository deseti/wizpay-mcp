# ABI staging boundary

Verified full ABI artifacts for the Arc Testnet Payroll and Swap contracts live here as the authoritative source of truth:

| Artifact | Contract | Runtime package |
|---|---|---|
| `WizPay.json` | WizPay (Payroll) | `internal/contracts/payroll` |
| `WizPaySwapExecutor.json` | WizPaySwapExecutor (Swap) | `internal/contracts/swap` |

These full ABI files are **reference-only**. Runtime code embeds only minimal allowlisted ABI fragments for approved execution functions, read functions, and verification events. Admin functions present in the full ABI are intentionally excluded from the MCP/runtime execution surface.

Adding or updating a file here does not by itself authorize money movement, enable Phase 10 capabilities, or perform a live transaction. Phase 12 remains the financial capability implementation boundary.

