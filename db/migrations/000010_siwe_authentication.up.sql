-- Existing tenants are NOT approved for browser onboarding by this migration.
ALTER TABLE tenants ADD COLUMN status text NOT NULL DEFAULT 'INACTIVE' CHECK(status IN ('INACTIVE','ACTIVE','REVOKED'));
CREATE FUNCTION enforce_tenant_lifecycle() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.tenant_id<>OLD.tenant_id OR NEW.created_at<>OLD.created_at OR
    (OLD.status='REVOKED' AND NEW.status<>'REVOKED') THEN
  RAISE EXCEPTION 'Tenant identity and terminal revocation are immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER tenants_lifecycle BEFORE UPDATE ON tenants FOR EACH ROW EXECUTE FUNCTION enforce_tenant_lifecycle();
ALTER TABLE browser_sessions ADD CONSTRAINT browser_session_transaction_identity UNIQUE(session_reference,transaction_id);
ALTER TABLE browser_sessions ADD CONSTRAINT browser_session_owner_identity UNIQUE(session_reference,tenant_id,user_id);
ALTER TABLE wallet_binding_versions ADD CONSTRAINT wallet_binding_version_owner_identity UNIQUE(tenant_id,binding_id,version,user_id);
CREATE TABLE siwe_challenges (
 challenge_id text PRIMARY KEY CHECK(challenge_id ~ '^[0-9a-f]{64}$'),
 nonce text NOT NULL UNIQUE CHECK(nonce ~ '^[0-9a-f]{64}$'),
 message text NOT NULL CHECK(octet_length(message) BETWEEN 1 AND 2048),
 address text NOT NULL CHECK(address ~ '^0x[0-9a-fA-F]{40}$'),
 tenant_id text NOT NULL REFERENCES tenants(tenant_id),
 session_reference text NOT NULL REFERENCES browser_sessions(session_reference),
 transaction_id text NOT NULL REFERENCES oauth_transactions(transaction_id),
 client_id text NOT NULL REFERENCES oauth_clients(client_id),
 domain text NOT NULL CHECK(domain='connect.wizpay.xyz'),
 uri text NOT NULL CHECK(uri='https://connect.wizpay.xyz'),
 chain_id text NOT NULL CHECK(chain_id='5042'),
 issued_at timestamptz NOT NULL, expires_at timestamptz NOT NULL, consumed_at timestamptz,
 FOREIGN KEY(session_reference,transaction_id) REFERENCES browser_sessions(session_reference,transaction_id),
 CHECK(expires_at>issued_at AND expires_at<=issued_at+interval '2 minutes'),
 CHECK(consumed_at IS NULL OR (consumed_at>=issued_at AND consumed_at<expires_at))
);
CREATE INDEX siwe_challenges_session_idx ON siwe_challenges(session_reference);
CREATE FUNCTION enforce_siwe_consumption() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (to_jsonb(NEW)-'consumed_at') IS DISTINCT FROM (to_jsonb(OLD)-'consumed_at') OR
    (OLD.consumed_at IS NOT NULL AND NEW.consumed_at IS DISTINCT FROM OLD.consumed_at) THEN
  RAISE EXCEPTION 'SIWE challenge authority is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER siwe_challenges_immutable BEFORE UPDATE ON siwe_challenges FOR EACH ROW EXECUTE FUNCTION enforce_siwe_consumption();
CREATE TABLE siwe_authentications (
 session_reference text PRIMARY KEY REFERENCES browser_sessions(session_reference),
 challenge_id text NOT NULL UNIQUE REFERENCES siwe_challenges(challenge_id),
 tenant_id text NOT NULL, user_id text NOT NULL,
 binding_id text NOT NULL, binding_version bigint NOT NULL,
 authenticated_at timestamptz NOT NULL,
 FOREIGN KEY(tenant_id,user_id) REFERENCES identities(tenant_id,user_id),
 FOREIGN KEY(session_reference,tenant_id,user_id) REFERENCES browser_sessions(session_reference,tenant_id,user_id),
 FOREIGN KEY(tenant_id,binding_id,binding_version,user_id) REFERENCES wallet_binding_versions(tenant_id,binding_id,version,user_id)
);
CREATE TRIGGER siwe_authentications_immutable BEFORE UPDATE OR DELETE ON siwe_authentications FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
-- No signatures, session cookies, keys or automatic tenant/user/wallet activation.
-- Forward-only; retained evidence references prevent cleanup of correlated sessions.
