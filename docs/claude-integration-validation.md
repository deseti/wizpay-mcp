# Claude connector implementation validation

## Result

Implementation complete and local non-browser verification complete. Public endpoint
activation and real Claude authentication/connectivity remain pending owner action.
No commit, push, remote GitHub modification, branch switch, public routing, deployment,
financial provider call, wallet transaction or existing WizPay service change occurred.
No Chromium, Chrome, Playwright, CDP, graphical browser or keyring session was started.

## Implemented features and artifacts

- Migration 012: immutable verified wallet proofs, separate attributed owner approvals,
  delegated ACTIVATE with replay/expiry/tenant/version locks, existing identity reuse,
  duplicate wallet prevention, proof referential/digest integrity and MCP admission audit.
- Production bootstrap: explicit default-off SIWE, enrollment/approval evidence required,
  full restricted EIP-4361 reconstruction and EOA verification, database authority clock,
  fail-closed clock errors and least-privilege runtime-role checks. Transaction clock
  reads work with a one-connection pool and do not acquire another connection.
- Onboarding operator: bounded ACTIVATE request with proof reference, separate TLS
  credential file and delegated database role; duplicate JSON owner decisions denied.
- Wallet UI: actual existing EIP-1193/SIWE/OAuth flow, enrollment-pending owner decision,
  fresh login after approval, explicit consent and clear missing/revoked enrollment errors.
- MCP: existing exact two-tool registry retained; tenant-bound reads, credential-free
  admission attribution and fail-closed audit. Forwarding headers rejected in backend.
- Claude: public-client preregistration SQL, exact official hosted callback, discovery
  and no-secret custom client instructions; no refresh token or public DCR endpoint.
- Deployment: separate operator image/context, TLS/database access policy overrides,
  explicit SIWE activation override, immutable-image validation, existing Tunnel ingress,
  trusted-proxy Nginx allowlist and offline configuration/HTTP validation scripts.
- Operations: updated backup socket user, rotation/recovery/storage instructions and
  complete owner runbook in [claude-custom-connector.md](claude-custom-connector.md).

## Commands executed and final results

From the repository root, all of these passed:

```sh
GOPROXY=off WIZPAY_ARC_INTEGRATION=0 WIZPAY_CIRCLE_INTEGRATION=0 go test -count=1 ./cmd/... ./internal/...
GOPROXY=off go test -count=1 ./internal/storage/postgres
GOPROXY=off go test -race -count=1 ./internal/requestauth ./internal/browser ./internal/siwe ./internal/oauth ./internal/storage/postgres -run 'TestMCPAudit|TestVerifiedEnrollment|TestProductionSIWE|TestAuthenticationClock|TestSIWEConcurrent|TestOAuthConcurrent|TestBrowserConcurrent|TestOnboarding'
GOPROXY=off go vet ./cmd/... ./internal/...
node --test deploy/local/config.test.mjs deploy/release/security.test.mjs
node deploy/release/validate-config.mjs
python3 deploy/tunnel/validate.py
cloudflared tunnel --config deploy/tunnel/config.yml.example ingress validate
cloudflared tunnel --config deploy/tunnel/config.yml.example ingress rule https://mcp.wizpay.xyz/mcp
cloudflared tunnel --config deploy/tunnel/config.yml.example ingress rule https://connect.wizpay.xyz/onboarding
cloudflared tunnel --config deploy/tunnel/config.yml.example ingress rule https://app.wizpay.xyz
git diff --check
```

The full Go run covered every cmd/internal package. PostgreSQL TestMain created fresh
isolated disposable databases; no existing Compose/E2E/production database was used.
Tests covered migrations, actual nonowner runtime login, delegated administration,
forbidden mutations, concurrent activation/redemption/rotation, replay, expiry,
revocation, tenant isolation, OAuth PKCE/resource bindings, MCP initialization/tools
and reads. The production enrollment flow uses the official hosted callback and a
one-connection runtime pool. Race-selected tests executed in requestauth/PostgreSQL;
other listed packages reported no matching tests and are not claimed as race coverage.
The deployment Node suite passed 21 tests. Nginx ran in a disposable nonroot container
with temporary certificates, mocked upstreams and loopback-only publication, then was
removed. Tunnel commands only parsed/matched ingress rules; no Tunnel was started.

From web/approval-ui, all of these passed (37 native Node tests):

```sh
./node_modules/.bin/tsc --noEmit --incremental false
./node_modules/.bin/tsc --target es2020 --module commonjs --moduleResolution node --esModuleInterop --skipLibCheck --strict --typeRoots node_modules/@types --outDir /tmp/wizpay-connector-node-tests tests/onboarding.test.ts
node /tmp/wizpay-connector-node-tests/tests/onboarding.test.js
NEXT_TELEMETRY_DISABLED=1 npm run build
```

Intermediate failures were corrected: old production-prohibition and late-rejection
expectations, duplicate-binding fixture behavior, mixed authentication clocks,
transaction clock reads requiring a second pool slot, the mock frontend upstream's
port, and a missing frontend test import. Final commands above pass. No expiry,
nonce, signature, Host or enrollment check was bypassed to obtain those results.

## Intentionally unexecuted stages and limits

Browser E2E **NOT RUN**. Real injected-wallet/mobile layout execution **NOT RUN**.
Real Claude **NOT RUN**. Real wallet enrollment/authentication **NOT RUN**.
Public Cloudflare routing **NOT RUN**. Release runtime/images/database-TLS startup
**NOT RUN**; templates were parsed, and Nginx TLS was tested with disposable test
certificates. PostgreSQL role/protocol tests used isolated test connections, not a
claim of release-host TLS/persistence. Off-host backup durability and restore on the
selected release host remain owner operations. Local tests do not prove Claude's
real callback exchange, Cloudflare header normalization or reconnect behavior.

## Minimal owner actions

Follow the exact commands and private-file requirements in the runbook:

1. Approve the isolated durable runtime; provide protected credentials, certificates,
   image digests, storage/backups and the approved tenant. Apply migration 012 and
   the separate runtime/admin role grants only to the new MCP database.
2. Preregister an owner-chosen public Claude client with the officially documented
   callback https://claude.ai/api/mcp/auth_callback, mcp:read, resource/audience binding
   and method none. No client secret is required or generated.
3. Stage the approved application identity and owner-controlled wallet; independently
   authorize public activation of the existing Tunnel using the reviewed MCP ingress.
   Do not change DNS or route existing WizPay/browser-test services.
4. In Claude, choose the MCP URL and Use your own OAuth client with that ID and blank
   secret. The wallet owner signs enrollment proof; the delegated owner consumes the
   resulting proof with ACTIVATE. Restart Connect, sign a fresh authentication message
   and separately grant read-only consent. Reconnect after token expiry; refresh tokens
   remain excluded. Verify exactly the two tools and revocation before claiming live
   integration. Never supply signing keys to the operator or agent.

## Repository snapshot

Branch: `main`

HEAD: `6ad3eafbadce8b360114d03deca576cde8fa95ab`

origin/main: `6ad3eafbadce8b360114d03deca576cde8fa95ab`

No commit or push occurred. Changes are local and uncommitted.

### git status --short

```text
 M README.md
 M cmd/onboarding-admin/main.go
 M cmd/onboarding-admin/main_test.go
 M cmd/server/main.go
 M deploy/release/backup.sh
 M deploy/release/onboarding-admin-role.sql
 M deploy/release/preflight.mjs
 M deploy/release/runtime-role.sql
 M deploy/release/security.test.mjs
 M docs/owner-onboarding-readiness.md
 M docs/wp3d-release-operations.md
 M internal/browser/siwe.go
 M internal/config/config.go
 M internal/config/config_test.go
 M internal/config/siwe_activation_test.go
 M internal/http/browser/handler.go
 M internal/http/browser/siwe.go
 M internal/http/oauth/handler.go
 M internal/http/oauth/handler_test.go
 M internal/oauth/service.go
 M internal/onboarding/request.go
 M internal/requestauth/middleware.go
 M internal/siwe/siwe.go
 M internal/storage/postgres/browser_repository.go
 M internal/storage/postgres/oauth_repository.go
 M internal/storage/postgres/postgres_integration_test.go
 M internal/storage/postgres/protocol_e2e_helpers_test.go
 M internal/storage/postgres/protocol_negative_integration_test.go
 M internal/storage/postgres/siwe_integration_test.go
 M internal/storage/postgres/siwe_repository.go
 M internal/storage/postgres/store.go
 M web/approval-ui/src/components/onboarding-ui.tsx
 M web/approval-ui/src/lib/onboarding-api.ts
 M web/approval-ui/src/lib/onboarding-flow.ts
 M web/approval-ui/tests/onboarding.test.ts
?? db/migrations/000012_verified_enrollment.up.sql
?? deploy/release/Operator.Dockerfile
?? deploy/release/Operator.Dockerfile.dockerignore
?? deploy/release/app.env.example
?? deploy/release/pg_hba.conf.example
?? deploy/release/pg_ident.conf
?? deploy/release/postgres-tls.override.yaml
?? deploy/release/register-claude-client.sql
?? deploy/release/siwe.override.yaml
?? deploy/release/validate-config.mjs
?? deploy/tunnel/
?? docs/claude-custom-connector.md
?? docs/claude-integration-validation.md
?? internal/requestauth/access_audit.go
?? internal/requestauth/access_audit_test.go
?? internal/storage/postgres/authentication_clock.go
?? internal/storage/postgres/mcp_access_audit.go
?? internal/storage/postgres/runtime_login_test.go
?? internal/storage/postgres/runtime_security.go
?? internal/storage/postgres/verified_enrollment_integration_test.go
```

### Modified tracked files

```text
README.md
cmd/onboarding-admin/main.go
cmd/onboarding-admin/main_test.go
cmd/server/main.go
deploy/release/backup.sh
deploy/release/onboarding-admin-role.sql
deploy/release/preflight.mjs
deploy/release/runtime-role.sql
deploy/release/security.test.mjs
docs/owner-onboarding-readiness.md
docs/wp3d-release-operations.md
internal/browser/siwe.go
internal/config/config.go
internal/config/config_test.go
internal/config/siwe_activation_test.go
internal/http/browser/handler.go
internal/http/browser/siwe.go
internal/http/oauth/handler.go
internal/http/oauth/handler_test.go
internal/oauth/service.go
internal/onboarding/request.go
internal/requestauth/middleware.go
internal/siwe/siwe.go
internal/storage/postgres/browser_repository.go
internal/storage/postgres/oauth_repository.go
internal/storage/postgres/postgres_integration_test.go
internal/storage/postgres/protocol_e2e_helpers_test.go
internal/storage/postgres/protocol_negative_integration_test.go
internal/storage/postgres/siwe_integration_test.go
internal/storage/postgres/siwe_repository.go
internal/storage/postgres/store.go
web/approval-ui/src/components/onboarding-ui.tsx
web/approval-ui/src/lib/onboarding-api.ts
web/approval-ui/src/lib/onboarding-flow.ts
web/approval-ui/tests/onboarding.test.ts
```

### Untracked files

```text
db/migrations/000012_verified_enrollment.up.sql
deploy/release/Operator.Dockerfile
deploy/release/Operator.Dockerfile.dockerignore
deploy/release/app.env.example
deploy/release/pg_hba.conf.example
deploy/release/pg_ident.conf
deploy/release/postgres-tls.override.yaml
deploy/release/register-claude-client.sql
deploy/release/siwe.override.yaml
deploy/release/validate-config.mjs
deploy/tunnel/config.yml.example
deploy/tunnel/nginx.conf
deploy/tunnel/validate.py
docs/claude-custom-connector.md
docs/claude-integration-validation.md
internal/requestauth/access_audit.go
internal/requestauth/access_audit_test.go
internal/storage/postgres/authentication_clock.go
internal/storage/postgres/mcp_access_audit.go
internal/storage/postgres/runtime_login_test.go
internal/storage/postgres/runtime_security.go
internal/storage/postgres/verified_enrollment_integration_test.go
```
