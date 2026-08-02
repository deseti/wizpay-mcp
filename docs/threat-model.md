# Phase 0 threat model

## Trust boundaries and assets

Untrusted inputs include user/agent prompts, all MCP arguments, browser/UI input, provider responses, RPC responses, webhooks, and job deliveries. Security-critical assets are user identity, wallet binding, immutable intent/digest, approval decision, execution identity, policy, provider/chain evidence, audit integrity, and service credentials. Signing secrets are outside the system boundary and forbidden to store.

| Threat | Impact | Preventive control | Detective control | Recovery behavior |
|---|---|---|---|---|
| Prompt injection requests payment | unauthorized spend | authentication, schema/semantic validation, immutable intent, explicit digest-bound user approval; prompts have no authority | audit tool caller, intent, and approval boundary | reject; revoke exposed session; no execution |
| Forged MCP arguments | altered action | reject unknown fields; typed schemas; server resolves owner/wallet; execution accepts references only | validation metrics and rejected-argument audit | reject and require corrected request/new intent |
| Approval substitution | one approval authorizes another intent | exact intent ID/version/digest/user/wallet binding; atomic consumption | digest mismatch event | reject; invalidate suspicious session/binding |
| Wallet substitution | attacker wallet used | verified versioned binding; exact wallet ID/address/chain in digest | mismatch audit and binding verification checks | fail closed; stale/revoke binding; new intent |
| Recipient substitution | funds diverted | ordered recipients in canonical digest; adapter/verifier compares exact recipients | pre-submit and receipt mismatch events | no submit; if submitted, recovery and incident response |
| Amount substitution | over/underpayment | exact decimal/base-unit pair; totals; no floats; digest binding | normalization and receipt-total checks | fail closed; do not compensate automatically |
| Chain substitution | execution on wrong network | numeric chain ID in binding, intent, adapter allowlist, and verifier | provider/RPC chain-ID checks | stop; isolate adapter; incident response |
| Token substitution | wrong asset | chain+standard+address token identity; symbol non-authoritative | bytecode/config and receipt checks | fail closed; disable token/route |
| Replay attack | repeated spend | nonce/version, expiry, one approval consumption, unique intent-to-execution constraint | replay/conflict audit | return same execution or reject conflict |
| Duplicate execution | double payment | database uniqueness, deterministic provider key, leases/fencing, reconcile-before-submit | duplicate-delivery and provider lookup audit | recover same execution; never auto-create replacement |
| Stale quote/plan | adverse or invalid execution | quote ID, amounts, min output, route and expiry in digest; pre-submit expiry check | quote-expiry events | expire intent; create/approve new intent |
| Compromised provider response | false success/route manipulation | provider is not authority; allowlisted route; independent domain receipt verification | response fingerprint and cross-source discrepancy alerts | `recovery_required`; disable provider/route |
| Forged transaction hash | false completion | bind hash to chain and independently fetch/verify receipt/logs/state | hash/receipt mismatch audit | reject evidence; investigate caller/provider |
| RPC inconsistency/equivocation | false confirmation or balance | chain-ID check, confirmation/finality policy, multi-source checks where required | compare block/hash/receipt observations | pause in recovery until trustworthy evidence |
| Database compromise | intent/approval/execution tampering | least privilege, encryption, constraints, append-oriented audit, digest checks, backups | audit hash chaining/export, anomaly alerts, reconciliation | halt execution, restore/reconcile, rotate credentials |
| Redis compromise | lock bypass/rate-limit loss | Redis never authoritative; DB constraints and fencing | lock anomalies and duplicate attempt audit | ignore cache, rotate/flush safely, continue from DB |
| Worker duplicate delivery | double submission | at-least-once-safe handlers, DB lease/fence, stable execution ID | lease contention and attempt audit | losing worker exits; reconcile winner |
| Secret leakage | provider/service takeover | secret manager later, redaction, no secrets in prompts/errors/audit, least scope | scanning, access logs, credential-use alerts | revoke/rotate, assess unauthorized actions |
| Sensitive logging | privacy/auth compromise | allowlisted structured fields, hashing/tokenization, payload minimization | log sampling/DLP and retention audits | purge where lawful, rotate secrets, notify/respond |
| Privilege escalation / IDOR | cross-user wallet or intent access | server-derived principal, ownership checks at repository/service layers, least privilege | denied-access and unusual-enumeration alerts | terminate sessions, revoke access, incident response |
| Arbitrary contract-call injection | theft/approval abuse | no public raw-call tool; allowlisted contract/function/arguments per domain; verified ABI | attempted selector/address audit | reject; disable compromised route/config |
| Malicious approval UI | misleading approval or credential theft | canonical server summary, CSP/origin binding later, no signing-secret custody | UI/backend digest comparison and security telemetry | revoke sessions/bindings; new intent and approval |
| Approval race/revocation race | execution after intended revocation | atomic approval consumption/execution creation; revocation allowed only pre-boundary | ordered timestamps and CAS conflict audit | if consumed, reconcile same execution; never claim rollback |
| Policy bypass | prohibited payment | mandatory policy evaluation before approval and execution; policy cannot grant approval | policy version/decision audit | deny/halt before submit; incident review |
| Fee/treasury injection | hidden fee or custody | all amounts/fees/routes in intent; no treasury default; allowlist verifier | compare approved totals with receipt flows | fail verification; disable route; no automatic correction |

## Residual risk

Phase 0 does not implement controls. Every future vertical slice must turn its relevant preventive/detective controls into tested code and operational procedures before execution is enabled. Any missing verifier, uncertain external outcome, unverified deployment, or unsupported wallet behavior fails closed.

