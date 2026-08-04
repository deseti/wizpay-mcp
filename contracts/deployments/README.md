# Deployment staging boundary

Verified Arc Testnet deployments used by the MCP-side contract registry
(`internal/contracts`, RegistryVersion `1` — MCP artifact metadata, **not** a
Solidity semantic version):

| Contract ID | Name | Chain ID | Network | Address |
|---|---|---:|---|---|
| `WIZPAY_PAYROLL` | WizPay | `5042002` | Arc Testnet (`TESTNET`) | `0x87ACE45582f45cC81AC1E627E875AE84cbd75946` |
| `WIZPAY_SWAP_EXECUTOR` | WizPaySwapExecutor | `5042002` | Arc Testnet (`TESTNET`) | `0x17685466759f9Cde06f0DCbB5464164ABe541eFA` |

Full verified ABIs: `contracts/abi/WizPay.json`, `contracts/abi/WizPaySwapExecutor.json`.

No separate FX Engine deployment is registered or assumed. Bridge, CCTP, and ANS deployments remain out of scope. Registration in the static registry does not enable Phase 10 capability availability and does not authorize live financial transactions.

