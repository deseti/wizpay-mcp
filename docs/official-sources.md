# Official Circle and Arc sources reviewed

Reviewed on 2026-09-27 for Track A — Mainnet Foundation.

## Circle

- [User-Controlled Wallets](https://developers.circle.com/wallets/user-controlled): user custody/control and user-authorized transactions; WizPay does not hold signing secrets.
- [Transaction signing and authorization](https://developers.circle.com/wallets/signing-and-authorization-models): initiation, authorization, submission, and finalization are distinct.
- [Contract execution challenge](https://developers.circle.com/api-reference/wallets/user-controlled-wallets/create-user-transaction-contract-execution-challenge): challenge creation is not financial success.
- [Supported blockchains](https://developers.circle.com/wallets/supported-blockchains) was reviewed, but Track A does not use a Circle Arc Mainnet enum or infer an executable authorization path from general network-support metadata.

The Arc Mainnet blockchain enum, production account-type selection, delegated/user authorization path, challenge lifecycle, webhook semantics, and wallet-binding verification procedure remain `UNVERIFIED / REQUIRES OWNER DECISION`. `WIZPAY_CIRCLE_ENABLED=true` is rejected and Track A cannot construct a Circle execution adapter.

## Arc

- [Connect to Arc](https://docs.arc.io/arc/references/connect-to-arc): Arc Mainnet chain ID `5042`, RPC `https://rpc.mainnet.arc.io`, explorer `https://explorer.arc.io`, and native USDC gas precision. It separately identifies Testnet as `5042002`, which the Mainnet configuration rejects.
- [Contract addresses](https://docs.arc.io/arc/references/contract-addresses): Mainnet USDC `0x3600000000000000000000000000000000000000` and EURC `0xbEf5f6d51CB62b58e6A8f77868681825C6fe21c1`, both with 6-decimal ERC-20 interfaces. Native USDC gas precision is 18 decimals and must not be mixed with ERC-20 units.
- Official Arc finality guidance describes deterministic BFT finality. WizPay MCP retains a minimum confirmation value of `1` and defensive observation-consistency checks.

No financial transaction was performed. The optional Arc integration harness is read-only and disabled unless `WIZPAY_ARC_INTEGRATION=1`; ordinary unit tests remain offline.
