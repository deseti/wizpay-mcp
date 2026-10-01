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

## 6. X / social posting rules for WizPay

When asked to draft an X post for **WizPay**, use these rules unless the user explicitly overrides them:

1. **WizPay means WizPay Core**, not WizPay MCP.
2. Write the post in **natural native US English**.
3. The post must sound like a real founder/product builder writing casually on X, **not like AI-generated marketing copy**.
4. Avoid overly formal language, corporate wording, rigid list structures, slogan-heavy phrasing, and generic AI-style patterns.
5. Do **not** default to themes like `WizPay is live`, `we just launched`, or similar launch framing; WizPay has already been live for some time.
6. Use **varied themes** based on the real product and what is already working, rather than repeating the same stablecoin/payroll positioning every time.
7. Keep the post product-focused and human-readable. Do not force technical implementation details unless the user explicitly asks for a technical post.
8. When Arc is relevant, **mention the official X account `@Arc`**, not only the word `Arc` or a hashtag.
9. Do not introduce WizPay MCP, ChatGPT, Claude, Grok, execution-authority work, or experimental architecture into a normal WizPay Core product post unless explicitly requested.
10. If the user asks for the meaning/translation, provide the English post first and then a clear Indonesian translation separately.
11. Before drafting, prefer current real WizPay Core product context over imagined roadmap claims. Do not present unfinished or experimental work as shipped product.

### Preferred tone example

Natural founder-style writing is preferred, for example:

> Been thinking a lot about how stablecoin payments should actually feel for normal users. With WizPay on @Arc, we're trying to make payroll and everyday payments feel simple without making people deal with all the crypto stuff underneath.

This example defines the **tone**, not a template to repeat verbatim.

## 7. Accuracy-first rule — "jangan pernah salah"

The user's standing instruction is **"jangan pernah salah"**. Operationally, this means:

1. **Never guess when a claim can be verified.** Check the relevant repository, source, current state, screenshot, or official documentation first.
2. **Never present an assumption as a fact.** If something cannot be verified, say that it is uncertain or unverified.
3. **Verify repository identity before acting.** In particular, never confuse `wizpay-core` with `wizpay-mcp`.
4. **Verify current product state before writing public-facing copy.** Do not claim a launch, feature, integration, deployment, partner, network status, or capability that is not actually confirmed.
5. **For forms, events, applications, and public posts, prioritize correctness over speed.** Check the context before producing final wording.
6. **If prior context conflicts with current repository/source evidence, use the current verified source and call out the discrepancy instead of silently guessing.**
7. **If there is material ambiguity, stop and verify before giving an answer that could cause a wrong submission, wrong command, wrong deployment, or wrong public statement.**

The practical rule is: **verify first, then answer; if verification is unavailable, state uncertainty rather than inventing certainty.**

## 8. Short canonical rule

> **WizPay = WizPay Core. WizPay MCP = the separate AI/MCP integration layer. Never conflate them.**

> **For WizPay X posts: native US English, casual human founder tone, varied product themes, no launch framing by default, mention `@Arc` when relevant, and never inject MCP unless explicitly requested.**

> **Accuracy first: never guess; verify before asserting. If something cannot be verified, say so instead of presenting it as fact.**
