# WP3D-4 / WP3D-5 — private release-candidate operations

2026-10-10. PUBLIC RELEASE IS NO-GO. These are reviewed operator procedures, not
executed deployment evidence. Do not run against the existing browser/local project
or another application. The new profile's project is exactly wizpay-mcp-rc.

## Build and configuration, only after owner approval

Run the following only after private-build approval, from the repository root in
Bash with `set -euo pipefail`. These commands have not been executed for this RC.
They do not start the application or authorize runtime/ingress. Use a separate
compatible Linux builder; do not rebuild or replace any browser-E2E artifacts.
Required beforehand: Go 1.26.3 and its cached modules/compiler dependencies, Node
22.22.2/npm with all lockfile packages cached, compatible architecture/glibc, and
reviewed cached base images. Missing cache is a blocker: dependency acquisition
needs its own owner review. Never substitute a browser-E2E application image.

Set GO_BASE_IMAGE to the reviewed digest of the compatible Go Bookworm base and
FRONTEND_BASE_IMAGE to the reviewed digest of the Node Bookworm slim base. Both
must be repository@sha256 references; no default or invented digest is supplied.
Set RC_TAG to a new owner-reviewed local artifact label (not an existing E2E tag).
Record HEAD, the reviewed working-tree diff, lockfiles, tool versions, platform,
base digests and output image identities with the candidate evidence.

```sh
set -euo pipefail
: "${RC_TAG:?new private RC artifact label required}"
node deploy/release/preflight.mjs --build
umask 077
mkdir -p deploy/release/runtime
chmod 700 deploy/release/runtime
# Refuse to overwrite prior release frontend artifacts. No local .next/node_modules reuse.
mkdir deploy/release/runtime/frontend
git ls-files -z web/approval-ui | tar --null -T - -cf - | \
  tar --strip-components=2 -xf - -C deploy/release/runtime/frontend
CGO_ENABLED=1 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go build -o deploy/release/runtime/server ./cmd/server
CGO_ENABLED=1 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go build -o deploy/release/runtime/healthcheck ./cmd/healthcheck
CGO_ENABLED=1 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go build -o deploy/release/runtime/dbmigrate ./cmd/dbmigrate
CGO_ENABLED=1 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go build -o deploy/release/runtime/dbmaintenance ./cmd/dbmaintenance
(
  cd deploy/release/runtime/frontend
  npm ci --offline --no-audit --no-fund
  NEXT_PUBLIC_APPROVAL_API_BASE_URL='' NEXT_TELEMETRY_DISABLED=1 npm run build
)
# Fresh nonsensitive image artifacts must be readable by the unprivileged runtime.
# This never chmods database credentials, TLS keys or other runtime files.
chmod -R a+rX deploy/release/runtime/frontend
chmod 0555 deploy/release/runtime/server deploy/release/runtime/healthcheck \
  deploy/release/runtime/dbmigrate deploy/release/runtime/dbmaintenance
docker build --pull=false --network=none --build-arg GO_BASE_IMAGE="$GO_BASE_IMAGE" \
  -f deploy/release/Go.Dockerfile -t "wizpay-mcp-rc-go:$RC_TAG" .
docker build --pull=false --network=none --build-arg FRONTEND_BASE_IMAGE="$FRONTEND_BASE_IMAGE" \
  -f deploy/release/Frontend.Dockerfile -t "wizpay-mcp-rc-frontend:$RC_TAG" .
docker image inspect --format '{{.Id}} {{json .RepoDigests}}' \
  "wizpay-mcp-rc-go:$RC_TAG" "wizpay-mcp-rc-frontend:$RC_TAG"
```

The frontend source snapshot includes tracked working-tree files only: review the
source list/diff first; do not put secrets in tracked files. No `.env`, local
node_modules, local `.next`, fixture, TLS or DB files are copied from the E2E
workspace. Each Dockerfile has its own deny-by-default context. The frontend
image admits only package.json, fresh dependencies and Next output (excluding
build cache); no source/signing service or operator binary is included. There are
currently no frontend public assets; adding them requires reviewing the release
COPY/context allowlist. No Docker build, npm install or image inspection was run
as part of this readiness change.

Local build tags/image IDs are NOT repository manifest digests. A local Docker
build may have empty RepoDigests. Stop before runtime until an owner-approved
private artifact distribution process produces and verifies repository@sha256
references available locally; registry push/publication is not authorized here.
Do not manufacture a RepoDigest from an image ID or relax preflight to accept tags.
Supply GO_IMAGE, FRONTEND_IMAGE, POSTGRES_IMAGE, NGINX_IMAGE as reviewed immutable
references. Review the PostgreSQL major/version and volume layout together; do
not assume a new major works with the existing data directory. RELEASE_UID/GID
must match the nonroot TLS file owner. Compose secrets are read-only files, not an
encrypted secret manager: protect the host.

Prepare database_url (runtime role), database_owner_password (bootstrap only),
and an operator-only migration_url under ignored runtime through protected tooling.
No credentials belong in shell command arguments, history, source or logs. Ensure
the Go UID/GID 10001 can read its mounted URL file (e.g. reviewed owner/group mode
0440), but no other service receives it. Do not make private TLS keys world-readable.
Owner must inspect effective mounted modes; Compose bind-backed secret uid/gid
settings are not assumed to fix source permissions. TLS files must have valid SANs
and a reviewed public chain before any public release; self-signed browser-test
certificates do not satisfy public activation.

```sh
RELEASE_STORAGE_APPROVED=yes RELEASE_RUNTIME_APPROVED=yes node deploy/release/preflight.mjs
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml config --quiet
# Explicitly approved private RC only, after host persistence approval:
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml up -d --pull never --no-build postgres
```

Before creating state, establish that the host's storage persists across process,
VM and service replacement and that off-host backups are possible. A named Docker
volume inside an expiring sandbox does not satisfy this requirement. Do not start
the private RC on the cloud sandbox merely to bypass that gate. The storage
approval flag acknowledges independently reviewed evidence; it does not prove
durability. Preflight is an operator gate, not protection against invoking Docker
directly. Check that loopback port 8443 is unused without stopping any existing
service; a collision blocks startup pending a separately reviewed RC mapping.

## Separate migrations and runtime privileges

Serialize migration jobs; Migrate does not provide a distributed deployment lock.
Take and verify an encrypted backup before changing an existing schema. Use the
operator binary with the migration URL, never grant application DDL ownership.
Example owner-run migration container, assuming the isolated project already
exists and the migration binary has executable mode 0555:

```sh
docker run --rm --pull never --network wizpay-mcp-rc_private \
 --read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges \
 --user 10001:10001 --entrypoint /tool \
 --mount "type=bind,src=$PWD/deploy/release/runtime/dbmigrate,dst=/tool,readonly" \
 --mount "type=bind,src=$PWD/deploy/release/runtime/migration_url,dst=/run/secrets/database_url,readonly" \
 -e DATABASE_URL_FILE=/run/secrets/database_url "$GO_IMAGE"
```

Only this short-lived operator process receives the owner URL. Create the runtime
role interactively, without embedding a password:

```sh
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml exec postgres \
 psql -X -v ON_ERROR_STOP=1 -U wizpay_migrator -d wizpay_mcp
```

Inside protected psql: `CREATE ROLE wizpay_runtime LOGIN NOSUPERUSER NOCREATEDB
NOCREATEROLE NOREPLICATION NOBYPASSRLS;` then `\password wizpay_runtime` using the
owner's protected credential workflow. Do not reuse the bootstrap password.

```sh
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml exec -T postgres \
 psql -X -v ON_ERROR_STOP=1 -U wizpay_migrator -d wizpay_mcp \
 < deploy/release/runtime-role.sql
```

This grants auth/session mutation but no identity/tenant/binding activation or
financial mutation. PostgreSQL SELECT FOR UPDATE/SHARE also requires an UPDATE
privilege: narrowly scoped column grants use trigger-immutable tenant creation time,
wallet binding ID and OAuth client ID, plus identity updated_at metadata. Status,
ownership and lifecycle-version mutation remain unavailable. Never replace these
with table-wide UPDATE grants. Runtime-role validation is not yet executed: prove startup,
browser/OAuth reads/writes and forbidden DDL/tenant UPDATE/intents INSERT under
the actual role in a disposable regression DB before production. The privileged
bootstrap owner remains operator-only; database superuser restrictions and stored
function ownership must be reviewed. No automatic user or client registration is
provided by this profile. Production runtime checks exact migrations without DDL.

## Start private candidate and observe

```sh
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml up -d --no-build --pull never go frontend
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml run --rm --no-deps --pull never nginx -t -p /work/ -c nginx.conf
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml up -d --no-build --pull never nginx
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml ps
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml port nginx 8443
```

Expect exactly 127.0.0.1:8443. Upstreams/database have no published ports. Readiness
pings PostgreSQL; process restart waits 30 seconds for graceful shutdown. Docker
marks unhealthy containers but does NOT automatically restart them just because
health fails. Configure approved operator alerting for unhealthy state/restart
loops, backend readiness failure, sustained 429/5xx, disk space, TLS expiry and
backup age. Logging is bounded structured JSON; do not log full URLs with codes,
Authorization, cookies, CSRF, request bodies or database URLs. Aggregate in-process
admission is not distributed security. Do not publicly expose health endpoints.

## Backup, restore and retention

```sh
# Requires an already-installed approved GPG tool and recipient public key.
RELEASE_BACKUP_APPROVED=yes BACKUP_RECIPIENT=OWNER_APPROVED_PUBLIC_RECIPIENT \
 bash deploy/release/backup.sh
```

Backup streams custom-format pg_dump directly into GPG encryption with pipefail,
mode-600 files and a SHA-256 manifest. No plaintext dump is written. Transfer
encrypted backups and authenticated manifests off the host using an approved
owner-controlled destination; none is chosen or contacted here. Suggested initial
operational objectives require owner approval: daily backups, RPO <=24h, measured
restore RTO <=1h and at least weekly off-host restore drills. Also test a backup
immediately before each migration. Final retention/legal policy remains undecided.

Restore ONLY into a newly approved empty `wizpay_mcp_restore` database:

```sh
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml exec -T postgres \
 createdb -U wizpay_migrator wizpay_mcp_restore
# Owner's GPG private-key operation, never performed by the agent:
gpg --decrypt deploy/release/runtime/backups/OWNER_BACKUP.dump.gpg | \
 docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml exec -T postgres \
 pg_restore -U wizpay_migrator -d wizpay_mcp_restore --no-owner --exit-on-error
```

Use shell pipefail. Verify archive integrity, exact migration set, FK integrity,
record counts, revocation state and read authorization using a separate isolated
candidate configured for the restore DB. Do not overwrite the live database or
switch DATABASE_URL until a separately approved recovery cutover. Existing opaque
token digests and immutable auth evidence are sensitive backup data. Production
recovery policy must revoke restored stale authority to prevent revocation loss
since the backup; this requires an owner-approved incident decision and operation.
The prepared restore-revoke.sql refuses any database except wizpay_mcp_restore
and revokes browser, OAuth session, consent and token authority atomically. It
preserves immutable financial records and challenge evidence. After incident approval:

```sh
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml exec -T postgres \
 psql -X -v ON_ERROR_STOP=1 -U wizpay_migrator -d wizpay_mcp_restore \
 < deploy/release/restore-revoke.sql
```

Prove all restored credentials fail before cutover; restore and invalidation have
not been executed here. Previously consumed codes may appear unconsumed in an old
backup, but revoked associated sessions must prevent their redemption.

cmd/dbmaintenance may run with the runtime URL in the same short-lived container
pattern as dbmigrate. It deletes only eligible anonymous pending tombstones older
than the existing 24-hour threshold with no challenge references. It does not solve
retention of consumed challenges or immutable authenticated evidence. No broad
DELETE, trigger disabling, destructive migration or down migration is approved.

## Rollback and public release gates

Keep previous reviewed image digests and the encrypted pre-change backup off-host.
For a binary-only rollback, set GO_IMAGE/FRONTEND_IMAGE to the previous reviewed
compatible digests, rerun preflight/config validation, then owner-run:

```sh
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml up -d \
 --no-deps --no-build --pull never go frontend
```

Do not reverse applied migrations or restore a backup as a casual binary rollback.
Version-set validation rejects older binaries if migrations differ; equal migration
versions alone do not prove semantic compatibility. Review compatibility first.

Only AFTER hosting/activation/client/security approval, public smoke commands:

```sh
curl --fail --silent --show-error https://connect.wizpay.xyz/.well-known/oauth-authorization-server
curl --fail --silent --show-error https://mcp.wizpay.xyz/.well-known/oauth-protected-resource/mcp
curl --silent --show-error -o /dev/null -w '%{http_code}\n' https://mcp.wizpay.xyz/mcp
```

Expect metadata 200 and unauthenticated MCP 401; trusted TLS verification must stay
enabled. Authenticated discovery/read smoke tests require separately authorized
user/client credentials through protected tooling, never token arguments or traces.
Expected external tools remain exactly get_intent/get_approval; every other name
must fail. Production SIWE remains disabled in this candidate. No public onboarding
release is possible until separate activation approval and secure real-account
policy are complete. No public ingress/DNS commands can be responsibly supplied
until the platform proves canonical domains, Host/SNI and durability support.

Final acceptance: actual Chrome flow and negative cases; approved real-client
evidence; reviewed DB privileges; backup/restore/incident revocation; measured
capacity; trusted canonical public TLS; monitoring; no synthetic signer or test
fixtures mounted; no financial/provider execution; explicit owner GO. Otherwise
NO-GO. No expiration-renewal workaround or alternate hosting provider is approved.


## Readiness update — 2026-10-12, baseline a3395b4

Subsequent owner-controlled enrollment preparation is documented in
[owner-onboarding-readiness.md](owner-onboarding-readiness.md). It adds pending
enrollment and terminal revocation only; production SIWE activation remains
blocked on the enrollment trust policy. Migration 011 and operator tooling have
not been run against any release or E2E database.

The owner reports WP3D-2D real HTTPS browser E2E and final security audit passed.
That evidence covers the isolated synthetic environment, not these new release
images, production authentication, a real wallet or a Claude connection. This
update supersedes historical browser-blocked wording for that reported evidence
only. Public integration remains NO-GO.

| Remaining gate | Required evidence / action before integration |
|---|---|
| Production authentication | `internal/config/config.go` rejects SIWE in staging/production; Compose intentionally leaves it false. `cmd/server/main.go` installs no browser verifier when disabled. A separately authorized activation design, implementation and negative tests are required. Setting APP_ENV=test/development on public ingress is forbidden. |
| Legitimate identity and wallet | SIWE currently verifies only EOA signatures, with chain ID 5042 and network MAINNET hardcoded in the existing profile. `siwe_repository.go` requires an active identity/tenant and exactly one active, verified, unrevoked EXTERNAL_EVM binding with verification evidence. The owner must approve the network and a real identity/binding provisioning and revocation process. No new-user registration, smart-contract-wallet verification or fixture promotion is supplied. |
| Claude registration | Confirm the account actually exposes custom connectors/own OAuth client and the operator has permission. Provision a distinct reviewed public client through the existing admin boundary, exact hosted callback `https://claude.ai/api/mcp/auth_callback`, method none, scope mcp:read, resource `https://mcp.wizpay.xyz/mcp`. No client ID or registration is invented here; runtime DB cannot register clients. |
| Images and persistent host | Fresh separate image evidence, cached immutable runtime references, compatible ABI/architecture, durable storage beyond sandbox expiry and off-host encrypted recovery. No E2E databases, credentials, certificates or signer assets. |
| Private acceptance | Exact schema, actual runtime-role positive and forbidden-operation checks, production guard failures, fresh-image readiness and TLS checks, backup/restore and stale-authority rejection. Review alerts/capacity and rollback evidence. Existing E2E passes do not prove these. |
| Canonical ingress | Separate owner authorization for DNS/Tunnel routing/connection. Prove both canonical hosts, public trusted TLS, origin TLS verification and canonical Host/SNI; expose only approved routes. The current local proxy rejects incoming Forwarded and X-Forwarded-* headers, so a trusted-edge header policy must be designed and tested before Tunnel use. Do not remove those checks as a workaround. Existing DNS and Tunnel existence prove none of this. |
| Controlled acceptance | Authorize one account/wallet and SIWE signing separately, only read-only MCP discovery/get_intent/get_approval, negative scope/tool/tenant checks, token-expiry reconnect and logout/revocation. Bound the test window and approve ingress shutdown and credential revocation afterward. No transactions or autonomous execution. |

Current official Claude [custom connector instructions](https://claude.com/docs/connectors/custom/add-unlisted)
document Sign in now / Use your own OAuth client (or Advanced settings), with a
blank secret for a public client. [Authentication requirements](https://claude.com/docs/connectors/building/authentication)
document the hosted callback above and S256 PKCE. This is the intended controlled
path, not evidence of this account's entitlement or successful interoperability.
Do not select DCR/CIMD or static bearer credentials as a substitute. Existing
WizPay token exchange requires server-side form POST, no Cookie/Origin/Authorization
headers, exact resource, and a valid verifier. Tokens last at most ten minutes;
refresh is unsupported, so actual expiry/reconnection behavior remains an
acceptance gate. Implementing DCR/CIMD or refresh is not required for preparing
this candidate and is outside this change.

Local readiness checks (no Docker, database or browser):

```sh
node --test deploy/release/security.test.mjs deploy/local/config.test.mjs
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test ./internal/config ./cmd/server
git diff --check
```
