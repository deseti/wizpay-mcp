CREATE TABLE IF NOT EXISTS schema_migrations (
    version bigint PRIMARY KEY CHECK (version > 0),
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenants (
    tenant_id text PRIMARY KEY CHECK (tenant_id <> ''),
    created_at timestamptz NOT NULL
);

CREATE TABLE identities (
    tenant_id text NOT NULL,
    user_id text NOT NULL,
    provider text NOT NULL,
    status text NOT NULL CHECK (status IN ('UNKNOWN','ACTIVE','SUSPENDED','REVOKED')),
    lifecycle_version bigint NOT NULL DEFAULT 1 CHECK (lifecycle_version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, user_id),
    UNIQUE (tenant_id, user_id, provider),
    FOREIGN KEY (tenant_id) REFERENCES tenants(tenant_id) ON DELETE RESTRICT
);

CREATE TABLE wallet_bindings (
    tenant_id text NOT NULL,
    binding_id text NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    user_id text NOT NULL,
    provider text NOT NULL,
    provider_user_reference text NOT NULL,
    wallet_id text NOT NULL,
    address text NOT NULL,
    chain_id text NOT NULL,
    network text NOT NULL,
    status text NOT NULL CHECK (status IN ('PENDING','ACTIVE','REVOKED')),
    verification_reference text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    verified_at timestamptz,
    revoked_at timestamptz,
    PRIMARY KEY (tenant_id, binding_id),
    UNIQUE (tenant_id, chain_id, network, address),
    UNIQUE (tenant_id, binding_id, version, user_id, provider, provider_user_reference, wallet_id, address, chain_id, network),
    FOREIGN KEY (tenant_id, user_id, provider) REFERENCES identities(tenant_id, user_id, provider) ON DELETE RESTRICT
);

CREATE TABLE wallet_binding_versions (
    tenant_id text NOT NULL,
    binding_id text NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    user_id text NOT NULL,
    provider text NOT NULL,
    provider_user_reference text NOT NULL,
    wallet_id text NOT NULL,
    address text NOT NULL,
    chain_id text NOT NULL,
    network text NOT NULL,
    status text NOT NULL CHECK (status IN ('PENDING','ACTIVE','REVOKED')),
    verification_reference text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    verified_at timestamptz,
    revoked_at timestamptz,
    PRIMARY KEY (tenant_id, binding_id, version),
    UNIQUE (tenant_id, binding_id, version, user_id, wallet_id, address, chain_id),
    UNIQUE (tenant_id, binding_id, version, user_id, provider, provider_user_reference, wallet_id, address, chain_id, network),
    FOREIGN KEY (tenant_id, binding_id) REFERENCES wallet_bindings(tenant_id, binding_id) ON DELETE RESTRICT
);

CREATE FUNCTION preserve_wallet_binding_version() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
BEGIN
    INSERT INTO wallet_binding_versions (tenant_id,binding_id,version,user_id,provider,provider_user_reference,wallet_id,address,chain_id,network,status,verification_reference,created_at,verified_at,revoked_at)
    VALUES (NEW.tenant_id,NEW.binding_id,NEW.version,NEW.user_id,NEW.provider,NEW.provider_user_reference,NEW.wallet_id,NEW.address,NEW.chain_id,NEW.network,NEW.status,NEW.verification_reference,NEW.created_at,NEW.verified_at,NEW.revoked_at);
    RETURN NEW;
END;
$$;

CREATE TRIGGER wallet_bindings_preserve_version AFTER INSERT OR UPDATE ON wallet_bindings
FOR EACH ROW EXECUTE FUNCTION preserve_wallet_binding_version();

CREATE TABLE intents (
    tenant_id text NOT NULL,
    intent_id text NOT NULL,
    intent_version bigint NOT NULL CHECK (intent_version > 0),
    client_request_id text NOT NULL,
    nonce text NOT NULL,
    intent_type text NOT NULL CHECK (intent_type IN ('PAYROLL','SWAP','BRIDGE','ANS_REGISTRATION')),
    user_id text NOT NULL,
    identity_provider text NOT NULL,
    provider_user_reference text NOT NULL,
    wallet_binding_id text NOT NULL,
    wallet_binding_version bigint NOT NULL CHECK (wallet_binding_version > 0),
    wallet_id text NOT NULL,
    wallet_address text NOT NULL,
    chain_id text NOT NULL,
    network text NOT NULL,
    financial jsonb NOT NULL CHECK (jsonb_typeof(financial) = 'object'),
    route_type text NOT NULL CHECK (route_type IN ('DIRECT_WALLET','ALLOWLISTED_CONTRACT','APPROVED_PROVIDER')),
    route_reference text NOT NULL,
    route_version bigint NOT NULL CHECK (route_version > 0),
    constraint_deadline timestamptz NOT NULL,
    policy_reference text NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('DRAFT','CREATED','APPROVAL_REQUIRED','APPROVED','READY_FOR_EXECUTION','EXPIRED','CANCELLED')),
    intent_digest text,
    operation_key text,
    operation_version bigint,
    lifecycle_version bigint NOT NULL DEFAULT 1 CHECK (lifecycle_version > 0),
    PRIMARY KEY (tenant_id, intent_id),
    UNIQUE (tenant_id, client_request_id),
    UNIQUE (tenant_id, intent_id, intent_version, intent_digest),
    UNIQUE (tenant_id, intent_id, intent_version, intent_digest, user_id),
    UNIQUE (tenant_id, intent_digest),
    UNIQUE (tenant_id, operation_key, operation_version),
    CHECK ((status = 'DRAFT' AND intent_digest IS NULL AND operation_key IS NULL AND operation_version IS NULL) OR
           (status <> 'DRAFT' AND intent_digest IS NOT NULL AND operation_key IS NOT NULL AND operation_version > 0)),
    CHECK (expires_at > created_at AND constraint_deadline >= created_at AND constraint_deadline <= expires_at),
    FOREIGN KEY (tenant_id, wallet_binding_id, wallet_binding_version, user_id, identity_provider, provider_user_reference, wallet_id, wallet_address, chain_id, network)
      REFERENCES wallet_binding_versions(tenant_id, binding_id, version, user_id, provider, provider_user_reference, wallet_id, address, chain_id, network) ON DELETE RESTRICT
);

CREATE TABLE approvals (
    tenant_id text NOT NULL,
    approval_id text NOT NULL,
    approval_version bigint NOT NULL CHECK (approval_version > 0),
    approval_request_id text NOT NULL,
    intent_id text NOT NULL,
    intent_version bigint NOT NULL CHECK (intent_version > 0),
    intent_digest text NOT NULL,
    user_id text NOT NULL,
    wallet_binding_id text NOT NULL,
    wallet_binding_version bigint NOT NULL CHECK (wallet_binding_version > 0),
    wallet_id text NOT NULL,
    wallet_address text NOT NULL,
    chain_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('PENDING','APPROVED','REJECTED','EXPIRED','CONSUMED')),
    decision text NOT NULL CHECK (decision IN ('PENDING','APPROVED','REJECTED')),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    decided_at timestamptz,
    consumed_at timestamptz,
    operation_key text,
    operation_version bigint,
    lifecycle_version bigint NOT NULL DEFAULT 1 CHECK (lifecycle_version > 0),
    PRIMARY KEY (tenant_id, approval_id),
    UNIQUE (tenant_id, approval_request_id),
    UNIQUE (tenant_id, intent_id, intent_version, intent_digest),
    UNIQUE (tenant_id, approval_id, approval_version, intent_id, intent_version, intent_digest),
    UNIQUE (tenant_id, approval_id, approval_version, intent_id, intent_version, intent_digest, user_id),
    CHECK (expires_at > created_at),
    CHECK (
      (status = 'PENDING' AND decision = 'PENDING' AND decided_at IS NULL) OR
      (status = 'APPROVED' AND decision = 'APPROVED' AND decided_at IS NOT NULL) OR
      (status = 'REJECTED' AND decision = 'REJECTED' AND decided_at IS NOT NULL) OR
      (status = 'EXPIRED' AND ((decision = 'PENDING' AND decided_at IS NULL) OR (decision = 'APPROVED' AND decided_at IS NOT NULL))) OR
      (status = 'CONSUMED' AND decision = 'APPROVED' AND decided_at IS NOT NULL)
    ),
    CHECK (decided_at IS NULL OR (decided_at >= created_at AND decided_at < expires_at)),
    CHECK ((status = 'CONSUMED' AND consumed_at IS NOT NULL AND consumed_at >= decided_at AND consumed_at < expires_at AND operation_key IS NOT NULL AND operation_version > 0) OR
           (status <> 'CONSUMED' AND consumed_at IS NULL AND operation_key IS NULL AND operation_version IS NULL)),
    FOREIGN KEY (tenant_id, intent_id, intent_version, intent_digest)
      REFERENCES intents(tenant_id, intent_id, intent_version, intent_digest) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, wallet_binding_id) REFERENCES wallet_bindings(tenant_id, binding_id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, wallet_binding_id, wallet_binding_version, user_id, wallet_id, wallet_address, chain_id)
      REFERENCES wallet_binding_versions(tenant_id, binding_id, version, user_id, wallet_id, address, chain_id) ON DELETE RESTRICT
);

CREATE TABLE policies (
    tenant_id text NOT NULL,
    policy_id text NOT NULL,
    policy_version bigint NOT NULL CHECK (policy_version > 0),
    name text NOT NULL,
    user_id text NOT NULL,
    wallet_binding_id text,
    intent_types text[] NOT NULL DEFAULT '{}',
    rules jsonb NOT NULL CHECK (jsonb_typeof(rules) = 'array'),
    status text NOT NULL CHECK (status IN ('DRAFT','ACTIVE','DISABLED','EXPIRED')),
    created_at timestamptz NOT NULL,
    valid_from timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    lifecycle_version bigint NOT NULL DEFAULT 1 CHECK (lifecycle_version > 0),
    PRIMARY KEY (tenant_id, policy_id, policy_version),
    UNIQUE (tenant_id, policy_id, policy_version, user_id),
    CHECK (expires_at > valid_from AND valid_from >= created_at),
    FOREIGN KEY (tenant_id, user_id) REFERENCES identities(tenant_id, user_id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, wallet_binding_id) REFERENCES wallet_bindings(tenant_id, binding_id) ON DELETE RESTRICT
);

CREATE TABLE policy_evaluations (
    tenant_id text NOT NULL,
    evaluation_key text NOT NULL,
    policy_id text NOT NULL,
    policy_version bigint NOT NULL,
    user_id text NOT NULL,
    intent_id text NOT NULL,
    intent_version bigint NOT NULL,
    intent_digest text NOT NULL,
    stage text NOT NULL CHECK (stage IN ('BEFORE_APPROVAL','BEFORE_EXECUTION')),
    decision text NOT NULL CHECK (decision IN ('ALLOW','DENY','REQUIRE_REVIEW')),
    evaluated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, evaluation_key),
    UNIQUE (tenant_id, evaluation_key, policy_id, policy_version, user_id, intent_id, intent_version, intent_digest, stage, evaluated_at),
    UNIQUE (tenant_id, policy_id, policy_version, intent_id, intent_version, intent_digest, stage, evaluated_at),
    FOREIGN KEY (tenant_id, policy_id, policy_version, user_id) REFERENCES policies(tenant_id, policy_id, policy_version, user_id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, intent_id, intent_version, intent_digest, user_id) REFERENCES intents(tenant_id, intent_id, intent_version, intent_digest, user_id) ON DELETE RESTRICT
);

CREATE TABLE policy_evaluation_findings (
    tenant_id text NOT NULL,
    evaluation_key text NOT NULL,
    finding_index integer NOT NULL CHECK (finding_index >= 0),
    rule_id text NOT NULL DEFAULT '',
    rule_type text NOT NULL DEFAULT '',
    decision text NOT NULL CHECK (decision IN ('ALLOW','DENY','REQUIRE_REVIEW')),
    reason text NOT NULL,
    PRIMARY KEY (tenant_id, evaluation_key, finding_index),
    FOREIGN KEY (tenant_id, evaluation_key) REFERENCES policy_evaluations(tenant_id, evaluation_key) ON DELETE RESTRICT
);

CREATE TABLE execution_requests (
    tenant_id text NOT NULL,
    request_id text NOT NULL,
    request_key text NOT NULL,
    request_version bigint NOT NULL CHECK (request_version > 0),
    execution_id text NOT NULL,
    operation_key text NOT NULL,
    operation_version bigint NOT NULL CHECK (operation_version > 0),
    intent_id text NOT NULL,
    intent_version bigint NOT NULL,
    intent_digest text NOT NULL,
    approval_id text NOT NULL,
    approval_version bigint NOT NULL,
    user_id text NOT NULL,
    policy_id text NOT NULL,
    policy_version bigint NOT NULL,
    policy_evaluation_key text NOT NULL,
    policy_evaluation_stage text NOT NULL CHECK (policy_evaluation_stage = 'BEFORE_EXECUTION'),
    policy_evaluated_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, request_id),
    UNIQUE (tenant_id, request_key, request_version),
    UNIQUE (tenant_id, execution_id),
    UNIQUE (tenant_id, operation_key, operation_version),
    UNIQUE (tenant_id, request_id, execution_id),
    FOREIGN KEY (tenant_id, intent_id, intent_version, intent_digest) REFERENCES intents(tenant_id, intent_id, intent_version, intent_digest) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, approval_id, approval_version, intent_id, intent_version, intent_digest, user_id)
      REFERENCES approvals(tenant_id, approval_id, approval_version, intent_id, intent_version, intent_digest, user_id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, policy_evaluation_key, policy_id, policy_version, user_id, intent_id, intent_version, intent_digest, policy_evaluation_stage, policy_evaluated_at)
      REFERENCES policy_evaluations(tenant_id, evaluation_key, policy_id, policy_version, user_id, intent_id, intent_version, intent_digest, stage, evaluated_at) ON DELETE RESTRICT
);

CREATE TABLE executions (
    tenant_id text NOT NULL,
    execution_id text NOT NULL,
    request_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('CREATED','AUTHORIZED','QUEUED','EXECUTING','SUBMITTED','CONFIRMING','CONFIRMED','VERIFIED','COMPLETED','FAILED','RECOVERY_REQUIRED','CANCELLED')),
    revision bigint NOT NULL CHECK (revision > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    failure_code text NOT NULL DEFAULT '',
    failure_eligibility text NOT NULL DEFAULT '',
    failure_recovery_target text NOT NULL DEFAULT '',
    failed_from_status text NOT NULL DEFAULT '',
    recovery_reason_code text NOT NULL DEFAULT '',
    recovery_from_status text NOT NULL DEFAULT '',
    recovery_target_status text NOT NULL DEFAULT '',
    PRIMARY KEY (tenant_id, execution_id),
    UNIQUE (tenant_id, request_id),
    UNIQUE (tenant_id, execution_id, revision),
    FOREIGN KEY (tenant_id, request_id, execution_id) REFERENCES execution_requests(tenant_id, request_id, execution_id) ON DELETE RESTRICT
);

CREATE TABLE execution_revisions (
    tenant_id text NOT NULL,
    execution_id text NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    status text NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, execution_id, revision),
    UNIQUE (tenant_id, execution_id, revision, status),
    FOREIGN KEY (tenant_id, execution_id) REFERENCES executions(tenant_id, execution_id) ON DELETE RESTRICT
);

CREATE FUNCTION preserve_execution_revision() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
BEGIN
    INSERT INTO execution_revisions (tenant_id,execution_id,revision,status,updated_at)
    VALUES (NEW.tenant_id,NEW.execution_id,NEW.revision,NEW.status,NEW.updated_at);
    RETURN NEW;
END;
$$;

CREATE TRIGGER executions_preserve_revision AFTER INSERT OR UPDATE ON executions
FOR EACH ROW EXECUTE FUNCTION preserve_execution_revision();

CREATE TABLE verification_evidence (
    tenant_id text NOT NULL,
    evidence_id bigint GENERATED ALWAYS AS IDENTITY,
    execution_id text NOT NULL,
    execution_version bigint NOT NULL CHECK (execution_version > 0),
    status text NOT NULL CHECK (status IN ('SUBMITTED','CONFIRMING','CONFIRMED','VERIFIED','COMPLETED','FAILED','RECOVERY_REQUIRED')),
    adapter_reference text NOT NULL DEFAULT '',
    error_code text NOT NULL DEFAULT '',
    observed_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, evidence_id),
    UNIQUE (tenant_id, execution_id, execution_version, status, observed_at),
    FOREIGN KEY (tenant_id, execution_id, execution_version, status) REFERENCES execution_revisions(tenant_id, execution_id, revision, status) ON DELETE RESTRICT
);

CREATE TABLE audit_records (
    tenant_id text NOT NULL,
    event_id text NOT NULL,
    event_type text NOT NULL,
    occurred_at timestamptz NOT NULL,
    actor_type text NOT NULL,
    actor_id text NOT NULL,
    request_id text NOT NULL,
    trace_id text NOT NULL DEFAULT '',
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    previous_state text NOT NULL DEFAULT '',
    new_state text NOT NULL DEFAULT '',
    safe_reason_code text NOT NULL DEFAULT '',
    source_component text NOT NULL,
    intent_id text NOT NULL DEFAULT '',
    intent_version bigint NOT NULL DEFAULT 0,
    intent_digest text NOT NULL DEFAULT '',
    approval_id text NOT NULL DEFAULT '',
    policy_id text NOT NULL DEFAULT '',
    policy_version bigint NOT NULL DEFAULT 0,
    policy_decision text NOT NULL DEFAULT '',
    policy_evaluation_key text NOT NULL DEFAULT '',
    execution_id text NOT NULL DEFAULT '',
    execution_revision bigint NOT NULL DEFAULT 0,
    execution_request_id text NOT NULL DEFAULT '',
    execution_request_key text NOT NULL DEFAULT '',
    execution_status text NOT NULL DEFAULT '',
    recovery_reason_code text NOT NULL DEFAULT '',
    wallet_binding_id text NOT NULL DEFAULT '',
    wallet_binding_version bigint NOT NULL DEFAULT 0,
    user_id text NOT NULL DEFAULT '',
    operation_key text NOT NULL DEFAULT '',
    operation_version bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, event_id),
    FOREIGN KEY (tenant_id) REFERENCES tenants(tenant_id) ON DELETE RESTRICT
);

CREATE INDEX identities_status_idx ON identities (tenant_id, status);
CREATE INDEX wallet_bindings_owner_status_idx ON wallet_bindings (tenant_id, user_id, status);
CREATE INDEX intents_owner_status_idx ON intents (tenant_id, user_id, status, expires_at);
CREATE INDEX approvals_intent_status_idx ON approvals (tenant_id, intent_id, status, expires_at);
CREATE INDEX policies_applicable_idx ON policies (tenant_id, user_id, status, valid_from, expires_at);
CREATE INDEX policy_evaluations_intent_idx ON policy_evaluations (tenant_id, intent_id, evaluated_at DESC);
CREATE INDEX executions_status_idx ON executions (tenant_id, status, updated_at);
CREATE INDEX verification_evidence_execution_idx ON verification_evidence (tenant_id, execution_id, observed_at);
CREATE INDEX audit_records_resource_idx ON audit_records (tenant_id, resource_type, resource_id, occurred_at, event_id);

CREATE FUNCTION reject_intent_material_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(OLD.tenant_id,OLD.intent_id,OLD.intent_version,OLD.client_request_id,OLD.nonce,OLD.intent_type,OLD.user_id,OLD.identity_provider,OLD.provider_user_reference,OLD.wallet_binding_id,OLD.wallet_binding_version,OLD.wallet_id,OLD.wallet_address,OLD.chain_id,OLD.network,OLD.financial,OLD.route_type,OLD.route_reference,OLD.route_version,OLD.constraint_deadline,OLD.policy_reference,OLD.created_at,OLD.expires_at)
       IS DISTINCT FROM
       ROW(NEW.tenant_id,NEW.intent_id,NEW.intent_version,NEW.client_request_id,NEW.nonce,NEW.intent_type,NEW.user_id,NEW.identity_provider,NEW.provider_user_reference,NEW.wallet_binding_id,NEW.wallet_binding_version,NEW.wallet_id,NEW.wallet_address,NEW.chain_id,NEW.network,NEW.financial,NEW.route_type,NEW.route_reference,NEW.route_version,NEW.constraint_deadline,NEW.policy_reference,NEW.created_at,NEW.expires_at) THEN
        RAISE EXCEPTION 'intent material is immutable' USING ERRCODE = '55000';
    END IF;
    IF OLD.status = 'DRAFT' THEN
        IF NEW.status <> 'CREATED' OR NEW.intent_digest IS NULL OR NEW.operation_key IS NULL OR NEW.operation_version IS NULL OR NEW.operation_version <= 0 THEN
            RAISE EXCEPTION 'intent freeze must transition DRAFT to CREATED with complete operation identity' USING ERRCODE = '55000';
        END IF;
    ELSIF ROW(OLD.intent_digest,OLD.operation_key,OLD.operation_version) IS DISTINCT FROM ROW(NEW.intent_digest,NEW.operation_key,NEW.operation_version) THEN
        RAISE EXCEPTION 'frozen intent identity is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.lifecycle_version <> OLD.lifecycle_version + 1 THEN
        RAISE EXCEPTION 'intent lifecycle revision must advance exactly once' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER intents_immutable_material BEFORE UPDATE ON intents FOR EACH ROW EXECUTE FUNCTION reject_intent_material_mutation();

CREATE FUNCTION reject_wallet_identity_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(OLD.tenant_id,OLD.binding_id,OLD.user_id,OLD.provider,OLD.provider_user_reference,OLD.wallet_id,OLD.address,OLD.chain_id,OLD.network,OLD.created_at)
       IS DISTINCT FROM ROW(NEW.tenant_id,NEW.binding_id,NEW.user_id,NEW.provider,NEW.provider_user_reference,NEW.wallet_id,NEW.address,NEW.chain_id,NEW.network,NEW.created_at) THEN
        RAISE EXCEPTION 'wallet binding identity is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.version <> OLD.version + 1 THEN
        RAISE EXCEPTION 'wallet binding version must advance exactly once' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER wallet_bindings_immutable_identity BEFORE UPDATE ON wallet_bindings FOR EACH ROW EXECUTE FUNCTION reject_wallet_identity_mutation();

CREATE FUNCTION reject_approval_binding_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(OLD.tenant_id,OLD.approval_id,OLD.approval_version,OLD.approval_request_id,OLD.intent_id,OLD.intent_version,OLD.intent_digest,OLD.user_id,OLD.wallet_binding_id,OLD.wallet_binding_version,OLD.wallet_id,OLD.wallet_address,OLD.chain_id,OLD.created_at,OLD.expires_at)
       IS DISTINCT FROM
       ROW(NEW.tenant_id,NEW.approval_id,NEW.approval_version,NEW.approval_request_id,NEW.intent_id,NEW.intent_version,NEW.intent_digest,NEW.user_id,NEW.wallet_binding_id,NEW.wallet_binding_version,NEW.wallet_id,NEW.wallet_address,NEW.chain_id,NEW.created_at,NEW.expires_at) THEN
        RAISE EXCEPTION 'approval binding is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.lifecycle_version <> OLD.lifecycle_version + 1 THEN
        RAISE EXCEPTION 'approval lifecycle revision must advance exactly once' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER approvals_immutable_binding BEFORE UPDATE ON approvals FOR EACH ROW EXECUTE FUNCTION reject_approval_binding_mutation();

CREATE FUNCTION reject_policy_material_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(OLD.tenant_id,OLD.policy_id,OLD.policy_version,OLD.name,OLD.user_id,OLD.wallet_binding_id,OLD.intent_types,OLD.rules,OLD.created_at,OLD.valid_from,OLD.expires_at)
       IS DISTINCT FROM
       ROW(NEW.tenant_id,NEW.policy_id,NEW.policy_version,NEW.name,NEW.user_id,NEW.wallet_binding_id,NEW.intent_types,NEW.rules,NEW.created_at,NEW.valid_from,NEW.expires_at) THEN
        RAISE EXCEPTION 'policy version material is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.lifecycle_version <> OLD.lifecycle_version + 1 THEN
        RAISE EXCEPTION 'policy lifecycle revision must advance exactly once' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER policies_immutable_material BEFORE UPDATE ON policies FOR EACH ROW EXECUTE FUNCTION reject_policy_material_mutation();

CREATE FUNCTION reject_immutable_record_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'immutable record cannot be updated or deleted' USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER execution_requests_immutable BEFORE UPDATE OR DELETE ON execution_requests FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
CREATE TRIGGER policy_evaluations_immutable BEFORE UPDATE OR DELETE ON policy_evaluations FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
CREATE TRIGGER policy_findings_immutable BEFORE UPDATE OR DELETE ON policy_evaluation_findings FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
CREATE TRIGGER verification_evidence_immutable BEFORE UPDATE OR DELETE ON verification_evidence FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
CREATE TRIGGER wallet_binding_versions_immutable BEFORE UPDATE OR DELETE ON wallet_binding_versions FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
CREATE TRIGGER execution_revisions_immutable BEFORE UPDATE OR DELETE ON execution_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();

CREATE FUNCTION reject_execution_identity_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(OLD.tenant_id,OLD.execution_id,OLD.request_id,OLD.created_at) IS DISTINCT FROM ROW(NEW.tenant_id,NEW.execution_id,NEW.request_id,NEW.created_at) THEN
        RAISE EXCEPTION 'execution identity is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.revision <> OLD.revision + 1 THEN
        RAISE EXCEPTION 'execution revision must advance exactly once' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER executions_immutable_identity BEFORE UPDATE ON executions FOR EACH ROW EXECUTE FUNCTION reject_execution_identity_mutation();

CREATE FUNCTION reject_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit records are append-only' USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER audit_records_no_update BEFORE UPDATE ON audit_records
FOR EACH ROW EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER audit_records_no_delete BEFORE DELETE ON audit_records
FOR EACH ROW EXECUTE FUNCTION reject_audit_mutation();
