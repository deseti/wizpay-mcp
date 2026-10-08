-- WP2 adds no client/user/session credential, no wallet activation and no financial authority.
CREATE TABLE oauth_clients (
 client_id text PRIMARY KEY, name text NOT NULL, client_type text NOT NULL CHECK(client_type='public'),
 auth_method text NOT NULL CHECK(auth_method='none'), status text NOT NULL CHECK(status IN ('ACTIVE','REVOKED')),
 redirect_uris text[] NOT NULL CHECK(cardinality(redirect_uris) BETWEEN 1 AND 8),
 scopes text[] NOT NULL CHECK(scopes=ARRAY['mcp:read']::text[]),
 resources text[] NOT NULL CHECK(resources=ARRAY['https://mcp.wizpay.xyz/mcp']::text[]),
 registered_at timestamp with time zone NOT NULL DEFAULT now()
);
CREATE TABLE oauth_transactions (
 transaction_id text PRIMARY KEY, client_id text NOT NULL REFERENCES oauth_clients(client_id),
 redirect_uri text NOT NULL, resource text NOT NULL CHECK(resource='https://mcp.wizpay.xyz/mcp'),
 scope text NOT NULL CHECK(scope='mcp:read'), challenge text NOT NULL CHECK(challenge ~ '^[A-Za-z0-9_-]{43}$'),
 state text NOT NULL CHECK(length(state)<=512), created_at timestamp with time zone NOT NULL, expires_at timestamp with time zone NOT NULL,
 completed_at timestamp with time zone,
 UNIQUE(transaction_id,client_id,redirect_uri,resource,scope,challenge), CHECK(expires_at>created_at AND expires_at<=created_at+interval '5 minutes')
);
-- Session IDs are non-secret references supplied by the future trusted browser port.
-- This table records OAuth eligibility/revocation, not browser cookie credentials.
CREATE TABLE oauth_sessions (
 tenant_id text NOT NULL, user_id text NOT NULL, session_id text NOT NULL,
 expires_at timestamp with time zone NOT NULL, revoked_at timestamp with time zone,
 PRIMARY KEY(tenant_id,user_id,session_id), FOREIGN KEY(tenant_id,user_id) REFERENCES identities(tenant_id,user_id)
);
CREATE TABLE oauth_consents (
 tenant_id text NOT NULL, user_id text NOT NULL, consent_id text NOT NULL,
 client_id text NOT NULL REFERENCES oauth_clients(client_id), resource text NOT NULL CHECK(resource='https://mcp.wizpay.xyz/mcp'),
 scope text NOT NULL CHECK(scope='mcp:read'), session_id text NOT NULL,
 identity_issuer text NOT NULL, subject text NOT NULL,
 created_at timestamp with time zone NOT NULL, expires_at timestamp with time zone NOT NULL, revoked_at timestamp with time zone,
 PRIMARY KEY(tenant_id,user_id,consent_id), UNIQUE(tenant_id,user_id,consent_id,client_id,resource,scope,session_id),
 UNIQUE(tenant_id,user_id,consent_id,client_id,resource,scope,session_id,identity_issuer,subject),
 FOREIGN KEY(tenant_id,user_id,session_id) REFERENCES oauth_sessions(tenant_id,user_id,session_id), CHECK(expires_at>created_at)
);
-- Reserved relationship only: WP2 creates no wallet selections. Any populated
-- restriction fails closed at issuance and verification until a reviewed mapper exists.
CREATE TABLE oauth_consent_wallets (
 tenant_id text NOT NULL, user_id text NOT NULL, consent_id text NOT NULL,
 wallet_binding_id text NOT NULL, wallet_binding_version bigint NOT NULL,
 wallet_id text NOT NULL, wallet_address text NOT NULL, chain_id text NOT NULL,
 PRIMARY KEY(tenant_id,user_id,consent_id,wallet_binding_id),
 FOREIGN KEY(tenant_id,user_id,consent_id) REFERENCES oauth_consents(tenant_id,user_id,consent_id),
 FOREIGN KEY(tenant_id,wallet_binding_id,wallet_binding_version,user_id,wallet_id,wallet_address,chain_id)
 REFERENCES wallet_binding_versions(tenant_id,binding_id,version,user_id,wallet_id,address,chain_id)
);
CREATE TRIGGER oauth_consent_wallets_immutable BEFORE UPDATE OR DELETE ON oauth_consent_wallets FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
CREATE TABLE oauth_codes (
 code_digest text PRIMARY KEY CHECK(code_digest ~ '^[A-Za-z0-9_-]{43}$'), transaction_id text NOT NULL UNIQUE REFERENCES oauth_transactions(transaction_id),
 tenant_id text NOT NULL, user_id text NOT NULL, consent_id text NOT NULL, session_id text NOT NULL,
 client_id text NOT NULL, redirect_uri text NOT NULL, resource text NOT NULL, scope text NOT NULL,
 challenge text NOT NULL, issued_at timestamp with time zone NOT NULL, expires_at timestamp with time zone NOT NULL, consumed_at timestamp with time zone,
 FOREIGN KEY(tenant_id,user_id,consent_id,client_id,resource,scope,session_id) REFERENCES oauth_consents(tenant_id,user_id,consent_id,client_id,resource,scope,session_id),
 FOREIGN KEY(transaction_id,client_id,redirect_uri,resource,scope,challenge) REFERENCES oauth_transactions(transaction_id,client_id,redirect_uri,resource,scope,challenge),
 CHECK(expires_at>issued_at AND expires_at<=issued_at+interval '2 minutes')
);
CREATE TABLE oauth_access_tokens (
 token_digest text PRIMARY KEY CHECK(token_digest ~ '^[A-Za-z0-9_-]{43}$'), issuer text NOT NULL CHECK(issuer='https://connect.wizpay.xyz'),
 resource text NOT NULL, tenant_id text NOT NULL, user_id text NOT NULL, client_id text NOT NULL,
 consent_id text NOT NULL, session_id text NOT NULL, scope text NOT NULL,
 identity_issuer text NOT NULL, subject text NOT NULL,
 issued_at timestamp with time zone NOT NULL, expires_at timestamp with time zone NOT NULL, revoked_at timestamp with time zone,
 FOREIGN KEY(tenant_id,user_id,consent_id,client_id,resource,scope,session_id) REFERENCES oauth_consents(tenant_id,user_id,consent_id,client_id,resource,scope,session_id),
 FOREIGN KEY(tenant_id,user_id,consent_id,client_id,resource,scope,session_id,identity_issuer,subject) REFERENCES oauth_consents(tenant_id,user_id,consent_id,client_id,resource,scope,session_id,identity_issuer,subject),
 CHECK(expires_at>issued_at AND expires_at<=issued_at+interval '10 minutes')
);
CREATE INDEX oauth_tokens_consent_idx ON oauth_access_tokens(tenant_id,user_id,consent_id);
CREATE INDEX oauth_transactions_expiry_idx ON oauth_transactions(expires_at);
-- Browser authorization and revocation audits contain record references only.
CREATE TABLE oauth_audit (
 event_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, event_type text NOT NULL CHECK(event_type IN ('AUTHORIZED','TOKEN_ISSUED','CONSENT_REVOKED','SESSION_REVOKED','TOKEN_REVOKED')),
 tenant_id text NOT NULL, user_id text NOT NULL, client_id text NOT NULL REFERENCES oauth_clients(client_id),
 consent_id text NOT NULL, occurred_at timestamp with time zone NOT NULL,
 FOREIGN KEY(tenant_id,user_id,consent_id) REFERENCES oauth_consents(tenant_id,user_id,consent_id)
);
CREATE TRIGGER oauth_audit_immutable BEFORE UPDATE OR DELETE ON oauth_audit FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
-- Forward only; no existing financial tables or data are rewritten.
-- Authority snapshots never change after issuance; terminal markers cannot clear.
CREATE FUNCTION reject_oauth_authority_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE marker text;
BEGIN
 marker := CASE TG_TABLE_NAME WHEN 'oauth_clients' THEN 'status' WHEN 'oauth_transactions' THEN 'completed_at' WHEN 'oauth_codes' THEN 'consumed_at' ELSE 'revoked_at' END;
 IF (to_jsonb(NEW)-marker) IS DISTINCT FROM (to_jsonb(OLD)-marker) THEN
  RAISE EXCEPTION 'OAuth authority snapshot is immutable' USING ERRCODE='55000';
 END IF;
 IF TG_TABLE_NAME='oauth_clients' THEN
  IF OLD.status='REVOKED' AND NEW.status<>'REVOKED' THEN RAISE EXCEPTION 'OAuth client revocation is terminal' USING ERRCODE='55000'; END IF;
 ELSIF to_jsonb(OLD)->>marker IS NOT NULL AND (to_jsonb(OLD)->marker) IS DISTINCT FROM (to_jsonb(NEW)->marker) THEN
  RAISE EXCEPTION 'OAuth lifecycle marker is terminal' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER oauth_clients_immutable BEFORE UPDATE ON oauth_clients FOR EACH ROW EXECUTE FUNCTION reject_oauth_authority_mutation();
CREATE TRIGGER oauth_transactions_immutable BEFORE UPDATE ON oauth_transactions FOR EACH ROW EXECUTE FUNCTION reject_oauth_authority_mutation();
CREATE TRIGGER oauth_codes_immutable BEFORE UPDATE ON oauth_codes FOR EACH ROW EXECUTE FUNCTION reject_oauth_authority_mutation();
CREATE TRIGGER oauth_consents_immutable BEFORE UPDATE ON oauth_consents FOR EACH ROW EXECUTE FUNCTION reject_oauth_authority_mutation();
CREATE TRIGGER oauth_sessions_immutable BEFORE UPDATE ON oauth_sessions FOR EACH ROW EXECUTE FUNCTION reject_oauth_authority_mutation();
CREATE TRIGGER oauth_tokens_immutable BEFORE UPDATE ON oauth_access_tokens FOR EACH ROW EXECUTE FUNCTION reject_oauth_authority_mutation();
