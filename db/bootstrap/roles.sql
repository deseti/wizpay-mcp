\set ON_ERROR_STOP on

-- Run as a PostgreSQL administrator after the Phase 7 migration. These are
-- NOLOGIN group roles: deployment-specific login roles and credentials are
-- created outside the repository and granted membership explicitly.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'wizpay_mcp_migration_owner') THEN
        CREATE ROLE wizpay_mcp_migration_owner NOLOGIN NOINHERIT NOCREATEDB NOCREATEROLE NOSUPERUSER NOREPLICATION;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'wizpay_mcp_application') THEN
        CREATE ROLE wizpay_mcp_application NOLOGIN INHERIT NOCREATEDB NOCREATEROLE NOSUPERUSER NOREPLICATION;
    END IF;
END
$$;

DO $$
BEGIN
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO wizpay_mcp_application', current_database());
END
$$;

REVOKE CREATE ON SCHEMA public FROM PUBLIC;
ALTER SCHEMA public OWNER TO wizpay_mcp_migration_owner;
GRANT USAGE ON SCHEMA public TO wizpay_mcp_application;

DO $$
DECLARE
    object_name record;
BEGIN
    FOR object_name IN SELECT tablename FROM pg_tables WHERE schemaname = 'public' LOOP
        EXECUTE format('ALTER TABLE public.%I OWNER TO wizpay_mcp_migration_owner', object_name.tablename);
    END LOOP;
    FOR object_name IN SELECT sequencename FROM pg_sequences WHERE schemaname = 'public' LOOP
        EXECUTE format('ALTER SEQUENCE public.%I OWNER TO wizpay_mcp_migration_owner', object_name.sequencename);
    END LOOP;
    FOR object_name IN
        SELECT p.oid::regprocedure AS signature
        FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
        WHERE n.nspname = 'public'
    LOOP
        EXECUTE format('ALTER FUNCTION %s OWNER TO wizpay_mcp_migration_owner', object_name.signature);
    END LOOP;
END
$$;

REVOKE ALL ON ALL TABLES IN SCHEMA public FROM PUBLIC, wizpay_mcp_application;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC, wizpay_mcp_application;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC, wizpay_mcp_application;

GRANT SELECT ON ALL TABLES IN SCHEMA public TO wizpay_mcp_application;
GRANT INSERT ON tenants, identities, wallet_bindings, intents, approvals, policies,
    policy_evaluations, policy_evaluation_findings, execution_requests, executions,
    verification_evidence, audit_records, execution_runtime_work TO wizpay_mcp_application;
GRANT UPDATE ON identities, wallet_bindings, intents, approvals, policies, executions,
    execution_runtime_work TO wizpay_mcp_application;
REVOKE UPDATE, DELETE, TRUNCATE, TRIGGER, REFERENCES ON audit_records FROM wizpay_mcp_application;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO wizpay_mcp_application;

ALTER DEFAULT PRIVILEGES FOR ROLE wizpay_mcp_migration_owner IN SCHEMA public
    GRANT SELECT ON TABLES TO wizpay_mcp_application;
ALTER DEFAULT PRIVILEGES FOR ROLE wizpay_mcp_migration_owner IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO wizpay_mcp_application;
