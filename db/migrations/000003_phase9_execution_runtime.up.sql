CREATE TABLE execution_runtime_work (
    tenant_id text NOT NULL,
    execution_id text NOT NULL,
    next_run_at timestamptz NOT NULL,
    lease_owner text NOT NULL DEFAULT '',
    lease_expires_at timestamptz,
    fencing_token bigint NOT NULL DEFAULT 0 CHECK (fencing_token >= 0),
    submission_started boolean NOT NULL DEFAULT false,
    PRIMARY KEY (tenant_id, execution_id),
    FOREIGN KEY (tenant_id, execution_id) REFERENCES executions(tenant_id, execution_id) ON DELETE RESTRICT,
    CHECK ((lease_owner = '' AND lease_expires_at IS NULL) OR (lease_owner <> '' AND lease_expires_at IS NOT NULL))
);

INSERT INTO execution_runtime_work (tenant_id, execution_id, next_run_at)
SELECT tenant_id, execution_id, updated_at
FROM executions
WHERE status NOT IN ('COMPLETED', 'CANCELLED')
ON CONFLICT (tenant_id, execution_id) DO NOTHING;

CREATE FUNCTION create_execution_runtime_work() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
BEGIN
    INSERT INTO execution_runtime_work (tenant_id, execution_id, next_run_at)
    VALUES (NEW.tenant_id, NEW.execution_id, NEW.updated_at)
    ON CONFLICT (tenant_id, execution_id) DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE TRIGGER executions_create_runtime_work AFTER INSERT ON executions
FOR EACH ROW EXECUTE FUNCTION create_execution_runtime_work();

CREATE INDEX execution_runtime_work_available_idx
    ON execution_runtime_work (next_run_at, lease_expires_at, tenant_id, execution_id);