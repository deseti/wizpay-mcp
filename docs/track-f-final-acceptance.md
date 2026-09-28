# Track F — final Arc Mainnet migration acceptance

Status: **PASS for code, security, recovery, protocol, and no-broadcast client acceptance**  
Baseline: `672a28e` (`feat: add Arc mainnet cross-token payroll track`)  
Validation date: 2026-09-28  
Live Mainnet activation: **NOT AUTHORIZED / DISABLED**

Track F is the final migration track. It creates neither Track G nor Phase 14 and adds no financial capability. No deployment, wallet funding, contract unpause, Mainnet transaction broadcast, capability enablement, commit, or push occurred.

## Locked release boundary

The accepted capability set is exactly Send, Same-token Payroll, Swap, and Cross-token Payroll. Invoice, Payment Link, Bridge/CCTP, ANS, x402, marketplace/discovery, wallet creation, arbitrary transaction or contract execution, developer-controlled wallets, and treasury-funded user execution remain excluded.

The executable financial material is restricted to:

| Resource | Canonical value |
|---|---|
| Chain / network | `5042` / `MAINNET` |
| USDC | `0x3600000000000000000000000000000000000000` |
| EURC | `0xbEf5f6d51CB62b58e6A8f77868681825C6fe21c1` |
| Payroll | `0x77AC7Cb6507D404b5530fC03e3D39BAaEdE10C34` |
| Swap Executor | `0x7A051F17B237750EF9D4E63fb75381B9F8755774` |

The Payroll and Swap default capability descriptors now carry this explicit chain/network/token/route allowlist while retaining `DISABLED` status. Send was already explicitly constrained. `WIZPAY_CIRCLE_ENABLED=false`, `WIZPAY_ARC_ENABLED=false`, and `WIZPAY_AUTONOMY_ENABLED=false` remain the example defaults. The provider plane never constructs a Mainnet execution adapter, verifier, domain verifier, or worker from the unreviewed Circle path.

## Security and recovery acceptance

- Exact Mainnet ABI selectors and canonical repacking are covered for `transfer(address,uint256)`, `executeSameTokenPayroll(...)`, `executeCrossTokenPayroll(...)`, and `executeSwap(...)`. Obsolete Testnet Payroll/Swap signatures are rejected.
- Arc Testnet chain `5042002`, `TESTNET`, noncanonical token/contract identities, and Testnet Circle authority configuration fail closed. Mainnet receipt verification rejects another chain; all domain verifiers bind sender, target, calldata, value, and events.
- Public MCP schemas expose only typed Send, Payroll, and Swap lifecycle operations. They expose no target, calldata, selector, router, spender, Permit2, sender, signer, signing secret, approval, native-value, or recipient replacement fields on execute.
- Internally generic provider structures remain sealed behind typed planners. Contract target and calldata originate only from immutable reviewed `EncodedCall` values. There is no public raw-call or arbitrary-approval tool.
- Native value is derived from sealed calldata only: Send has no native value; Same-token Payroll is zero; USDC→EURC Swap and Cross-token Payroll equal input base units × `10^12`; EURC→USDC paths are zero. Exact, ±1, missing-required zero, forbidden nonzero, malformed, noncanonical, negative, and over-uint256 funding cases are covered.
- No user-level ERC-20 approval can be submitted in this release because the Mainnet user/delegated authority is unavailable. MCP schemas accept no spender. Consequently the only intended future typed spenders—Payroll for Same-token Payroll and EURC→USDC Cross-token Payroll, Swap Executor for EURC→USDC Swap—cannot be replaced or exercised through current execution.
- Wallet identity is frozen in intent, approval, execution request, plan, and receipt sender verification. Configuration, schemas, DTOs, adapters, persistence, and repository scans found no executable private key, seed phrase, signing share, developer wallet, treasury wallet, caller-selected signer, or Testnet authority path.
- Payroll reference hashing is global per employer and reference string across Same-token and Cross-token variants. Retry/recovery retains the immutable intent and reference. UTF-8 length is bounded to 64 bytes and recipient count to 50.
- One operation key maps to one execution identity. A durable `submission_started` marker switches all later work to reconciliation. Deadline or quote expiry after that marker never authorizes replacement. First submission after the immutable freshness bound is rejected.
- PostgreSQL lease/fencing tests prove exclusive acquisition, durable restart recovery, rollback safety, and stale-owner rejection. The Track F stress case ran 16 concurrent marker attempts with exactly one winner, preserved the marker across lease reclaim, and passed 20 repeated runs.
- RPC absence, timeout, malformed receipts/transactions/logs, missing transaction body, mismatched transaction envelope, wrong chain, contradictory observations, transaction-not-found, and temporary unavailability never promote financial completion or authorize blind resubmission.
- Receipt success is only generic inclusion evidence. Send additionally requires exact ERC-20 calldata and Transfer evidence. Payroll and Swap additionally require their canonical domain events and checked fee/net/output/reference/aggregate/refund arithmetic. The Track F mismatch additions cover wrong chain/emitter/pair, recipient order, obligations, gross input, fee/net, minimum, reference, batch aggregate, and surplus refund.

## Validation results

All commands used Go `1.25.14`. PostgreSQL tests used Testcontainers `postgres:16-alpine` with PostgreSQL client `16.15`.

| Area | Command / evidence | Result |
|---|---|---|
| Focused security | `go test ./internal/contracts/... ./internal/payroll ./internal/swap ./internal/providers ./internal/providers/wiring ./internal/intents ./internal/capabilities ./internal/mcp ./internal/mcp/tools ./internal/execution/... ./internal/services ./internal/policies -count=1` | PASS |
| PostgreSQL 16 | `go test ./internal/storage/postgres -count=1` | PASS (`8.489s`) |
| Repeated submission ownership | `go test ./internal/storage/postgres -run '^TestTrackFSubmissionStartMarkerHasOneWinnerAndSurvivesReclaim$' -count=20` | PASS (`5.280s`) |
| Reference fuzz | `go test ./internal/intents -run '^$' -fuzz '^FuzzTrackFPayrollReferenceUTF8ByteBound$' -fuzztime=5s` | PASS; 107,785 executions |
| Native-value fuzz | `go test ./internal/providers -run '^$' -fuzz '^FuzzTrackFNativeValueDerivationIsDeterministic$' -fuzztime=5s` | PASS; 35,843 executions |
| Race | `CGO_ENABLED=1 go test -race ./... -count=1` | PASS; full repository including PostgreSQL (`10.552s` for that package) |
| Full suite, run 1 | `go test ./... -count=1` | PASS; PostgreSQL `7.795s` |
| Full suite, run 2 | `go test ./... -count=1` | PASS; PostgreSQL `7.594s` |
| MCP protocol | `TestTrackFMCPProtocolClientAcceptance` | PASS |
| No-broadcast dry run | `TestTrackFNoBroadcastDryRunPlansAllLockedCapabilities` plus canonical domain-verifier fixtures/mutations | PASS |
| Formatting / patch integrity | `gofmt` on changed Go files; `git diff --check` | PASS |

The first attempted race command exited before compilation because the base environment had CGO disabled and no C compiler. A temporary local GCC/binutils toolchain was supplied outside the repository, the command was rerun with CGO enabled, and the complete race-instrumented suite passed. The first PostgreSQL run similarly identified missing host `psql`/`pg_dump` utilities; temporary PostgreSQL 16.15 client binaries were supplied outside the repository and the complete integration package then passed. These were environment setup issues, not financial test failures.

## MCP protocol and no-broadcast acceptance

The protocol integration uses one official-SDK Streamable HTTP server with the same typed tools for every client. It verifies initialize/handshake, bearer authentication rejection, `tools/list`, valid schemas, all 12 Send/Payroll/Swap preview/create/execute/status tools, Send preview, immutable intent creation, fail-closed execute without Mainnet authority, reconcilable status, and malformed input rejection. No raw signing or contract-execution tool is listed.

The deterministic dry run traverses real intent freeze, policy allow, explicit approval consumption, deterministic execution request, typed planner, sealed target/value/calldata, stable idempotency, and an unconfigured provider plane for all four locked capabilities. Existing canonical synthetic receipt fixtures reach financial completion only after domain verification; critical-field mutations fail closed. Nothing broadcasts.

## External client acceptance

No supported external client credentials or configured connections were present. These statuses are therefore not inferred from server-side protocol tests:

| Client | Status |
|---|---|
| ChatGPT | **NOT RUN / MANUAL REQUIRED** |
| Claude | **NOT RUN / MANUAL REQUIRED** |
| Grok | **NOT RUN / MANUAL REQUIRED** |

Use a non-production server instance with Mainnet execution authority absent and all capability/provider flags disabled. Use an HTTPS-reachable Streamable HTTP `/mcp` endpoint and the deployment's reviewed authentication flow; never paste a bearer token, wallet secret, OTP, or signing material into this checklist or a chat.

### ChatGPT

1. In a supported ChatGPT workspace, enable developer mode, then open **Settings / Workspace settings → Apps → Create**.
2. Enter the remote WizPay `/mcp` endpoint and configure the reviewed authentication method. Run **Scan Tools**, finish authentication, and create the draft app.
3. In a new chat, select the draft app. Confirm the 12 typed Send/Payroll/Swap tools are visible and no raw calldata, arbitrary execution, approval, or signing tool appears.
4. Run one Send preview and one Send `create_intent` using non-production fixture data. Confirm canonical chain/network/token output.
5. Call Send execute with fixture references and require `capability_unavailable`. Call status and verify the safe not-found/reconcilable response. Repeat visibility and safe preview/create checks for Payroll and Swap without approving or broadcasting.
6. Record PASS only with the app scan and call evidence. Current first-party setup guidance: <https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt>.

### Claude

1. Open **Settings → Connectors** (organization owners first provision the connector for Team/Enterprise), choose **Add connector**, name it WizPay, and enter the remote `/mcp` endpoint.
2. Complete the reviewed authentication flow, connect, and inspect the enabled tools.
3. Perform the same tool visibility, preview, `create_intent`, fail-closed execute, and status checks listed for ChatGPT. Keep every financial action non-broadcast and confirm no arbitrary execution/signing surface.
4. Record PASS only with connection and call evidence. Current first-party setup guidance: <https://support.anthropic.com/en/articles/11175166-getting-started-with-custom-connectors-using-remote-mcp>.

### Grok

1. Open <https://grok.com/connectors>, choose **New Connector → Custom**, enter the remote `/mcp` endpoint, and complete the reviewed authentication flow. Business/Enterprise administrators first provision it under **console.x.ai → Grok Business → Connectors**.
2. Inspect discovered tools and perform the same visibility, preview, `create_intent`, fail-closed execute, and status checks listed above. Confirm no arbitrary execution/signing surface and no broadcast.
3. Record PASS only with connection and call evidence. Current first-party setup guidance: <https://docs.x.ai/grok/connectors>.

## Final checklist

- [x] Mainnet path supports only Arc `5042` / `MAINNET`.
- [x] Canonical USDC/EURC, Payroll, and Swap Executor identities are enforced.
- [x] ABI parity is tested and old Testnet paths/selectors are rejected.
- [x] Send, Same-token Payroll, Swap, and Cross-token Payroll remain correct.
- [x] Native value is typed, deterministic, immutable, and direction-specific.
- [x] No caller-selected spender and no arbitrary approval or contract execution public path exists.
- [x] Wallet binding, approval/grant, policy, intent, execution, and receipt bindings remain enforced.
- [x] Private keys, signing shares, and executable signing-secret inputs are absent.
- [x] Execution is idempotent, persisted, reconcilable, and exclusive across concurrent workers.
- [x] Every financial success is receipt verified; Payroll/Swap success is domain-event verified.
- [x] Wrong chain/address/sender/value/calldata/event evidence and RPC ambiguity fail closed.
- [x] PostgreSQL 16 integration, full race, focused fuzz/property, protocol, dry-run, and two full suites pass.
- [x] No unresolved Critical/High fund-loss or security-boundary issue exists.
- [x] No production deployment or Mainnet transaction occurred.
- [x] Mainnet capability/provider flags remain disabled.
- [x] External client status is recorded honestly as manual required.
- [x] Live Mainnet execution still requires a separate, exact owner authorization after validation of an Arc Mainnet user/delegated signing provider.

## Remaining release blockers

**None under the Track F fund-loss/security blocker standard.**

The absence of a validated Arc Mainnet user/delegated signing provider is an activation prerequisite, not a Track F code/security acceptance blocker. Live financial broadcast remains unavailable and fail-closed. It must not be enabled until a later, separately authorized owner decision validates the exact authority and action.
