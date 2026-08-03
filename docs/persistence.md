# Phase 7 PostgreSQL persistence

## Ownership and boundaries

WizPay MCP owns this schema independently. PostgreSQL is the only source of truth for tenant ownership, identities, wallet-binding versions, immutable intents, approvals, versioned policies and evaluations, execution requests/lifecycle, verification evidence, and audit records. No WizPay Core or Nano WizPay database, migration, runtime, or queue is imported.

`internal/storage` defines tenant-scoped domain repository ports. `internal/storage/postgres` implements those ports with pgx/v5 and sqlc-generated code. Generated types remain in `internal/storage/postgres/dbsqlc` and never cross into domain packages. MCP handlers do not import storage.

Every tenant-owned query includes `tenant_id` in SQL. A trusted `storage.Scope` supplies tenant, actor, request, and trace references after application identity resolution. MCP input cannot supply this scope. Missing and cross-tenant records use the same safe inaccessible/not-found response.

## Schema and invariants

The forward migration in `db/migrations` creates:

- tenants and tenant-composite ownership keys;
- identities and current wallet bindings plus append-only wallet-binding version snapshots;
- immutable typed intents with exact JSON financial unions, digest, client request, and operation uniqueness;
- approvals bound by composite foreign keys to the exact intent ID/version/digest and wallet-binding tenant, user, ID, immutable version, wallet ID/address, and chain ID;
- versioned policies, deterministic evaluations, and ordered findings, with execution requests bound by one composite foreign key to the exact policy/evaluation/intent/stage/timestamp tuple;
- immutable execution requests and one execution per operation key;
- provider-neutral verification observations bound to an immutable execution-revision/status snapshot;
- append-only audit records protected by database triggers.

Financial values stay as canonical decimal/base-unit strings inside the typed financial JSON. No floating-point database type is used. Foreign keys use `ON DELETE RESTRICT`; no cascade can erase financial or audit evidence. Lifecycle changes use status/revision compare-and-swap clauses, while unique constraints are the correctness boundary for concurrent idempotency.

## Migrations and sqlc

Migrations are forward-only `.up.sql` files. The embedded migrator uses `DATABASE_MIGRATION_URL`, applies each version in a transaction, and records it in `schema_migrations`; repeated invocation is a no-op. Runtime repositories use the separately configured `DATABASE_URL`. For simple local development both may temporarily identify the same administrator login; production must separate them.

```bash
make sqlc-generate
make sqlc-check
```

`make sqlc-check` regenerates code with pinned sqlc `v1.30.0` and fails if generated files differ. Never edit `internal/storage/postgres/dbsqlc` manually.

Because Phase 7 is uncommitted and undeployed, the corrected schema remains the single deterministic `000001` fresh-install migration. There is no down migration. After deployment, never rewrite an applied migration: stop rollout, retain the failed database and logs, restore the pre-migration backup when service recovery requires it, and ship a new higher-numbered forward-fix migration.

## Database roles and bootstrap

Run the migration with a deployment-managed login that can assume the `wizpay_mcp_migration_owner` group role. Then run the idempotent administrator script and grant the application group to a deployment-specific login whose password comes from a secret manager:

```bash
psql "$DATABASE_MIGRATION_URL" --file db/bootstrap/roles.sql
# Administrator action; APP_LOGIN is a deployment-created identifier, not MCP input.
psql "$DATABASE_MIGRATION_URL" --command "GRANT wizpay_mcp_application TO \"$APP_LOGIN\""
```

The migration-owner role owns the public schema, tables, sequences, functions, and triggers. The application role has `CONNECT`, schema `USAGE`, read/insert access needed by repositories, and update access only for lifecycle tables. It has no audit `UPDATE`, `DELETE`, `TRUNCATE`, `TRIGGER`, ownership, schema-create, or trigger-function replacement privilege. It also cannot insert wallet/execution revision history directly; security-definer preservation triggers owned by the migration role append those snapshots. Audit mutation triggers remain defense in depth. New migrations must update the narrow grant list deliberately before the application uses a new table.

## Local development

Copy `.env.example` to the ignored `.env`, replace the local-only placeholder password, then run:

```bash
docker compose up -d postgres
go run ./cmd/server
```

The Compose database binds only to loopback, uses a WizPay MCP-scoped volume, and has a PostgreSQL health check. Server startup validates database configuration, creates a bounded pgx pool, verifies connectivity, applies migrations, and closes the pool during shutdown. `/readiness` checks PostgreSQL with a bounded context.

Integration tests use a real disposable PostgreSQL 16 Testcontainers instance:

```bash
make test-integration
```

No SQLite or in-memory persistence substitute is supported.

## Transactions, idempotency, and concurrency

Serializable transactions couple:

- immutable intent creation with its initial audit event;
- approval creation with its audit event;
- approval consumption with create-or-load of the single execution request;
- execution lifecycle compare-and-swap with audit insertion;
- verification evidence with the `VERIFIED` lifecycle transition and audit event.

A failure rolls back every record in the transaction. Retry loads the same operation/execution identity. Process-local mutexes are not used as a correctness boundary.

Intent, approval, policy, wallet, and execution lifecycle writes use an explicit expected revision and must advance exactly once. Database triggers repeat that rule for direct SQL. Status alone is not a concurrency token. Immutable intent, approval-binding, policy-version, execution-request, and execution-identity material cannot be changed by lifecycle updates.

An execution retry is equivalent only when every immutable request field matches: request/execution identity, operation key/version, intent ID/version/digest, approval ID/version, policy ID/version, exact evaluation key/stage/time, and request creation time. Mutable execution status, revision, timestamps, failure, and recovery fields are not retry identity. The unique operation key is the race boundary; a same-key mismatch is a typed conflict.

`FindApplicablePolicies` requires an explicit evaluation time and filters in tenant-scoped SQL. Only the highest stored version of each policy ID is considered; it must be `ACTIVE`, have `valid_from <= evaluated_at`, have `expires_at > evaluated_at`, and match the current user, optional wallet binding, and intent type. Thus `valid_from` equality is included and `expires_at` equality is excluded.

## Audit, redaction, and retention

Audit APIs expose append/read only; no update or delete query exists. Database triggers reject update and delete even through direct SQL. Audit metadata is a typed allowlist and rejects known credential/private-material markers. Raw errors, provider payloads, authorization tokens, private keys, seed phrases, signing shares, and credentials are forbidden.

Retention durations remain an owner/legal decision. Until approved:

- identity and wallet records are retained through revocation plus the required audit/dispute period;
- immutable intents, approvals, policy versions, execution requests, evidence, and audit records are retained together for idempotency, dispute, and regulatory needs;
- revoked or superseded records remain immutable evidence rather than being overwritten;
- temporary logs and metadata are minimized, redacted at source, and assigned a short operational retention window;
- no cleanup worker runs in Phase 7.

## Backup and restore

Use credentials from a secret manager or local untracked environment. Never place them in command history or documentation.

```bash
pg_dump --dbname "$DATABASE_URL" --format=custom --no-owner --no-privileges --file wizpay-mcp.dump
createdb wizpay_mcp_restore
pg_restore --dbname "$RESTORE_DATABASE_URL" --no-owner --no-privileges wizpay-mcp.dump
```

After restore, run the migrator with the migration-owner URL, rerun `db/bootstrap/roles.sql`, compare table counts and sampled tenant-scoped domain records, verify lifecycle and execution-revision history, operation keys, exact composite bindings, and test that audit mutation and duplicate execution constraints still fail through the restricted application role. The integration suite automates schema/data restoration with safe fixtures and a separate empty restore database; roles are cluster objects and therefore must be bootstrapped rather than expected in `pg_dump --no-owner --no-privileges` output.

## Phase 7 non-goals

Phase 7 adds no authentication middleware, OAuth/OIDC, Circle/Arc/provider integration, blockchain client, signing, submission, receipt polling, worker, River job, queue, Redis authority, execution adapter, approval UI, treasury routing, or autonomous spending.
