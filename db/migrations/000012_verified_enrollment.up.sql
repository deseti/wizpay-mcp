-- Trusted authentication runtime attests signature verification; delegated owner
-- independently decides application identity entitlement. Neither role can do both.
ALTER TABLE siwe_challenges ADD COLUMN purpose text NOT NULL DEFAULT 'AUTHENTICATE'
 CHECK(purpose IN ('AUTHENTICATE','ENROLL'));
CREATE UNIQUE INDEX wallet_external_address_unique ON wallet_bindings
 (tenant_id,chain_id,network,lower(address)) WHERE provider='EXTERNAL_EVM';
CREATE TABLE wallet_enrollment_proofs (
 challenge_id text PRIMARY KEY REFERENCES siwe_challenges(challenge_id),
 tenant_id text NOT NULL, user_id text NOT NULL, binding_id text NOT NULL,
 binding_version bigint NOT NULL, verified_at timestamptz NOT NULL,
 message_digest text NOT NULL CHECK(message_digest ~ '^[0-9a-f]{64}$'),
 FOREIGN KEY(tenant_id,binding_id,binding_version,user_id)
 REFERENCES wallet_binding_versions(tenant_id,binding_id,version,user_id)
);
CREATE FUNCTION enforce_enrollment_proof_boundary() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public,pg_temp AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.siwe_challenges c JOIN public.wallet_bindings b
 ON b.tenant_id=c.tenant_id AND lower(b.address)=lower(c.address)
 WHERE c.challenge_id=NEW.challenge_id AND c.purpose='ENROLL' AND c.consumed_at=NEW.verified_at
 AND NEW.tenant_id=c.tenant_id AND NEW.user_id=b.user_id AND NEW.binding_id=b.binding_id
 AND NEW.binding_version=b.version AND b.status='PENDING' AND b.provider='EXTERNAL_EVM'
 AND b.chain_id=c.chain_id AND b.network='MAINNET'
 AND NEW.message_digest=encode(sha256(convert_to(c.message,'UTF8')),'hex')) THEN
  RAISE EXCEPTION 'Enrollment evidence denied' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER wallet_enrollment_proofs_boundary BEFORE INSERT ON wallet_enrollment_proofs
 FOR EACH ROW EXECUTE FUNCTION enforce_enrollment_proof_boundary();
CREATE TRIGGER wallet_enrollment_proofs_immutable BEFORE UPDATE OR DELETE ON wallet_enrollment_proofs
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
CREATE TABLE wallet_enrollment_approvals (
 tenant_id text NOT NULL, binding_id text NOT NULL, user_id text NOT NULL,
 challenge_id text NOT NULL UNIQUE REFERENCES wallet_enrollment_proofs(challenge_id),
 request_id text NOT NULL, login_role name NOT NULL, review_reference text NOT NULL,
 approved_at timestamptz NOT NULL, PRIMARY KEY(tenant_id,binding_id),
 FOREIGN KEY(tenant_id,user_id) REFERENCES identities(tenant_id,user_id),
 FOREIGN KEY(tenant_id,binding_id) REFERENCES wallet_bindings(tenant_id,binding_id)
);
CREATE TRIGGER wallet_enrollment_approvals_immutable BEFORE UPDATE OR DELETE ON wallet_enrollment_approvals
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
REVOKE ALL ON wallet_enrollment_proofs,wallet_enrollment_approvals FROM PUBLIC;

ALTER TABLE onboarding_administration_audit DROP CONSTRAINT onboarding_administration_audit_action_check;
ALTER TABLE onboarding_administration_audit ADD CHECK(action IN ('STAGE','ACTIVATE','REVOKE_TENANT','REVOKE_IDENTITY','REVOKE_BINDING'));

CREATE FUNCTION admin_activate_wallet(p_request text,p_tenant text,p_user text,p_binding text,
 p_proof text,p_review text,p_expires timestamptz) RETURNS void
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public,pg_temp AS $$
DECLARE grant_expiry timestamptz; at_time timestamptz; value text; version_now bigint;
BEGIN
 IF array_position(ARRAY[p_request,p_tenant,p_user,p_binding,p_proof,p_review],NULL) IS NOT NULL OR p_expires IS NULL THEN
  RAISE EXCEPTION 'Activation denied' USING ERRCODE='42501';
 END IF;
 FOREACH value IN ARRAY ARRAY[p_request,p_tenant,p_user,p_binding,p_review] LOOP
  IF value !~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$' THEN
   RAISE EXCEPTION 'Activation denied' USING ERRCODE='42501';
  END IF;
 END LOOP;
 SELECT expires_at INTO grant_expiry FROM public.onboarding_administrators
 WHERE login_role=session_user AND tenant_id=p_tenant FOR SHARE;
 at_time:=clock_timestamp();
 IF NOT FOUND OR grant_expiry<=at_time OR p_expires<=at_time OR p_expires>at_time+interval '15 minutes' THEN
  RAISE EXCEPTION 'Activation denied' USING ERRCODE='42501';
 END IF;
 -- Same identity -> tenant -> binding order as authentication/redemption.
 PERFORM 1 FROM public.identities WHERE tenant_id=p_tenant AND user_id=p_user AND status IN ('UNKNOWN','ACTIVE') FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'Activation denied' USING ERRCODE='42501'; END IF;
 PERFORM 1 FROM public.tenants WHERE tenant_id=p_tenant AND status IN ('INACTIVE','ACTIVE') FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'Activation denied' USING ERRCODE='42501'; END IF;
 SELECT version INTO version_now FROM public.wallet_bindings WHERE tenant_id=p_tenant AND user_id=p_user
 AND binding_id=p_binding AND provider='EXTERNAL_EVM' AND status='PENDING' AND revoked_at IS NULL
 AND chain_id='5042' AND network='MAINNET' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'Activation denied' USING ERRCODE='42501'; END IF;
 PERFORM 1 FROM public.wallet_enrollment_proofs e JOIN public.siwe_challenges c USING(challenge_id)
 JOIN public.wallet_bindings b ON b.tenant_id=e.tenant_id AND b.binding_id=e.binding_id
 WHERE e.challenge_id=p_proof AND e.tenant_id=p_tenant AND e.user_id=p_user AND e.binding_id=p_binding
 AND e.binding_version=version_now AND c.purpose='ENROLL' AND c.consumed_at=e.verified_at
 AND c.tenant_id=e.tenant_id AND lower(c.address)=lower(b.address)
 AND e.verified_at<=clock_timestamp() AND e.verified_at>clock_timestamp()-interval '24 hours';
 IF NOT FOUND THEN RAISE EXCEPTION 'Activation denied' USING ERRCODE='42501'; END IF;
 -- request IDs share the existing audit replay namespace.
 INSERT INTO public.onboarding_administration_audit VALUES(p_tenant,p_request,session_user,'ACTIVATE',p_user,p_binding,p_review,at_time);
 INSERT INTO public.wallet_enrollment_approvals VALUES(p_tenant,p_binding,p_user,p_proof,p_request,session_user,p_review,at_time);
 UPDATE public.identities SET status='ACTIVE',lifecycle_version=lifecycle_version+1,updated_at=at_time
 WHERE tenant_id=p_tenant AND user_id=p_user AND status='UNKNOWN';
 UPDATE public.tenants SET status='ACTIVE' WHERE tenant_id=p_tenant AND status='INACTIVE';
 UPDATE public.wallet_bindings SET status='ACTIVE',version=version+1,verified_at=at_time,
 verification_reference='enrollment:'||p_proof WHERE tenant_id=p_tenant AND binding_id=p_binding;
 at_time:=clock_timestamp();
 IF at_time>=grant_expiry OR at_time>=p_expires THEN RAISE EXCEPTION 'Activation denied' USING ERRCODE='42501'; END IF;
END $$;
REVOKE ALL ON FUNCTION admin_activate_wallet(text,text,text,text,text,text,timestamptz) FROM PUBLIC;

-- Existing identities require exact immutable issuer/subject attribution.
CREATE OR REPLACE FUNCTION admin_onboarding(
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
  PERFORM 1 FROM public.identities WHERE tenant_id=p_tenant AND user_id=p_user FOR UPDATE;
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
   VALUES(p_tenant,p_user,p_issuer,p_subject,'UNKNOWN',at_time,at_time) ON CONFLICT(tenant_id,user_id) DO NOTHING;
  PERFORM 1 FROM public.identities WHERE tenant_id=p_tenant AND user_id=p_user AND provider=p_issuer AND provider_subject=p_subject AND status IN ('UNKNOWN','ACTIVE') FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'Administration denied' USING ERRCODE='42501'; END IF;
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

-- Every MCP admission attempt is attributable without retaining credentials.
CREATE TABLE mcp_access_audit (
 event_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 request_id text NOT NULL CHECK(request_id ~ '^[0-9a-f]{32}$'),
 tenant_id text NOT NULL, user_id text NOT NULL, client_id text NOT NULL,
 http_method text NOT NULL CHECK(http_method IN ('GET','POST','DELETE','OPTIONS','OTHER')),
 decision text NOT NULL CHECK(decision IN ('RECEIVED','AUTHORIZED')),
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(decision<>'AUTHORIZED' OR (tenant_id<>'' AND user_id<>'' AND client_id<>''))
);
CREATE TRIGGER mcp_access_audit_immutable BEFORE UPDATE OR DELETE ON mcp_access_audit
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_record_mutation();
REVOKE ALL ON mcp_access_audit FROM PUBLIC;
