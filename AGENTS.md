# Agent instructions

These rules apply to the entire repository.

## Safety and authority

- Never commit secrets, credentials, private material, or real `.env` values.
- Never request, use, log, or store private keys, seed phrases, signing shares, user tokens, PINs, OTPs, entity secrets, or equivalent authorization secrets.
- Never perform a wallet, provider, smart-contract, blockchain, or financial transaction unless the owner explicitly authorizes that later phase and the exact action.
- Never commit, push, create/configure a remote, deploy, or modify another repository unless explicitly requested.
- Never silently change roadmap architecture, custody assumptions, fee behavior, or treasury routing.
- Circle and Arc facts require current official documentation from `developers.circle.com` or `docs.arc.io`. Mark unresolved behavior `UNVERIFIED / REQUIRES OWNER DECISION`.

## Architecture

- Preserve the modular monolith and domain boundaries in `docs/architecture.md`.
- MCP handlers must not call low-level chain/provider clients directly.
- Keep payroll, swap, bridge, and ANS lifecycles separate; do not create a mega-service or generic `payment.service`.
- Keep source files narrowly scoped; split oversized files by responsibility.
- Do not bypass intent immutability, policy evaluation, explicit MVP approval, deterministic idempotency, or receipt verification.
- Do not create raw or arbitrary contract-call MCP tools.
- The approval UI is never a signing-secret custodian.
- Provider payloads and transaction hashes are observations, not verified success.

## Work discipline

- Inspect the relevant contracts, schemas, sources, and local diffs before editing.
- Preserve user-owned changes and stay within the authorized phase and files.
- Use only names/placeholders in `.env.example`.
- Run only safe local validation unless broader actions are explicitly authorized.

