-- No owner role, tenant, identity, binding or activation is seeded.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
-- Only the migration owner may populate/withdraw this expiring delegation.
CREATE TABLE onboarding_administrators (
 login_role name NOT NULL,
 tenant_id text NOT NULL CHECK(tenant_id ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$'),
 expires_at timestamptz NOT NULL CHECK(isfinite(expires_at)),
 PRIMARY KEY(login_role,tenant_id)
);
CREATE TABLE onboarding_administration_audit (
 tenant_id text NOT NULL,
 request_id text NOT NULL,
 login_role name NOT NULL,
 action text NOT NULL CHECK(action IN ('STAGE','REVOKE_TENANT','REVOKE_IDENTITY','REVOKE_BINDING')),
 user_id text NOT NULL,
 binding_id text NOT NULL,
 review_reference text NOT NULL,
 applied_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,request_id)
);
CREATE TRIGGER onboarding_audit_immutable BEFORE UPDATE OR DELETE ON onboarding_administration_audit
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
REVOKE ALL ON onboarding_administrators,onboarding_administration_audit FROM PUBLIC;

CREATE FUNCTION enforce_identity_administration_boundary() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public,pg_temp AS $$
BEGIN
 IF ROW(NEW.tenant_id,NEW.user_id,NEW.provider,NEW.provider_subject,NEW.created_at)
    IS DISTINCT FROM ROW(OLD.tenant_id,OLD.user_id,OLD.provider,OLD.provider_subject,OLD.created_at)
    OR (OLD.status='REVOKED' AND NEW.status<>'REVOKED')
    OR (NEW.status<>OLD.status AND NEW.lifecycle_version<>OLD.lifecycle_version+1) THEN
  RAISE EXCEPTION 'Identity authority mutation denied' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER identities_administration_boundary BEFORE UPDATE ON identities
 FOR EACH ROW EXECUTE FUNCTION enforce_identity_administration_boundary();
CREATE FUNCTION enforce_wallet_terminal_revocation() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public,pg_temp AS $$
BEGIN
 IF OLD.status='REVOKED' AND (NEW.status<>'REVOKED' OR NEW.revoked_at IS DISTINCT FROM OLD.revoked_at) THEN
  RAISE EXCEPTION 'Wallet revocation is terminal' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER wallet_terminal_revocation BEFORE UPDATE ON wallet_bindings
 FOR EACH ROW EXECUTE FUNCTION enforce_wallet_terminal_revocation();

-- SECURITY DEFINER has fixed SQL and a trusted search path. The authority is the
-- actual login (session_user), never SET ROLE/current_user or request JSON.
-- This function can stage or remove authority, never manufacture verified proof.
CREATE FUNCTION admin_onboarding(
 p_action text,p_request text,p_tenant text,p_user text,p_binding text,
 p_issuer text,p_subject text,p_address text,p_review text,p_expires timestamptz
) RETURNS void LANGUAGE plpgsql SECURITY DEFINER
SET search_path=pg_catalog,public,pg_temp AS $$
DECLARE
 grant_expiry timestamptz;
 at_time timestamptz;
 value text;
 current_status text;
BEGIN
 IF p_expires IS NULL OR array_position(ARRAY[p_action,p_request,p_tenant,p_user,p_binding,p_issuer,p_subject,p_address,p_review],NULL) IS NOT NULL THEN
  RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
 END IF;
 FOREACH value IN ARRAY ARRAY[p_request,p_tenant,p_review] LOOP
  IF value !~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$' THEN
   RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
  END IF;
 END LOOP;
 SELECT expires_at INTO grant_expiry FROM public.onboarding_administrators
  WHERE login_role=session_user AND tenant_id=p_tenant FOR SHARE;
 at_time := clock_timestamp();
 IF NOT FOUND OR grant_expiry<=at_time OR p_expires<=at_time OR p_expires>at_time+interval '15 minutes' THEN
  RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
 END IF;
 -- Insert first: a repeated request (including changed payload) fails closed;
 -- any later denial rolls back both the audit and the state change.
 INSERT INTO public.onboarding_administration_audit
  VALUES(p_tenant,p_request,session_user,p_action,p_user,p_binding,p_review,at_time);
 IF p_action='STAGE' THEN
  FOREACH value IN ARRAY ARRAY[p_user,p_binding,p_issuer,p_subject] LOOP
   IF value !~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$' THEN
    RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
   END IF;
  END LOOP;
  IF p_address !~ '^0x[0-9a-fA-F]{40}$' THEN
   RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
  END IF;
  INSERT INTO public.tenants(tenant_id,created_at,status) VALUES(p_tenant,at_time,'INACTIVE') ON CONFLICT DO NOTHING;
  SELECT status INTO current_status FROM public.tenants WHERE tenant_id=p_tenant FOR UPDATE;
  IF current_status='REVOKED' THEN
   RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
  END IF;
  -- Includes revoked and pending matches; ownership conflicts are never ignored.
  IF EXISTS(SELECT 1 FROM public.wallet_bindings WHERE tenant_id=p_tenant AND chain_id='5042' AND network='MAINNET' AND lower(address)=lower(p_address)) THEN
   RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
  END IF;
  INSERT INTO public.identities(tenant_id,user_id,provider,provider_subject,status,created_at,updated_at)
   VALUES(p_tenant,p_user,p_issuer,p_subject,'UNKNOWN',at_time,at_time);
  INSERT INTO public.wallet_bindings(tenant_id,binding_id,version,user_id,provider,provider_user_reference,wallet_id,address,chain_id,network,status,created_at)
   VALUES(p_tenant,p_binding,1,p_user,'EXTERNAL_EVM',p_user,p_binding,p_address,'5042','MAINNET','PENDING',at_time);
 ELSE
  IF p_issuer<>'' OR p_subject<>'' OR p_address<>'' OR
     (p_action='REVOKE_TENANT' AND (p_user<>'' OR p_binding<>'')) OR
     (p_action IN ('REVOKE_IDENTITY','REVOKE_BINDING') AND p_user !~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$') OR
     (p_action='REVOKE_IDENTITY' AND p_binding<>'') OR
     (p_action='REVOKE_BINDING' AND p_binding !~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$') THEN
   RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
  END IF;
  CASE p_action
   WHEN 'REVOKE_TENANT' THEN
    UPDATE public.tenants SET status='REVOKED' WHERE tenant_id=p_tenant AND status<>'REVOKED';
   WHEN 'REVOKE_IDENTITY' THEN
    UPDATE public.identities SET status='REVOKED',lifecycle_version=lifecycle_version+1,updated_at=clock_timestamp()
     WHERE tenant_id=p_tenant AND user_id=p_user AND status<>'REVOKED';
   WHEN 'REVOKE_BINDING' THEN
    UPDATE public.wallet_bindings SET status='REVOKED',version=version+1,revoked_at=clock_timestamp()
     WHERE tenant_id=p_tenant AND user_id=p_user AND binding_id=p_binding AND status<>'REVOKED';
   ELSE RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
  END CASE;
  IF NOT FOUND THEN
   RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
  END IF;
 END IF;
 at_time := clock_timestamp();
 IF at_time>=grant_expiry OR at_time>=p_expires THEN
  RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501';
 END IF;
END $$;
REVOKE ALL ON FUNCTION admin_onboarding(text,text,text,text,text,text,text,text,text,timestamptz) FROM PUBLIC;
-- No EXECUTE grant is made by migration. Separate per-login owner delegation only.
