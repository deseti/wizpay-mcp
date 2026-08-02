# Official Circle and Arc sources reviewed

Reviewed on 2026-08-02. These are the only external sources used for Circle/Arc-specific Phase 0 statements.

## Circle

- [User-Controlled Wallets](https://developers.circle.com/wallets/user-controlled): supports the selected wallet model, user custody/control, application orchestration without holding user keys, and user approval of transactions.
- [Key Management](https://developers.circle.com/wallets/key-management): supports the statement that user-controlled wallets use 2-of-2 MPC and only users sign after their authentication; signing shares/credentials remain outside WizPay MCP.
- [Transaction Signing and Authorization](https://developers.circle.com/wallets/signing-and-authorization-models): supports separation of initiation from user authorization and the rule that submitted and finalized are different events.
- [Create wallets API reference](https://developers.circle.com/api-reference/wallets/user-controlled-wallets/create-user-wallet): reviewed only to confirm that Circle wallet creation is challenge-based and uses user-scoped credentials. No API behavior is implemented and no credential is stored.

The exact Circle identifier for Arc Testnet, supported EOA/SCA choice, production API sequence, authentication mechanism, challenge lifecycle, webhook semantics, and wallet-binding verification procedure remain `UNVERIFIED / REQUIRES OWNER DECISION`.

## Arc

- [Arc Network](https://docs.arc.io/arc-chain): supports Arc Testnet chain ID `5042002`, EVM execution, and USDC as gas.
- [Connect to Arc](https://docs.arc.io/arc/references/connect-to-arc): corroborates Arc Testnet chain ID and USDC network currency/native gas precision.
- [Contract addresses](https://docs.arc.io/arc/references/contract-addresses): supports the specific Arc Testnet addresses and token precision facts transcribed in the inventory, and states mainnet addresses are not yet available in the reviewed documentation.
- [Network deployment model](https://docs.arc.io/arc/concepts/deployment-model): supports classification of the public testnet as live and mainnet as upcoming in the reviewed documentation.

No RPC endpoint was called. No address was copied from a third party. Documented addresses are not enabled for execution and still require ABI/source/runtime verification in a later authorized phase.

