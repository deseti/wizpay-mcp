-- Operator-only AFTER migrations, connected as migration owner. No password here.
-- Provision wizpay_runtime LOGIN credentials separately through protected tooling.
BEGIN;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM wizpay_runtime;
GRANT USAGE ON SCHEMA public TO wizpay_runtime;
-- PostgreSQL row locks require UPDATE on at least one column. Never grant
-- authority/status mutation: the first three columns are trigger-immutable;
-- identity updated_at is metadata only, not ownership/status/version authority.
GRANT UPDATE(created_at) ON tenants TO wizpay_runtime;
GRANT UPDATE(binding_id) ON wallet_bindings TO wizpay_runtime;
GRANT UPDATE(client_id) ON oauth_clients TO wizpay_runtime;
GRANT UPDATE(updated_at) ON identities TO wizpay_runtime;
GRANT SELECT ON schema_migrations,tenants,identities,wallet_bindings,wallet_binding_versions,intents,approvals,oauth_clients,oauth_consent_wallets TO wizpay_runtime;
GRANT SELECT,INSERT,UPDATE ON oauth_transactions,oauth_sessions,oauth_consents,oauth_codes,oauth_access_tokens,browser_sessions,siwe_challenges TO wizpay_runtime;
GRANT DELETE ON browser_sessions TO wizpay_runtime;
GRANT SELECT,INSERT ON siwe_authentications TO wizpay_runtime;
GRANT INSERT ON oauth_audit TO wizpay_runtime;
COMMIT;
