# WizPay MCP: owner activation and Claude Custom Connector

Code uses the existing OAuth/browser/identity/storage foundations. Public release
is **not activated** by source changes. This guide supersedes the previous
pending-only onboarding and production-SIWE prohibition. No real wallet enrollment,
public routing or real Claude connection is asserted.

## Policy and evidence

The owner controls **application identity entitlement**, not legal identity.
STAGE approves an existing identity's exact issuer/subject or creates an explicitly
approved UNKNOWN application identity, with a PENDING external EOA binding. It
never grants access. No real-world identity provider or document verification is
implemented; wallet control does not identify a legal person.

With SIWE explicitly configured, only staged/active wallets in the configured
tenant can request a challenge. A staged wallet signs an **ENROLL** EIP-4361
message, with canonical domain/URI, the repository's fixed chain profile, random
nonce, two-minute expiry, browser session, CSRF and OAuth transaction binding.
The trusted backend cryptographically verifies it and records an immutable proof
reference, SHA-256 message digest, binding version and database verification time.
Signatures, keys and raw session/token credentials are not stored. The UI reports
ENROLLMENT_PENDING and closes the pending browser session. No authentication,
OAuth code, consent or token results from this proof.

A separately delegated owner login calls ACTIVATE within 24 hours of proof.
It must name the exact tenant/user/binding/proof and a reviewed application
entitlement reference. The reference attributes the owner decision; it is **not**
cryptographic evidence. The immutable approval records session_user, review,
request and time. Tenant and identity become ACTIVE, and the binding advances
exactly one version. Duplicate addresses within a tenant/profile, conflicting
identity attribution, replay, cross-tenant approval, expired delegation and
terminal revocation fail. Existing database duplicates cause migration failure;
reconcile explicitly rather than discarding history.

Runtime can verify proof but cannot activate any binding. Delegated administration
can approve existing proof but cannot insert/read proof tables or mutate authority
tables directly. The migration owner remains a highly privileged trust boundary;
protect its credentials and never install them in the server/frontend. Production
startup rejects a privileged runtime login. Compromise of the trusted authentication
runtime or migration owner is outside the wallet proof's protection.

After approval, restart from Claude. The ACTIVE wallet signs a **fresh AUTHENTICATE**
message. Production bootstrap always requires server-verified enrollment **and**
owner approval, not arbitrary verification metadata. The verifier reconstructs the
complete restricted EIP-4361 message and validates the EOA signature, address and
time after locks. Browser rotation precedes separate explicit OAuth consent. OAuth/browser runtime
and SIWE share the database authority clock; clock lookup failures deny access
and never fall back to a host clock.
Every subsequent SIWE-derived session/code/token use rechecks identity, tenant,
binding status/version and revocation. Revoked authority cannot be restored.
In-flight request cancellation after a concurrent revocation is not claimed.

OAuth remains Authorization Code, S256, public client method `none`, exact redirect,
resource/audience `https://mcp.wizpay.xyz/mcp`, and only `mcp:read`. Access tokens are
short-lived; no refresh tokens or offline_access are supported. Reconnect after
expiry/session expiry. Exactly `wizpay.get_intent` and `wizpay.get_approval` are
externally registered. MCP admission records contain generated request ID, HTTP
method and verified tenant/user/client attribution, never token/header/body data.
RECEIVED and AUTHORIZED mean admission attempts, not successful tool results.
Audit persistence failure blocks admission. Proxy/security rejection before MCP
admission does not produce an authenticated admission record.

## Official Claude compatibility

Verified against current official documentation on 2026-10-12:

- [Claude connector authentication](https://claude.com/docs/connectors/building/authentication)
  documents public clients with a blank optional secret, S256, hosted callback
  **https://claude.ai/api/mcp/auth_callback**, and discovery through a 401 challenge.
- [Custom connector setup](https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp)
  offers **Use your own OAuth client**. Use this option, not automatic registration
  or published identity. Availability can depend on the owner's Claude interface.

This implementation preregisters clients through an owner-only SQL procedure;
there is no public registration endpoint, CIMD fetching, client secret or refresh
endpoint. Metadata accurately advertises supported features. If the actual setup
interface cannot select a supplied public client with no secret, stop and capture
the nonsecret configuration requirements for review; do not invent credentials or
weaken client validation. This guide targets hosted Claude Custom Connectors, not
Claude Code's variable loopback redirects. Official docs discuss refresh support;
this intentionally restricted profile requires reauthentication and must be
confirmed with the real client before claiming continuous connectivity.

## Prepare the isolated release (no public activation)

Use only `deploy/release`, project **wizpay-mcp-rc**, and a new independent database
volume. Never reuse WizPay Core or browser-E2E runtime, fixtures, signer or volumes.
A temporary Docker sandbox is not durable hosting. Owner approval of a durable host,
volume backup and recovery is needed for activation. Keep all secret/artifact files
under ignored `deploy/release/runtime/` or a protected external host directory.

1. Review source, migration 012 and validation results. Build fresh artifacts from
   this working tree. No test source or signer enters the server image:

   ```sh
   mkdir -p deploy/release/runtime
   CGO_ENABLED=0 GOPROXY=off go build -o deploy/release/runtime/server ./cmd/server
   CGO_ENABLED=0 GOPROXY=off go build -o deploy/release/runtime/healthcheck ./cmd/healthcheck
   CGO_ENABLED=0 GOPROXY=off go build -o deploy/release/runtime/onboarding-admin ./cmd/onboarding-admin
   CGO_ENABLED=0 GOPROXY=off go build -o deploy/release/runtime/dbmigrate ./cmd/dbmigrate
   ```

   Copy the freshly built frontend `.next`, installed locked dependencies and
   package.json into `runtime/frontend/` as described in the release operations
   guide. Set reviewed immutable GO_BASE_IMAGE/FRONTEND_BASE_IMAGE references;
   use release Go/Frontend/Operator Dockerfiles, never test image contexts. Build
   with `--network=none --pull=false`; record resulting image digests. Supply
   GO_IMAGE, FRONTEND_IMAGE, OPERATOR_IMAGE, POSTGRES_IMAGE and NGINX_IMAGE as
   immutable `repository@sha256:...` references in a private release env file.
   All must be present locally; no Docker Cloud Sandbox durability is presumed.

2. Prepare secrets through owner-controlled tooling. Never send them to an agent.
   Required files: `database_owner_password`, `database_url`,
   `migration_database_url`, `onboarding_database_url`, `postgres/ca.pem`,
   `postgres/server.crt`, `postgres/server.key`, `postgres/pg_hba.conf`,
   `tls.crt` and `tls.key`. Database URLs use DNS name `postgres`, database
   `wizpay_mcp`, `sslmode=verify-full` and
   `sslrootcert=/run/secrets/database_ca`, with distinct runtime, migration and
   dedicated administration logins. No plaintext TLS fallback. The database server
   certificate must cover `postgres`; ingress certificate must cover both canonical
   hosts. Do not use the disposable validation certificates for release.
   The admin URL must be a regular mode-0600 file owned by UID 10001; runtime URL
   files must be readable only by their intended container UID. PostgreSQL key:
   database-image postgres UID with 0600, or root with its postgres group and 0640.
   Ingress key belongs to RELEASE_UID/RELEASE_GID with 0600. Keep CA/certificates
   nonsecret but integrity-protected. Copy/review pg_hba.conf.example; pg_ident.conf
   limits the migration-owner local socket to the postgres OS account.

3. After isolated runtime authorization, use all three Compose files consistently:

   ```sh
   mcp_release() {
     docker compose --env-file /OWNER_PRIVATE_PATH/release.env -p wizpay-mcp-rc \
       -f deploy/release/compose.yaml -f deploy/release/postgres-tls.override.yaml \
       -f deploy/release/siwe.override.yaml "$@"
   }
   mcp_release config --quiet
   ```

   The base profile still disables SIWE. The explicit override enables only the
   verifier for WIZPAY_ONBOARDING_TENANT_ID; PENDING wallets can prove control but
   cannot authenticate. The server's role/schema checks and row-level lifecycle
   prerequisites remain mandatory. Use `node deploy/release/preflight.mjs` with the
   approved private environment (runtime/storage approvals and immutable references)
   before starting services. The operator image is separate and profile-gated.

   Start **postgres only** with `mcp_release up -d postgres`, then run
   `mcp_release --profile operator run --rm -T migrate`. Provision `wizpay_runtime` and
   `wizpay_onboarding_admin` as dedicated LOGIN roles (NOSUPERUSER, NOCREATEDB,
   NOCREATEROLE, NOREPLICATION, NOBYPASSRLS, no memberships) and passwords through
   protected owner tooling. Apply runtime-role.sql as migration owner using the
   private postgres socket. Grant one finite tenant delegation using
   onboarding-admin-role.sql, with admin_role, tenant_id and delegation_expires
   psql variables. No authority or test identity is seeded. For example:

   ```sh
   mcp_release exec -T --user postgres postgres psql -U wizpay_migrator -d wizpay_mcp \
     -v ON_ERROR_STOP=1 < deploy/release/runtime-role.sql
   mcp_release exec -T --user postgres postgres psql -U wizpay_migrator -d wizpay_mcp \
     -v admin_role=wizpay_onboarding_admin -v tenant_id=OWNER_APPROVED_TENANT \
     -v delegation_expires=OWNER_REVIEWED_UTC_DEADLINE < deploy/release/onboarding-admin-role.sql
   mcp_release exec -T --user postgres postgres psql -U wizpay_migrator -d wizpay_mcp \
     -v client_id=OWNER_CHOSEN_PUBLIC_ID -v callback_uri=https://claude.ai/api/mcp/auth_callback \
     < deploy/release/register-claude-client.sql
   ```

   Only after schema/role/client provisioning, start the independent MCP components
   with `mcp_release up -d go frontend nginx`. Public
   ingress blocks /health, /readiness, /approval, debug and arbitrary /browser or
   /oauth routes. Internal health checks stay on the private Docker network.

## Owner enrollment ceremony

Use the separate operator service with `ONBOARDING_ADMIN_APPROVED=yes` in the
private operator environment. Pipe a bounded, reviewed **nonsecret** JSON request
into `mcp_release --profile operator run --rm -T onboarding-admin`.

STAGE input fields: action=STAGE, unique request_id, tenant_id, user_id, binding_id,
issuer, subject, canonical checksum address, review_reference, and expires_at
(an actual UTC deadline no more than 15 minutes ahead). issuer/subject must exactly
match an existing identity; otherwise this creates the explicitly approved
application identity. Reference only the entitlement decision, never sensitive
identity documents. Do not provision arbitrary identities for a successful demo.

Once the owner authorizes public routing below, the wallet owner starts Connect
in Claude, explicitly connects the injected EIP-1193 wallet, selects the reviewed
network, and signs the displayed enrollment message. The page shows a nonsecret
verification reference and waits for owner approval. No financial action is signed.

ACTIVATE input fields: action=ACTIVATE, a new request_id, the same tenant_id,
user_id, binding_id, verification_id from the page, review_reference attributing
an independent owner entitlement decision, and a fresh expires_at deadline.
Do not include issuer/subject/address. Approval requires the proof to be less than
24 hours old. The owner can inspect immutable proof/approval/audit records through
the protected migration-owner boundary; the delegated login has no table access.

After activation, restart Connect from Claude and sign the new authentication
message; then separately select **Grant read-only access**. Revocations use existing
REVOKE_TENANT/REVOKE_IDENTITY/REVOKE_BINDING requests and retain historical evidence.
Remove/expire administrator delegation when no longer needed.

## Existing Tunnel: activation only after explicit approval

Do not create another Tunnel or change DNS. Use existing UUID
`540a7a13-67fe-4787-a1cc-13e6ff2b2de5`, name `wizpay-mcp`.
Copy config.yml.example to a protected external path and replace only the absolute
credentials-file and caPool paths. Credential JSON stays private, outside every
image/context. Use the existing credential file; do not reveal its contents.

[Cloudflare origin settings](https://developers.cloudflare.com/tunnel/reference/origin-parameters/)
provide verified origin TLS with originServerName, httpHostHeader and caPool.
The template matches only mcp/connect and denies other hostnames. Nginx checks
canonical Host **and** SNI, publishes only 127.0.0.1:8443, rejects Forwarded and host,
port, server, original-host and real-IP override headers. On the trusted Tunnel
listener it requires HTTPS proto and a single X-Forwarded-For value equal to
CF-Connecting-IP. [Cloudflare header behavior](https://developers.cloudflare.com/fundamentals/reference/http-headers/)
adds this matching pair for requests without preexisting XFF; appended/spoofed XFF
chains fail. Cloudflare's overwritten proto is only checked against the fixed
HTTPS value; it never defines application origin. All proxy/CF headers are removed
before the backend. Other transport metadata is ignored and dropped.

This boundary trusts the reviewed cloudflared process and Cloudflare edge. Do not
put another proxy/Worker in this path or publish the listener beyond loopback.
Local host administrators remain trusted. The local direct-test nginx config is
separate and rejects all forwarding headers. Live Cloudflare normalization is
**UNVERIFIED** until the owner-approved ingress smoke test; no live routing was run.

Offline commands (safe now, no connection to Cloudflare):

```sh
cloudflared tunnel --config deploy/tunnel/config.yml.example ingress validate
cloudflared tunnel --config deploy/tunnel/config.yml.example ingress rule https://mcp.wizpay.xyz/mcp
cloudflared tunnel --config deploy/tunnel/config.yml.example ingress rule https://connect.wizpay.xyz/onboarding
cloudflared tunnel --config deploy/tunnel/config.yml.example ingress rule https://app.wizpay.xyz
```

**Only after explicit public activation approval**, on the durable reviewed host:

```sh
cloudflared tunnel --no-autoupdate --config /OWNER_PRIVATE_PATH/wizpay-mcp.yml \
  --loglevel warn run 540a7a13-67fe-4787-a1cc-13e6ff2b2de5
```

Do not run route dns. Do not start another replica if an unreviewed existing replica
would split traffic. Review/stop only this Tunnel's obsolete MCP replica under
owner authorization. Never alter the existing WizPay services, zone mail records,
Vercel deployments or browser E2E containers. Do not enable debug/header logging.

## Minimal Claude connection procedure

After the preceding owner-approved provisioning/enrollment/routing:

1. Customize → Connectors → Add custom connector. Name: WizPay MCP. URL:
   **https://mcp.wizpay.xyz/mcp**. For organization setup, use the corresponding
   owner Connectors settings.
2. Choose OAuth sign-in and **Use your own OAuth client**. Enter the public client
   ID registered above. Leave secret blank. Do not use automatic registration,
   published identity, static bearer headers or authless mode.
3. Connect. Explicitly select/connect the external wallet, review the fixed network,
   sign the server authentication message, and separately grant `mcp:read` consent.
4. Confirm discovery contains only wizpay.get_intent and wizpay.get_approval. Read an
   existing authorized record; reject a different tenant's ID. Do not create a
   financial record or execute a transaction to supply test data.
5. Reconnect after token expiry. Verify revocation rejection before claiming real
   Claude integration. Successful local protocol tests are not a real-Claude result.

## Rotation, backups and recovery

Rotate each DB role password independently through protected tooling, replace its
private URL file, then restart only the dependent MCP process/operator. Revoke
sessions/tokens on incidents. Rotate ingress/database certificates with verified
CA/name checks; never use noTLSVerify or sslmode=disable. Rotate existing Tunnel
credentials only through separately authorized owner procedures; do not put them
in images. Credential rotation does not replace authority revocation.

Persistent PostgreSQL volume and off-host encrypted backups are owner prerequisites.
backup.sh runs as the postgres OS account and emits only an encrypted dump.
Include new immutable enrollment/access audit tables in backups. Restore to isolated
wizpay_mcp_restore, verify integrity, then run restore-revoke.sql to invalidate
restored credentials before any cutover. Do not delete or rewrite revoked proof,
bindings or approvals. Test restoration and storage persistence on the selected
host; temporary sandbox volumes are not evidence of durability.

Browser E2E, real wallet signatures, public Tunnel routing and real Claude tests
are intentionally **NOT RUN** by the implementation task.
