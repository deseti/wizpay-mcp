-- WP1: separate authentication issuer from wallet provider without rewriting records.
-- Empty wallet_provider denotes the exact pre-WP1 representation and digest.
ALTER TABLE intents ADD COLUMN wallet_provider text NOT NULL DEFAULT '';

-- Discover PostgreSQL's potentially truncated names by their referenced tables.
DO $$
DECLARE item record;
BEGIN
 FOR item IN SELECT conname FROM pg_constraint WHERE conrelid='wallet_bindings'::regclass
   AND contype='f' AND confrelid='identities'::regclass LOOP
   EXECUTE format('ALTER TABLE wallet_bindings DROP CONSTRAINT %I',item.conname);
 END LOOP;
 FOR item IN SELECT conname FROM pg_constraint WHERE conrelid='intents'::regclass
   AND contype='f' AND confrelid='wallet_binding_versions'::regclass LOOP
   EXECUTE format('ALTER TABLE intents DROP CONSTRAINT %I',item.conname);
 END LOOP;
END $$;
ALTER TABLE wallet_bindings ADD CONSTRAINT wallet_bindings_owner_fk
 FOREIGN KEY (tenant_id,user_id) REFERENCES identities(tenant_id,user_id) ON DELETE RESTRICT;
ALTER TABLE intents ADD CONSTRAINT intents_auth_identity_fk
 FOREIGN KEY (tenant_id,user_id,identity_provider) REFERENCES identities(tenant_id,user_id,provider) ON DELETE RESTRICT;
ALTER TABLE wallet_binding_versions ADD CONSTRAINT wallet_versions_relationship_unique
 UNIQUE (tenant_id,binding_id,version,user_id,provider_user_reference,wallet_id,address,chain_id,network);
ALTER TABLE intents ADD CONSTRAINT intents_wallet_relationship_fk
 FOREIGN KEY (tenant_id,wallet_binding_id,wallet_binding_version,user_id,provider_user_reference,wallet_id,wallet_address,chain_id,network)
 REFERENCES wallet_binding_versions(tenant_id,binding_id,version,user_id,provider_user_reference,wallet_id,address,chain_id,network) ON DELETE RESTRICT;

CREATE FUNCTION validate_intent_wallet_provider() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND NEW.wallet_provider IS DISTINCT FROM OLD.wallet_provider THEN
   RAISE EXCEPTION 'intent wallet provider is immutable' USING ERRCODE='55000';
 END IF;
 IF NOT EXISTS (SELECT 1 FROM wallet_binding_versions b
   WHERE b.tenant_id=NEW.tenant_id AND b.binding_id=NEW.wallet_binding_id AND b.version=NEW.wallet_binding_version
     AND b.user_id=NEW.user_id AND b.provider=COALESCE(NULLIF(NEW.wallet_provider,''),NEW.identity_provider)
     AND b.provider_user_reference=NEW.provider_user_reference AND b.wallet_id=NEW.wallet_id
     AND b.address=NEW.wallet_address AND b.chain_id=NEW.chain_id AND b.network=NEW.network) THEN
   RAISE EXCEPTION 'intent wallet provider relationship is invalid' USING ERRCODE='23503';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER intents_wallet_provider_relationship BEFORE INSERT OR UPDATE ON intents
 FOR EACH ROW EXECUTE FUNCTION validate_intent_wallet_provider();
-- Forward-only. Old binaries fail closed restoring new explicit-provider digests.
-- Restoring old constraints is not possible after separated-provider records exist.

-- Migration 005 dropped the expiry check (approvals_check), leaving the older
-- lifecycle check (approvals_check1) which forbids READY_FOR_EXECUTION_CONFIRMATION.
-- Keep its replacement lifecycle check and restore the missing expiry invariant.
ALTER TABLE approvals DROP CONSTRAINT approvals_check1;
ALTER TABLE approvals ADD CONSTRAINT approvals_expiry_after_creation CHECK (expires_at > created_at);
