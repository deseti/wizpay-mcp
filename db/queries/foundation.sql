-- name: CreateTenant :one
INSERT INTO tenants (tenant_id, created_at) VALUES ($1, $2)
ON CONFLICT (tenant_id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id
RETURNING *;

-- name: CreateIdentity :one
INSERT INTO identities (tenant_id, user_id, provider, provider_subject, status, lifecycle_version, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, 1, $6, $6)
RETURNING *;

-- name: FindIdentityByID :one
SELECT * FROM identities WHERE tenant_id = $1 AND user_id = $2 AND user_id = sqlc.arg(actor_id);

-- name: CreateWalletBinding :one
INSERT INTO wallet_bindings (tenant_id, binding_id, version, user_id, provider, provider_user_reference, wallet_id, address, chain_id, network, status, verification_reference, created_at, verified_at, revoked_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
RETURNING *;

-- name: FindWalletBindingByID :one
SELECT * FROM wallet_bindings WHERE tenant_id = $1 AND binding_id = $2 AND user_id = sqlc.arg(actor_id);

-- name: FindWalletBindingByWallet :one
SELECT * FROM wallet_bindings WHERE tenant_id = $1 AND chain_id = $2 AND network = $3 AND address = $4 AND user_id = sqlc.arg(actor_id);

-- name: UpdateWalletBinding :one
UPDATE wallet_bindings SET version=$3, status=$4, verification_reference=$5, verified_at=$6, revoked_at=$7
WHERE tenant_id=$1 AND binding_id=$2 AND version=$8 AND $3=$8+1 AND user_id=sqlc.arg(actor_id) RETURNING *;

-- name: CreateIntent :one
INSERT INTO intents (tenant_id,intent_id,intent_version,client_request_id,nonce,intent_type,user_id,identity_provider,provider_user_reference,wallet_binding_id,wallet_binding_version,wallet_id,wallet_address,chain_id,network,financial,route_type,route_reference,route_version,constraint_deadline,policy_reference,created_at,expires_at,status,intent_digest,operation_key,operation_version,lifecycle_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)
RETURNING *;

-- name: FindIntentByID :one
SELECT * FROM intents WHERE tenant_id=$1 AND intent_id=$2 AND user_id=sqlc.arg(actor_id);

-- name: FindIntentByClientRequestID :one
SELECT * FROM intents WHERE tenant_id=$1 AND client_request_id=$2 AND user_id=sqlc.arg(actor_id);

-- name: FindIntentByOperationKey :one
SELECT * FROM intents WHERE tenant_id=$1 AND operation_key=$2 AND operation_version=$3 AND user_id=sqlc.arg(actor_id);

-- name: UpdateIntent :one
UPDATE intents SET status=$3, lifecycle_version=$4
WHERE tenant_id=$1 AND intent_id=$2 AND status <> 'DRAFT' AND lifecycle_version=$5 AND $4=$5+1 AND user_id=sqlc.arg(actor_id) RETURNING *;

-- name: FreezeIntent :one
UPDATE intents SET status='CREATED', intent_digest=$3, operation_key=$4, operation_version=$5, lifecycle_version=$6
WHERE tenant_id=$1 AND intent_id=$2 AND status='DRAFT' AND lifecycle_version=$7 AND $6=$7+1 AND user_id=sqlc.arg(actor_id) RETURNING *;

-- name: CreateApproval :one
INSERT INTO approvals (tenant_id,approval_id,approval_version,approval_request_id,intent_id,intent_version,intent_digest,user_id,wallet_binding_id,wallet_binding_version,wallet_id,wallet_address,chain_id,status,decision,created_at,expires_at,decided_at,consumed_at,operation_key,operation_version,lifecycle_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
RETURNING *;

-- name: FindApprovalByID :one
SELECT * FROM approvals WHERE tenant_id=$1 AND approval_id=$2 AND user_id=sqlc.arg(actor_id);

-- name: FindApprovalByIntent :one
SELECT * FROM approvals WHERE tenant_id=$1 AND intent_id=$2 AND intent_version=$3 AND intent_digest=$4 AND user_id=sqlc.arg(actor_id);

-- name: UpdateApproval :one
UPDATE approvals SET status=$3,decision=$4,decided_at=$5,consumed_at=$6,operation_key=$7,operation_version=$8,lifecycle_version=$9
WHERE tenant_id=$1 AND approval_id=$2 AND lifecycle_version=$10 AND $9=$10+1 AND user_id=sqlc.arg(actor_id) RETURNING *;

-- name: CreatePolicy :one
INSERT INTO policies (tenant_id,policy_id,policy_version,name,user_id,wallet_binding_id,intent_types,rules,status,created_at,valid_from,expires_at,lifecycle_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING *;

-- name: FindPolicyByID :one
SELECT * FROM policies WHERE tenant_id=$1 AND policy_id=$2 AND policy_version=$3 AND user_id=sqlc.arg(actor_id);

-- name: FindApplicablePolicies :many
WITH current_versions AS (
  SELECT DISTINCT ON (tenant_id, policy_id) * FROM policies
  WHERE tenant_id=$1
  ORDER BY tenant_id, policy_id, policy_version DESC
)
SELECT * FROM current_versions
WHERE user_id=$2 AND (wallet_binding_id IS NULL OR wallet_binding_id=$3)
  AND (cardinality(intent_types)=0 OR sqlc.arg(intent_type)::text = ANY(intent_types))
  AND status='ACTIVE' AND valid_from <= sqlc.arg(evaluated_at)::timestamptz
  AND expires_at > sqlc.arg(evaluated_at)::timestamptz
ORDER BY policy_id, policy_version;

-- name: UpdatePolicy :one
UPDATE policies SET status=$4,lifecycle_version=$5
WHERE tenant_id=$1 AND policy_id=$2 AND policy_version=$3 AND lifecycle_version=$6 AND $5=$6+1 AND user_id=sqlc.arg(actor_id) RETURNING *;

-- name: CreatePolicyEvaluation :one
INSERT INTO policy_evaluations (tenant_id,evaluation_key,policy_id,policy_version,user_id,intent_id,intent_version,intent_digest,stage,decision,evaluated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING *;

-- name: CreatePolicyFinding :exec
INSERT INTO policy_evaluation_findings (tenant_id,evaluation_key,finding_index,rule_id,rule_type,decision,reason)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (tenant_id,evaluation_key,finding_index) DO NOTHING;

-- name: FindPolicyEvaluation :one
SELECT * FROM policy_evaluations WHERE tenant_id=$1 AND evaluation_key=$2 AND user_id=sqlc.arg(actor_id);

-- name: FindPolicyFindings :many
SELECT * FROM policy_evaluation_findings WHERE tenant_id=$1 AND evaluation_key=$2 ORDER BY finding_index;

-- name: CreateExecutionRequest :one
INSERT INTO execution_requests (tenant_id,request_id,request_key,request_version,execution_id,operation_key,operation_version,intent_id,intent_version,intent_digest,approval_id,approval_version,user_id,policy_id,policy_version,policy_evaluation_key,policy_evaluation_stage,policy_evaluated_at,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
RETURNING *;

-- name: CreateExecution :one
INSERT INTO executions (tenant_id,execution_id,request_id,status,revision,created_at,updated_at,failure_code,failure_eligibility,failure_recovery_target,failed_from_status,recovery_reason_code,recovery_from_status,recovery_target_status)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING *;

-- name: FindExecutionByID :one
SELECT e.*, r.request_key, r.request_version, r.operation_key, r.operation_version, r.intent_id, r.intent_version, r.intent_digest, r.approval_id, r.approval_version, r.policy_id, r.policy_version, r.policy_evaluation_key, r.policy_evaluation_stage, r.policy_evaluated_at, r.created_at AS request_created_at
FROM executions e JOIN execution_requests r USING (tenant_id, request_id)
WHERE e.tenant_id=$1 AND e.execution_id=$2 AND r.user_id=sqlc.arg(actor_id);

-- name: FindExecutionByRequestKey :one
SELECT e.*, r.request_key, r.request_version, r.operation_key, r.operation_version, r.intent_id, r.intent_version, r.intent_digest, r.approval_id, r.approval_version, r.policy_id, r.policy_version, r.policy_evaluation_key, r.policy_evaluation_stage, r.policy_evaluated_at, r.created_at AS request_created_at
FROM executions e JOIN execution_requests r USING (tenant_id, request_id)
WHERE e.tenant_id=$1 AND r.request_key=$2 AND r.request_version=$3 AND r.user_id=sqlc.arg(actor_id);

-- name: FindExecutionByOperationKey :one
SELECT e.*, r.request_key, r.request_version, r.operation_key, r.operation_version, r.intent_id, r.intent_version, r.intent_digest, r.approval_id, r.approval_version, r.policy_id, r.policy_version, r.policy_evaluation_key, r.policy_evaluation_stage, r.policy_evaluated_at, r.created_at AS request_created_at
FROM executions e JOIN execution_requests r USING (tenant_id, request_id)
WHERE e.tenant_id=$1 AND r.operation_key=$2 AND r.operation_version=$3 AND r.user_id=sqlc.arg(actor_id);

-- name: UpdateExecution :one
UPDATE executions SET status=$3,revision=$4,updated_at=$5,failure_code=$6,failure_eligibility=$7,failure_recovery_target=$8,failed_from_status=$9,recovery_reason_code=$10,recovery_from_status=$11,recovery_target_status=$12
WHERE executions.tenant_id=$1 AND executions.execution_id=$2 AND executions.revision=$13 AND $4=$13+1
  AND EXISTS (SELECT 1 FROM execution_requests r WHERE r.tenant_id=executions.tenant_id AND r.request_id=executions.request_id AND r.user_id=sqlc.arg(actor_id))
RETURNING *;

-- name: ClaimExecutionWork :one
UPDATE execution_runtime_work w
SET lease_owner=sqlc.arg(lease_owner), lease_expires_at=clock_timestamp() + (sqlc.arg(lease_duration_microseconds)::bigint * interval '1 microsecond'), fencing_token=w.fencing_token+1
FROM execution_requests r, executions e
WHERE w.tenant_id=sqlc.arg(tenant_id) AND w.execution_id=sqlc.arg(execution_id)
  AND r.tenant_id=w.tenant_id AND r.execution_id=w.execution_id AND r.user_id=sqlc.arg(actor_id)
  AND e.tenant_id=w.tenant_id AND e.execution_id=w.execution_id
  AND w.next_run_at <= clock_timestamp()
  AND (w.lease_expires_at IS NULL OR w.lease_expires_at <= clock_timestamp())
  AND e.status NOT IN ('COMPLETED','CANCELLED')
  AND NOT (e.status='FAILED' AND e.failure_eligibility='TERMINAL')
RETURNING w.*;

-- name: ClaimNextExecutionWork :one
WITH candidate AS (
  SELECT w.tenant_id, w.execution_id
  FROM execution_runtime_work w
  JOIN executions e USING (tenant_id, execution_id)
  WHERE w.next_run_at <= clock_timestamp()
    AND (w.lease_expires_at IS NULL OR w.lease_expires_at <= clock_timestamp())
    AND e.status NOT IN ('COMPLETED','CANCELLED')
    AND NOT (e.status='FAILED' AND e.failure_eligibility='TERMINAL')
  ORDER BY w.next_run_at,w.tenant_id,w.execution_id
  FOR UPDATE OF w SKIP LOCKED
  LIMIT 1
)
UPDATE execution_runtime_work w
SET lease_owner=sqlc.arg(lease_owner), lease_expires_at=clock_timestamp() + (sqlc.arg(lease_duration_microseconds)::bigint * interval '1 microsecond'), fencing_token=w.fencing_token+1
FROM candidate c
WHERE w.tenant_id=c.tenant_id AND w.execution_id=c.execution_id
RETURNING w.*;

-- name: MarkSubmissionStarted :one
UPDATE execution_runtime_work
SET submission_started=true
WHERE tenant_id=$1 AND execution_id=$2 AND lease_owner=$3 AND fencing_token=$4
  AND lease_expires_at > clock_timestamp() AND submission_started=false
RETURNING *;

-- name: ResetSubmissionStarted :execrows
UPDATE execution_runtime_work
SET submission_started=false
WHERE tenant_id=$1 AND execution_id=$2 AND lease_owner=$3 AND fencing_token=$4
  AND lease_expires_at > clock_timestamp() AND submission_started=true;

-- name: ValidateExecutionClaim :execrows
UPDATE execution_runtime_work
SET next_run_at=next_run_at
WHERE tenant_id=$1 AND execution_id=$2 AND lease_owner=$3 AND fencing_token=$4
  AND lease_expires_at > clock_timestamp();

-- name: ReleaseExecutionWork :execrows
UPDATE execution_runtime_work
SET lease_owner='',lease_expires_at=NULL,next_run_at=sqlc.arg(next_run_at)
WHERE tenant_id=$1 AND execution_id=$2 AND lease_owner=$3 AND fencing_token=$4;

-- name: FindExecutionOwner :one
SELECT tenant_id,user_id FROM execution_requests WHERE tenant_id=$1 AND execution_id=$2;

-- name: AppendVerificationEvidence :execrows
INSERT INTO verification_evidence (tenant_id,execution_id,execution_version,status,adapter_reference,error_code,observed_at)
SELECT $1,$2,$3,$4,$5,$6,$7
FROM executions e JOIN execution_requests r ON r.tenant_id=e.tenant_id AND r.request_id=e.request_id
WHERE e.tenant_id=$1 AND e.execution_id=$2 AND e.revision=$3 AND e.status=$4 AND r.user_id=sqlc.arg(actor_id);

-- name: FindVerificationEvidence :many
SELECT v.* FROM verification_evidence v
JOIN executions e ON e.tenant_id=v.tenant_id AND e.execution_id=v.execution_id
JOIN execution_requests r ON r.tenant_id=e.tenant_id AND r.request_id=e.request_id
WHERE v.tenant_id=$1 AND v.execution_id=$2 AND r.user_id=sqlc.arg(actor_id)
ORDER BY v.observed_at,v.evidence_id;

-- name: AppendAudit :exec
INSERT INTO audit_records (tenant_id,event_id,event_type,occurred_at,actor_type,actor_id,request_id,trace_id,resource_type,resource_id,previous_state,new_state,safe_reason_code,source_component,intent_id,intent_version,intent_digest,approval_id,policy_id,policy_version,policy_decision,policy_evaluation_key,execution_id,execution_revision,execution_request_id,execution_request_key,execution_status,recovery_reason_code,wallet_binding_id,wallet_binding_version,user_id,operation_key,operation_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33);

-- name: FindAuditByResource :many
SELECT * FROM audit_records WHERE tenant_id=$1 AND resource_type=$2 AND resource_id=$3 ORDER BY occurred_at,event_id;
