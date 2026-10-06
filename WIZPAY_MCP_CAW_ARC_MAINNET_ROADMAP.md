# WizPay MCP — Circle Agent Wallet (CAW) Arc Mainnet Roadmap

**Project:** WizPay MCP  
**Network:** Arc Mainnet  
**Chain ID:** `5042`  
**Status:** Locked implementation roadmap  
**Scope:** Wallet authority and execution integration for Circle Agent Wallet (CAW)

## Core Rules

- Arc Mainnet only (`chainId = 5042`).
- Do not fall back to Arc Testnet (`5042002`) or any other test chain.
- WizPay must never store or control user private keys, seed phrases, signing shares, OTPs, or unrestricted wallet credentials.
- Circle Agent Wallet is treated as a delegated execution wallet, not as a WizPay treasury wallet.
- WizPay policy, delegation, grants, spend caps, idempotency, recovery, verification, and audit remain the primary control plane.
- Circle wallet policy is defense-in-depth and must not replace WizPay authorization controls.
- Production money movement remains disabled until the required validation steps are complete.
- SEND is implemented and validated before Payroll or Swap.
- Any real Mainnet transaction requires explicit approval before broadcast.

## Implementation Roadmap

### 1. Audit live CAW API/CLI semantics

Validate the actual Circle Agent Wallet runtime behavior using the current Circle CLI/API and the existing Arc Mainnet Agent Wallet.

Confirm:

- Circle Agent Wallet lifecycle.
- Mainnet authentication/session behavior.
- `ARC` Mainnet support.
- Wallet type and account model.
- Available read, sign, transfer, execute, and policy operations.
- Which operations are user-authorized versus agent-authorized.
- Provider identifiers and wallet metadata returned by Circle.
- Transaction submission and status/reconciliation semantics.
- Spending-policy behavior.
- Whether signing, execution, and broadcasting are separate or combined operations.

This step is observational first. No fund movement.

---

### 2. Determine the exact authorization credential model

Identify the exact credential/authority model required for WizPay MCP to use CAW safely.

Determine:

- What credential authorizes an agent session.
- Whether credentials are ephemeral or durable.
- Credential expiry and revocation behavior.
- Whether credentials are bound to a user, agent, wallet, session, or policy.
- What WizPay may hold transiently.
- What must never enter WizPay backend persistence.
- What can safely be represented in the provider-neutral execution layer.
- How revocation and session expiry interact with retry/recovery.

Required invariant:

> WizPay must not gain unrestricted signing authority or custody of user key material.

---

### 3. Read-only verify Agent Wallet binding on Arc Mainnet

Create a provider-backed verification path for an existing Circle Agent Wallet without executing a transaction.

Verify:

- Circle wallet identity.
- Human owner identity/reference.
- Agent Wallet type.
- Wallet address.
- `blockchain = ARC`.
- `chainId = 5042`.
- `network = MAINNET`.
- Wallet status.
- Account type if Circle exposes it.
- Relevant policy/session metadata.
- Binding version and verification evidence.

The resulting WizPay `WalletBinding` must fail closed on any mismatch.

No broadcast and no fund movement.

---

### 4. Refactor provider identity from UCW to CAW

Replace the current execution-provider identity:

```text
CIRCLE_USER_CONTROLLED_WALLET
```

with the CAW execution model:

```text
CIRCLE_AGENT_WALLET
```

Keep the wallet domain provider-neutral.

Do not rewrite the control plane.

Preserve:

- Identity.
- Wallet binding.
- Intent.
- Approval.
- Policy.
- Delegation.
- Grant.
- Spend caps.
- Execution identity.
- Idempotency.
- Recovery.
- Verification.
- Audit.

---

### 5. Refactor the authorization abstraction

Remove UCW-specific assumptions from the provider-neutral authorization boundary.

The abstraction must support delegated Agent Wallet authority without exposing provider credentials to:

- MCP clients.
- ChatGPT.
- Claude.
- Grok.
- intents.
- approvals.
- policies.
- PostgreSQL.
- logs.
- audit payloads.

Authorization material must remain redacted and non-persistent unless a future reviewed design explicitly allows safe metadata.

---

### 6. Implement CAW SEND plan

Implement CAW execution for **SEND only**.

Target flow:

```text
ChatGPT / Claude / Grok
        ↓
WizPay MCP
        ↓
Typed SEND Intent
        ↓
Policy + Delegation + Grant
        ↓
Execution Request
        ↓
Circle Agent Wallet
        ↓
Arc Mainnet
```

SEND must remain:

```text
canonical ERC-20 transfer(recipient, amount)
```

Supported release tokens remain only the canonical Arc Mainnet tokens approved by WizPay.

Required invariants:

- `chainId == 5042`
- `network == MAINNET`
- canonical token only
- exact recipient
- exact amount
- exact calldata
- no arbitrary target
- no arbitrary calldata
- no arbitrary signer
- no arbitrary native value
- intent-bound
- policy-bound
- delegation/grant-bound
- idempotent
- recoverable
- audited

---

### 7. No-broadcast / local validation

Before any real transaction:

- Build the exact CAW SEND execution plan.
- Validate wallet binding.
- Validate authorization scope.
- Validate policy/delegation/grant.
- Validate transaction material.
- Validate signer/wallet identity when applicable.
- Validate `chainId = 5042`.
- Validate canonical token target.
- Validate exact `transfer(recipient, amount)` calldata.
- Validate retry/recovery behavior.
- Validate no-blind-resubmit behavior.
- Validate provider ambiguity handling.
- Validate fail-closed mismatch behavior.

The test must stop before broadcast.

Arc Testnet must remain rejected.

---

### 8. Real Arc Mainnet SEND with minimum amount

Only after Steps 1–7 pass:

- Use Arc Mainnet `5042`.
- Use the verified CAW binding.
- Use the minimum practical transaction amount.
- Require explicit human approval before execution.
- Confirm the exact execution intent.
- Broadcast only once.
- Reconcile rather than blindly resubmit.
- Verify Arc receipt.
- Verify sender.
- Verify token contract.
- Verify calldata.
- Verify exact `Transfer` event.
- Record audit evidence.

A Circle provider status or transaction hash alone is not financial success.

Success requires Arc Mainnet verification.

---

### 9. Implement Payroll and Swap

Only after real SEND is proven.

Order:

1. Same-token Payroll.
2. Swap.
3. Cross-token Payroll.

Reuse the existing reviewed Arc Mainnet contracts and typed execution model.

Do not introduce:

- arbitrary contract execution
- arbitrary router selection
- arbitrary calldata
- arbitrary token selection
- treasury-funded user execution
- developer-controlled user wallets

## Target Authority Model

```text
Human User
    │
    │ owns / authorizes
    ▼
WizPay Identity
    │
    ├── Wallet Binding
    │
    ├── Delegation
    │
    ├── Grant
    │
    └── Spend Policy
            │
            ▼
       AI Agent Identity
            │
            ▼
    Circle Agent Wallet
            │
     bounded execution
            │
            ▼
      Arc Mainnet 5042
```

Effective authority is the intersection of:

```text
WizPay Policy
∩ WizPay Delegation
∩ WizPay Grant
∩ Circle Agent Wallet policy
```

The most restrictive applicable limit wins.

## Release Boundary

Until explicitly approved:

```text
CAW Arc Mainnet binding        = READ-ONLY
CAW production execution       = DISABLED
Mainnet SEND broadcast         = DISABLED
Payroll execution              = DISABLED
Swap execution                 = DISABLED
Cross-token Payroll execution  = DISABLED
```

The release progresses only when each preceding roadmap step has been verified and accepted.
