# Chain, token, contract, deployment, and ABI inventory

## Status contract

- `VERIFIED`: the stated static fact is supported by the cited official Arc documentation reviewed for Phase 0. This does not mean a runtime RPC/code verification has occurred.
- `UNVERIFIED`: insufficient trusted evidence exists in this repository and the reviewed official documentation. It is disabled.

An address listed as `VERIFIED` is inventory evidence only, not permission to execute. Enablement separately requires owner selection, reviewed ABI, bytecode/runtime verification, domain verifier, tests, and explicit later-phase authorization.

## Networks

| Name | Network | Chain ID | Address | Source | Status | Purpose | Allowed MCP domain |
|---|---|---:|---|---|---|---|---|
| Arc Testnet | Arc public testnet | `5042002` | N/A (network) | [Arc Network](https://docs.arc.io/arc-chain), [Connect to Arc](https://docs.arc.io/arc/references/connect-to-arc) | VERIFIED | future non-production development | none in Phase 0 |
| Arc Mainnet | Arc mainnet | UNVERIFIED | N/A (network) | [Arc deployment model](https://docs.arc.io/arc/concepts/deployment-model) says mainnet is upcoming in the reviewed material | UNVERIFIED | future production | none |
| all other chains | owner-selected | UNVERIFIED | N/A (network) | no Phase 0 trusted project inventory | UNVERIFIED | future source/destination chains | none |

## Tokens

| Name | Network | Chain ID | Address | Source | Status | Purpose | Allowed MCP domain |
|---|---|---:|---|---|---|---|---|
| USDC optional ERC-20 interface | Arc Testnet | `5042002` | `0x3600000000000000000000000000000000000000` | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) | VERIFIED | token identity; documented as 6 decimals | candidate wallet/payroll/swap/bridge only; disabled |
| USDC native gas representation | Arc Testnet | `5042002` | native (no contract address) | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) | VERIFIED | gas/native balance; documented as 18-decimal precision | candidate balance only; disabled |
| EURC | Arc Testnet | `5042002` | UNVERIFIED in this Phase 0 inventory | reviewed official page but address not transcribed/validated | UNVERIFIED | potential swap token | none |
| any mainnet token | Arc Mainnet | UNVERIFIED | UNVERIFIED | official mainnet deployment data unavailable in reviewed source | UNVERIFIED | future production | none |

The native and ERC-20 USDC decimal representations must never be mixed. Token amounts use the selected token representation's exact decimals and base units.

## Candidate official Arc Testnet contracts (not enabled)

| Name | Network | Chain ID | Address | Source | Status | Purpose | Allowed MCP domain |
|---|---|---:|---|---|---|---|---|
| CCTP TokenMessengerV2 | Arc Testnet | `5042002` | `0x8FE6B999Dc680CcFDD5Bf7EB0974218be2542DAA` | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) | VERIFIED | candidate cross-chain messaging/burn entrypoint | bridge candidate; disabled |
| CCTP MessageTransmitterV2 | Arc Testnet | `5042002` | `0xE737e5cEBEEBa77EFE34D4aa090756590b1CE275` | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) | VERIFIED | candidate message receive/verification | bridge candidate; disabled |
| CCTP TokenMinterV2 | Arc Testnet | `5042002` | `0xb43db544E2c27092c107639Ad201b3dEfAbcF192` | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) | VERIFIED | protocol token minting component; not a public MCP target | none |
| StableFX FxEscrow | Arc Testnet | `5042002` | `0x867650F5eAe8df91445971f14d89fd84F0C9a9f8` | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) | VERIFIED | candidate documented FX settlement escrow | swap candidate; disabled |
| Permit2 | Arc Testnet | `5042002` | `0x000000000022D473030F116dDEE9F6B43aC78BA3` | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) | VERIFIED | candidate allowance mechanism required by documented StableFX flow | swap candidate; disabled |
| GatewayWallet | Arc Testnet | `5042002` | `0x0077777d7EBA4688BDeF3E311b846F25870A19B9` | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) | VERIFIED | candidate chain-abstracted balance component | none until owner decision |
| GatewayMinter | Arc Testnet | `5042002` | `0x0022222ABE238Cc2C7Bb1f21003F0a260052475B` | [Arc contract addresses](https://docs.arc.io/arc/references/contract-addresses) | VERIFIED | candidate gateway minting component; not a public MCP target | none |
| payroll contract(s) | any | UNVERIFIED | UNVERIFIED | no trusted project artifact supplied | UNVERIFIED | payroll | none |
| ANS registry/resolver/registrar | Arc | `5042002` or mainnet UNVERIFIED | UNVERIFIED | no trusted project artifact supplied/reviewed official deployment | UNVERIFIED | ANS reads/registration | none |
| withdrawal route/contract | any | UNVERIFIED | UNVERIFIED | owner decision absent | UNVERIFIED | withdrawal | none |

## ABI requirements

No ABI file is verified or enabled in Phase 0; `contracts/abi` remains placeholder-only.

| Contract/interface | Version/source | Required functions | Required events/evidence | Status |
|---|---|---|---|---|
| ERC-20 token | exact deployed source/official ABI UNVERIFIED | `balanceOf`, `decimals`, and only later-approved transfer/allowance functions | `Transfer`, plus call return/revert semantics | UNVERIFIED |
| CCTP TokenMessengerV2 | official exact deployment ABI UNVERIFIED | exact burn/deposit function selected by later design | message/burn events linking source to destination | UNVERIFIED |
| CCTP MessageTransmitterV2 | official exact deployment ABI UNVERIFIED | exact receive function selected later | message receipt and mint linkage | UNVERIFIED |
| StableFX FxEscrow | official exact deployment ABI UNVERIFIED | exact taker/settlement functions selected later | trade identifiers, input/output, parties, terminal settlement | UNVERIFIED |
| Permit2 | exact deployed ABI/version UNVERIFIED | only narrowly approved allowance/permit functions | approval/nonce evidence required by design | UNVERIFIED |
| payroll | contract/version absent | exact allowlisted batch function(s) | per-recipient transfers and/or authoritative batch event | UNVERIFIED |
| ANS | registry/resolver/registrar version absent | availability/read and exact registration functions | ownership/controller/registration evidence | UNVERIFIED |

## Owner decisions required before Phase 1 execution work

- first vertical-slice domain and whether it targets Arc Testnet;
- exact Circle-supported Arc network identifier/account type and wallet API flow;
- exact contracts/routes and official ABI sources;
- receipt finality/verification policy per domain;
- provider allowlist and idempotency/reconciliation guarantees;
- production Arc/mainnet chain and deployment details when officially available;
- ANS contracts, normalization standard, registration lifecycle, and trusted source;
- fee policy (currently no added WizPay fee) and any custody/treasury proposal (currently prohibited by default).
