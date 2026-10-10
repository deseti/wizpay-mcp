# WP3D delivery record — 2026-10-10

Stages are delivery groupings, not replacements for historical phases. Baseline
0307cd6; WP3D-2C remains completed. This record covers new uncommitted work only.

| Stage | Code state | Runtime acceptance |
|---|---|---|
| 1 — WP3D-2D / 2E | Browser harness, isolated trust setup and fixtures implemented | BLOCKED: owner cloud setup/browser run and genuine client evidence |
| 2 — WP3D-3 / 4 | Production guard, secret files, rate admission, CSP, probes, private RC manifest, role SQL and backup tooling implemented | BLOCKED: DB permissions, recovery, CSP/browser and runtime validation |
| 3 — WP3D-5 | Private candidate and operator procedure prepared | NO-GO: hosting, public ingress, authentication activation and acceptance missing |

## Actual engineering changes

Production now rejects JWT/legacy-registry selection, missing authentication,
autonomy and in-process migrations. Nonproduction legacy behavior remains.
DATABASE_MIGRATIONS_ON_START defaults true outside production and false in
production. Normal runtime checks the exact existing migration version set without
DDL; it rejects a missing or newer schema. cmd/dbmigrate is a separate operator
tool. Production refuses migration credential environment variables.

DATABASE_URL_FILE / DATABASE_MIGRATION_URL_FILE support protected filesystem secret
injection. Conflicting sources fail closed. Diagnostics omit file paths and values.
Production application receives only the runtime DB secret. Local/test credential
environment usage remains backward-compatible. cmd/dbmaintenance invokes the
existing conservative pending-session cleanup; retained evidence is not removed.

Production OAuth constructors add finite, process-wide token-bucket admission:
30/min token and authorization, 20/min shared SIWE, 120/min browser and MCP, with
one minute burst capacity. Exhaustion returns 429/no-store/Retry-After. State uses
fixed route classes, never attacker-selected keys, tokens or forwarded headers.
This is aggregate overload protection, NOT fair per-user or distributed limiting.
Multiple replicas require an approved shared limiter or trusted edge enforcement;
no Redis assumption or new dependency is introduced. Health remains available.
Streaming responses are not wrapped or given a global write timeout.

Onboarding has request-specific script nonces and dynamic rendering, same-origin
connect policy, frame denial, object denial, no-referrer and no-store. Inline styles
remain permitted; inline scripts require a nonce. The proxy neither authenticates
users nor forwards backend credentials. CSP hydration and injected-wallet behavior
must pass actual Chrome acceptance before release.

Private RC Compose uses reviewed immutable image references, no pulls, nonroot Go
and edge, dropped capabilities, read-only application filesystems, health checks,
resource limits, log rotation, private upstreams, a persistent PostgreSQL named
volume and secret files. Nginx stays loopback-only: this is NOT a public deployment.
The Go image excludes dbmigrate, test fixtures and the synthetic signer entirely.
Its Docker build context admits only server and healthcheck binaries.

## Stage 1 — existing cloud sandbox only, owner-operated

Reuse sbx_001m4jc444ma9c7f254jkvf0rca / shell/wizpay-mcp-wp3d2d at
/home/agent/workspace/wizpay-mcp. Do not repeat successful Chrome installation,
archive transfer, image loading or smoke test. No cloud command was run here.
The local tool catalog does not provide an authenticated Docker Cloud execution
capability; operations also require owner execution under this task's rules.

After explicit package-install approval, INSIDE that sandbox:

```sh
# Owner only: the already-simulated package is still not installed.
sudo apt-get install --no-install-recommends libnss3-tools
cd /home/agent/workspace/wizpay-mcp
WP3D_CLOUD_TRUST_APPROVED=yes bash tests/browser-e2e/cloud-trust.sh
```

This creates a fresh private CA and SAN leaf under ignored browser-tls, and an NSS
database under an isolated browser HOME. It modifies no global trust store. The
script refuses existing paths. No production certificate is used. Verify that the
installed Chrome actually honors this NSS trust, without clicking through errors.
Existing passed image identities remain evidence for the imported artifacts, not
evidence they contain these new source changes. Transfer only changed source with
an owner-verified checksum and update the dedicated test artifacts when approved.
Never claim stale image binaries validate a changed source tree.

Build the ephemeral signer offline and run the harness as described in
wp3d-browser-https-e2e.md. It prepares public SQL and waits for READY. Two ephemeral
EOAs now allow same-client, different-user/wallet substitution checks. Set
WP3D_BROWSER_EXPIRY=yes to include the real two-minute expiry case. No clock is
forged. The fixture database is exclusively wizpay_mcp_browser_e2e.

For cloud Compose, append the cloud TLS override after the browser override:

```sh
docker compose -p wizpay-mcp-wp3d-browser -f deploy/local/compose.yaml \
 -f tests/browser-e2e/compose.override.yaml -f tests/browser-e2e/cloud.override.yaml config --quiet
# Owner-approved runtime only, with cached current images and private test env:
docker compose -p wizpay-mcp-wp3d-browser -f deploy/local/compose.yaml \
 -f tests/browser-e2e/compose.override.yaml -f tests/browser-e2e/cloud.override.yaml up -d --pull never --no-build
```

Inspect effective ports and IDs before starting. Seed using the existing guarded
psql command; use all three files for subsequent cloud operations. Use the optional
relay only after separate port-443 approval. Browser launch, using the existing
installed Chrome executable (set CHROME to its verified path):

```sh
HOME="$PWD/deploy/local/runtime/browser-home" "$CHROME" --headless=new --enable-automation \
 --user-data-dir="$PWD/deploy/local/runtime/browser-home/profile" \
 --remote-debugging-address=127.0.0.1 --remote-debugging-port=9222 \
 --no-proxy-server \
 --host-resolver-rules='MAP connect.wizpay.xyz 127.0.0.1, MAP mcp.wizpay.xyz 127.0.0.1, MAP * ~NOTFOUND' about:blank
```

Run as the existing unprivileged agent, never root or --no-sandbox. Keep CDP private.
Validate HTTPS in that exact HOME/browser before READY. curl must use the private
CA, not the leaf and never -k. No public port publication in the cloud control
plane is approved. Clean up only this context, browser, relay and test project.
Offload encrypted evidence/backups before any sandbox expiration; no TTL loop.

## WP3D-2E — real client acceptance, prepared not executed

Verified official documentation on 2026-10-10:

- [OpenAI authentication](https://developers.openai.com/plugins/build/auth)
  describes predefined OAuth clients as well as CIMD/DCR and S256. Choose an
  actually supported predefined public client configuration and capture its exact
  callback from the host's UI. WizPay supports none, not private_key_jwt, DCR,
  CIMD or refresh tokens. Do not advertise those unsupported features.
- [Claude connector setup](https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp)
  offers published identity, automatic registration or own OAuth client. Select
  own client and verify public-client compatibility and callback before registering.
- [xAI remote MCP](https://docs.x.ai/developers/tools/remote-mcp) documents remote
  MCP and an authorization token parameter. This is evidence for API bearer use,
  not evidence for Grok consumer OAuth onboarding. Test these separately.

For each approved client: record host product/version, exact registered public
callback, authorization resource and S256 behavior, consent, token errors, exact
two-tool discovery, permitted tenant reads, all excluded names and revoked-token
denial. Store only redacted results; never tokens or screenshots containing codes.
The loopback cloud test is NOT reachable by remote AI hosts. External HTTPS ingress
requires owner approval and a reviewed controlled-test boundary. No client PASS
is claimed. Client support and unsupported refresh behavior remain release gates.

## Hosting verdict — NO-GO: HOSTING BLOCKER

[Docker cloud usage](https://docs.docker.com/ai/sandboxes/cloud/usage/) currently
documents a default one-hour TTL, bounded renewal, stop/delete behavior and assigned
public HTTPS URLs. Experimental volumes snapshot on exit rather than continuous
durable commits. These statements do not prove this existing sandbox's settings.
No verified custom-domain/SNI/Host-preserving ingress, continuous availability,
independently durable PostgreSQL or tested restore is established here.

Keep using existing promotional credit for integration where supported. Do not
use automatic TTL renewal as a production availability solution, claim a sandbox
named volume survives replacement, or silently provision a different host.
Owner must obtain a documented persistent Docker hosting option (same platform if
supported), canonical-domain routing and off-instance backups, or explicitly
approve another platform. Existing 2 CPU / 4 GiB capacity may fit this constrained
read-only stack, but measured load/DB latency/disk growth are missing. Current credit
balance, tariffs and custom-domain availability are UNVERIFIED; no price invented.

## Remaining release blockers

Production/staging SIWE stays disabled by existing policy. Therefore new users
cannot currently finish production OAuth: deploying this RC does NOT constitute
public onboarding. Separate reviewed activation and legitimate account/binding
administration are required. No synthetic signer may ever be exposed publicly.
Retention for immutable authentication evidence requires an owner-approved policy
and forward migration; current cleanup deliberately cannot delete it. Runtime-role
column grants preserve authority fields while enabling PostgreSQL row locks;
permissions need real PostgreSQL regression, startup migration concurrency remains
operator-serialized, and distributed abuse controls/load/backup monitoring need
runtime verification. Database TLS is required for any remote database connection;
the private same-host profile is not an approval for cleartext off-host databases.

## Dated progress checklist — 2026-10-10

- [x] Existing phases preserved; test harness preserved and extended.
- [x] Independent security/operations code and private RC profile implemented.
- [x] Focused Go/race/vet, TypeScript, 34 controller, 8 harness, 12 proxy and 5 RC static checks passed.
- [ ] Next.js build: blocked by sandbox port-binding permission (Turbopack CSS worker).
- [ ] PostgreSQL new schema/role integration: compiled; owner Docker execution pending.
- [ ] Actual cloud HTTPS browser acceptance, expiry and cross-user cases passed.
- [ ] Actual ChatGPT / Claude / Grok supported-mode evidence recorded.
- [ ] Least-privilege DB, encrypted off-host backup and restore proven.
- [ ] Continuous hosting and canonical public ingress approved and verified.
- [ ] Production authentication activation separately reviewed and approved.
- [ ] Public release authorized, deployed and monitored.

NO-GO until all relevant runtime/security gates pass. No Git or deployment action
is performed by this document. See wp3d-release-operations.md for operator commands.

## Fresh workspace validation

2026-10-10: focused Go race tests (config, security boundary, app, server wiring,
ephemeral signer) passed with -count=1 and offline module settings. Operator
commands compiled; go vet ./... passed. TypeScript noEmit passed. Native Node
checks passed: 34 existing controller cases, 8 browser-harness cases, 12 local
proxy configuration cases, 5 release-profile cases. Shell syntax and JS syntax
passed. Both browser/cloud and release Compose config --quiet passed using
non-secret placeholder values, without starting services. git diff --check passed.

PostgreSQL test binary compiled, including the new exact-schema and restricted-role
lock tests; neither ran here. Owner command:

```sh
WIZPAY_ARC_INTEGRATION=0 go test -count=1 -v ./internal/storage/postgres -run 'TestReleaseRuntime|TestProtocol'
WIZPAY_ARC_INTEGRATION=0 go test -race -count=1 ./internal/storage/postgres -run 'TestReleaseRuntime|TestProtocol'
```

Next.js npm run build failed because the sandbox denied Turbopack's CSS worker
port binding (Operation not permitted), not because a browser flow passed or
failed. Owner must run the existing build and then validate CSP hydration in real
Chrome. No cloud/browser/client, PostgreSQL-runtime, restore or deployment success
is claimed. Dependency manifests and migrations are unchanged.
