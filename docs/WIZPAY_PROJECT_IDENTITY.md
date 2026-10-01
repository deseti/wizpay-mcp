# WizPay Project Identity

This document is the canonical naming and context rule for WizPay-related work.

## 1. Default meaning of "WizPay"

When the term **WizPay** is used without any qualifier, it means:

- Repository: `deseti/wizpay-core`
- Role: the primary WizPay product/platform
- Context: Arc-native stablecoin payroll and payment infrastructure
- Use for: product discussions, Arc ecosystem events, forms, pitches, demos, production status, frontend/backend/contracts, deployment, and business/product descriptions

**Do not reinterpret an unqualified reference to "WizPay" as WizPay MCP.**

## 2. Meaning of "WizPay MCP"

The term **WizPay MCP** means:

- Repository: `deseti/wizpay-mcp`
- Role: a separate AI/MCP integration and execution-control layer built around WizPay capabilities
- Target clients: ChatGPT, Claude, Grok, and other MCP-capable AI clients
- Use for: MCP tools, AI-agent access, execution authority, wallet binding, intent/policy/approval controls, delegated execution, recovery, verification, and AI-facing integrations

WizPay MCP is **not** the default meaning of WizPay and must not replace WizPay Core in product/event/pitch context unless the discussion explicitly concerns MCP.

## 3. Relationship

```text
WizPay Core
  = primary product/platform
  = web app + backend + contracts + Arc settlement

WizPay MCP
  = separate AI integration layer
  = lets AI clients access approved WizPay capabilities through MCP
```

WizPay MCP may call or integrate with capabilities/contracts from WizPay Core, but the two repositories, product roles, and release states must remain distinct.

## 4. Mandatory context rules

1. If the user says only **"WizPay"**, use `deseti/wizpay-core`.
2. Use `deseti/wizpay-mcp` only when the user explicitly says **"WizPay MCP"** or the task clearly targets that repository.
3. For Arc events, applications, pitches, project descriptions, and product forms, default to **WizPay Core**.
4. Do not describe WizPay MCP as the primary WizPay product.
5. Do not mix deployment status, feature status, branches, commits, wallet architecture, or production readiness between the two repositories.
6. Before giving repository-specific commands, code, or deployment instructions, verify the repository name/path.
7. If a request could materially apply to either repository and the repository cannot be established from context, verify before acting rather than guessing.

## 5. Status separation

### WizPay Core

Treat as the main WizPay application/protocol stack. Its production, Arc, frontend, backend, contract, and operator status must be derived from `deseti/wizpay-core` sources.

### WizPay MCP

Treat as the separate MCP/AI integration layer. Its migration, execution-authority, capability activation, and AI-client status must be derived from `deseti/wizpay-mcp` sources.

A status from one repository must never be presented as the status of the other.

## 6. Short canonical rule

> **WizPay = WizPay Core. WizPay MCP = the separate AI/MCP integration layer. Never conflate them.**
