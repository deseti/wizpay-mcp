# Deployment staging boundary

Reviewed Arc Mainnet deployments used by the MCP-side contract registry
(`internal/contracts`, RegistryVersion `1` — MCP artifact metadata, **not** a
Solidity semantic version):

| Contract ID | Name | Chain ID | Network | Address |
|---|---|---:|---|---|
| `WIZPAY_PAYROLL` | WizPayPayrollMainnet | `5042` | Arc Mainnet (`MAINNET`) | `0x77AC7Cb6507D404b5530fC03e3D39BAaEdE10C34` |
| `WIZPAY_SWAP_EXECUTOR` | WizPaySwapExecutorMainnet | `5042` | Arc Mainnet (`MAINNET`) | `0x7A051F17B237750EF9D4E63fb75381B9F8755774` |

Full reviewed ABIs: `contracts/abi/WizPayPayrollMainnet.json`, `contracts/abi/WizPaySwapExecutorMainnet.json`.

No separate FX Engine deployment is registered or assumed. Bridge, CCTP, and ANS deployments remain out of scope. Registration in the static registry does not enable Phase 10 capability availability and does not authorize live financial transactions.
