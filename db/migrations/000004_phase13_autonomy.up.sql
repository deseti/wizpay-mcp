-- Phase 13 durable autonomy state. Payloads are typed references/digests only;
-- execution material remains in the immutable Phase 12 intent tables.
CREATE TABLE IF NOT EXISTS autonomy_grants (
    tenant_id text NOT NULL, grant_id text NOT NULL, version bigint NOT NULL CHECK (version > 0), principal_user_id text NOT NULL,
    wallet_binding_id text NOT NULL, intent_type text NOT NULL CHECK (intent_type IN ('PAYROLL','SWAP')), schedule_id text,
    expires_at timestamptz NOT NULL, paused boolean NOT NULL DEFAULT false, revoked boolean NOT NULL DEFAULT false,
    per_action_base_units numeric(78,0) CHECK (per_action_base_units IS NULL OR per_action_base_units > 0),
    aggregate_cap_base_units numeric(78,0) CHECK (aggregate_cap_base_units IS NULL OR aggregate_cap_base_units > 0),
    rolling_cap_base_units numeric(78,0) CHECK (rolling_cap_base_units IS NULL OR rolling_cap_base_units > 0),
    rolling_window_seconds bigint CHECK (rolling_window_seconds IS NULL OR rolling_window_seconds > 0),
    step_up_base_units numeric(78,0) CHECK (step_up_base_units IS NULL OR step_up_base_units > 0),
    allowed_recipients text[] NOT NULL DEFAULT '{}', allowed_tokens text[] NOT NULL DEFAULT '{}', allowed_chains text[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (tenant_id, grant_id, version)
);
CREATE TABLE IF NOT EXISTS autonomy_delegations (
    tenant_id text NOT NULL, delegation_id text NOT NULL, version bigint NOT NULL CHECK (version > 0), principal_user_id text NOT NULL,
    agent_id text NOT NULL, capabilities text[] NOT NULL CHECK (cardinality(capabilities) BETWEEN 1 AND 2) CHECK (capabilities <@ ARRAY['PAYROLL','SWAP']::text[]), expires_at timestamptz NOT NULL, revoked boolean NOT NULL DEFAULT false,
    non_transitive boolean NOT NULL DEFAULT true CHECK (non_transitive), PRIMARY KEY (tenant_id, delegation_id, version)
);
CREATE TABLE IF NOT EXISTS autonomy_schedules (
    tenant_id text NOT NULL, schedule_id text NOT NULL, version bigint NOT NULL CHECK (version > 0),
    user_id text NOT NULL, agent_id text NOT NULL, wallet_binding_id text NOT NULL, wallet_binding_version bigint NOT NULL CHECK (wallet_binding_version > 0),
    intent_type text NOT NULL CHECK (intent_type IN ('PAYROLL','SWAP')), grant_id text NOT NULL, grant_version bigint NOT NULL CHECK (grant_version > 0), delegation_id text NOT NULL, delegation_version bigint NOT NULL CHECK (delegation_version > 0), status text NOT NULL CHECK (status IN ('ACTIVE','PAUSED','REVOKED')),
    spec_digest text NOT NULL, schedule_digest text NOT NULL, timezone text NOT NULL, start_at timestamptz NOT NULL, end_at timestamptz,
    max_recipients integer NOT NULL CHECK (max_recipients BETWEEN 1 AND 500),
    recurrence text NOT NULL CHECK (recurrence IN ('ONCE','DAILY','WEEKLY','MONTHLY')), missed_run text NOT NULL CHECK (missed_run IN ('SKIP','RUN_LATEST')),
    concurrency text NOT NULL CHECK (concurrency = 'FORBID_OVERLAP'), created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, schedule_id, version), UNIQUE (tenant_id, schedule_id, version, schedule_digest),
    FOREIGN KEY (tenant_id, grant_id, grant_version) REFERENCES autonomy_grants(tenant_id, grant_id, version),
    FOREIGN KEY (tenant_id, delegation_id, delegation_version) REFERENCES autonomy_delegations(tenant_id, delegation_id, version)
);
CREATE TABLE IF NOT EXISTS autonomy_occurrences (
    tenant_id text NOT NULL, occurrence_id text NOT NULL, occurrence_key text NOT NULL, schedule_id text NOT NULL, schedule_version bigint NOT NULL,
    schedule_digest text NOT NULL, scheduled_at timestamptz NOT NULL, status text NOT NULL CHECK (status IN ('DUE','CLAIMED','BLOCKED','APPROVAL_REQUIRED','DISPATCHED','RECONCILIATION_ONLY','COMPLETED','SKIPPED')), grant_id text NOT NULL, grant_version bigint NOT NULL CHECK (grant_version > 0), intent_id text, approval_id text, execution_id text,
    lease_owner text, lease_until timestamptz, fence bigint NOT NULL DEFAULT 0 CHECK (fence >= 0), reason_code text NOT NULL DEFAULT '' CHECK (reason_code IN ('','ELIGIBLE','RUNTIME_DISABLED','SCHEDULE_PAUSED','GRANT_DENIED','DELEGATION_DENIED','EMERGENCY_STOP','STEP_UP_REQUIRED','OVERLAP_FORBIDDEN','CAPABILITY_UNAVAILABLE','ALREADY_SUBMITTED_RECONCILE')),
    created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL, PRIMARY KEY (tenant_id, occurrence_id), UNIQUE (tenant_id, occurrence_key),
    FOREIGN KEY (tenant_id, schedule_id, schedule_version) REFERENCES autonomy_schedules(tenant_id, schedule_id, version),
    FOREIGN KEY (tenant_id, grant_id, grant_version) REFERENCES autonomy_grants(tenant_id, grant_id, version)
);
CREATE INDEX IF NOT EXISTS autonomy_occurrences_due_idx ON autonomy_occurrences(tenant_id, status, scheduled_at, lease_until);
CREATE TABLE IF NOT EXISTS autonomy_spend_reservations (
    tenant_id text NOT NULL, grant_id text NOT NULL, grant_version bigint NOT NULL CHECK (grant_version > 0), occurrence_id text NOT NULL, amount_base_units numeric(78,0) NOT NULL CHECK (amount_base_units > 0),
    reserved_at timestamptz NOT NULL, state text NOT NULL CHECK (state IN ('RESERVED','COMMITTED','RELEASED')), PRIMARY KEY (tenant_id, grant_id, grant_version, occurrence_id),
    FOREIGN KEY (tenant_id, grant_id, grant_version) REFERENCES autonomy_grants(tenant_id, grant_id, version),
    FOREIGN KEY (tenant_id, occurrence_id) REFERENCES autonomy_occurrences(tenant_id, occurrence_id)
);
CREATE INDEX IF NOT EXISTS autonomy_spend_window_idx ON autonomy_spend_reservations(tenant_id, grant_id, grant_version, reserved_at) WHERE state IN ('RESERVED','COMMITTED');
CREATE TABLE IF NOT EXISTS autonomy_emergency_stop (
    tenant_id text NOT NULL PRIMARY KEY, active boolean NOT NULL, scope text NOT NULL CHECK (scope = 'TENANT'), actor_id text NOT NULL, reason_code text NOT NULL CHECK (length(reason_code) BETWEEN 1 AND 256), changed_at timestamptz NOT NULL
);
