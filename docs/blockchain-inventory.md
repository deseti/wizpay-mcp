# Arc Mainnet foundation inventory

This is the Track A static inventory. Registration is not execution authority: Payroll and Swap capabilities remain disabled, and no native-value or ERC-20 approval sequence is implemented.

## Network

| Network | Chain ID | RPC | Explorer | Status |
|---|---:|---|---|---|
| Arc Mainnet (`MAINNET`) | `5042` | `https://rpc.mainnet.arc.io` | `https://explorer.arc.io` | VERIFIED against current [Arc connection documentation](https://docs.arc.io/arc/references/connect-to-arc) |

Arc Testnet (`5042002`) is not accepted by the Mainnet provider or deployment registry.

## Canonical tokens

| Token | Address | ERC-20 decimals | Source |
|---|---|---:|---|
| USDC | `0x3600000000000000000000000000000000000000` | 6 | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) |
| EURC | `0xbEf5f6d51CB62b58e6A8f77868681825C6fe21c1` | 6 | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) |

Arc native USDC uses 18-decimal native gas precision while the ERC-20 interface uses 6 decimals. Track A records this distinction but does not add native-value execution support.

## WizPay contracts

| Contract ID | Source name | Address | Allowed execution descriptors | Verification events |
|---|---|---|---|---|
| `WIZPAY_PAYROLL` | `WizPayPayrollMainnet` | `0x77AC7Cb6507D404b5530fC03e3D39BAaEdE10C34` | `executeSameTokenPayroll(address,address[],uint256[],string)`; `executeCrossTokenPayroll(address,address,address[],uint256[],uint256,uint256,uint256,uint256,string)` | `PayrollBatchExecuted`, `PayrollPayment`, `PayrollReferenceConsumed`, `PayrollSurplusRefunded`, `PayrollSwapExecuted` |
| `WIZPAY_SWAP_EXECUTOR` | `WizPaySwapExecutorMainnet` | `0x7A051F17B237750EF9D4E63fb75381B9F8755774` | `executeSwap(address,address,uint256,uint256,uint256,uint256)` | `WizPayMainnetSwapExecuted` |

Full reviewed ABI artifacts are `contracts/abi/WizPayPayrollMainnet.json` and `contracts/abi/WizPaySwapExecutorMainnet.json`. The old Testnet `batchRouteAndPay`, `routeAndPay`, router/recipient-parameterized swap shape, and Testnet events are not in the Mainnet allowlists.

## Canonical Uniswap V4 resources

| Resource | Value |
|---|---|
| PoolManager | `0x8366a39CC670B4001A1121B8F6A443A643e40951` |
| UniversalRouter | `0x4fca4a51ab4f23a7447b3284fbd7d73289a89fb1` |
| Permit2 | `0x000000000022D473030F116dDEE9F6B43aC78BA3` |
| V4 Quoter | `0x8dc178efb8111bb0973dd9d722ebeff267c98f94` |
| USDC/EURC candidate pool ID | `0xeb0fd02fb8044d5514fb6e165ee134fd547eff0378bb33b76f4b81d8b03bd1ae` |
| Fee | `500` |
| Tick spacing | `10` |
| Hooks | `0x0000000000000000000000000000000000000000` |

The pool ID is a candidate identity, not a claim of current liquidity, executable routing, or quote quality. Router, token, pool, and hook selection are not caller-configurable.

## Track A execution status

- Payroll capability: disabled.
- Swap capability: disabled.
- Mainnet Payroll and Swap planners: fail closed.
- Native-value support: not implemented.
- ERC-20 approval sequencing: not implemented.
- Bridge/CCTP, ANS, invoice, payment link, x402, wallet creation, and generic transaction execution: not implemented.
- Deployment attestation and RPC integration checks are read-only and opt-in; ordinary unit tests remain offline. Attestation observes runtime bytecode/hash and fixed getters for owner, fee recipient/rate, paused state, USDC, EURC, UniversalRouter, Permit2, exposed PoolManager, pool fee, and tick spacing. Bytecode or configuration presence is not execution readiness.
- Circle Arc Mainnet authorization/execution is `UNVERIFIED / REQUIRES OWNER DECISION`; environment configuration cannot activate the adapter or worker in Track A.
