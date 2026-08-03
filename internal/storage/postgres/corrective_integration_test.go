package postgres

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/audit"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

func requirePostgresConstraint(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("database accepted an invalid relationship")
	}
	var pgErr *pgconn.PgError
	if !stderrors.As(err, &pgErr) {
		// pgx errors returned by Exec are normally *pgconn.PgError. Keep this
		// branch explicit so an infrastructure failure is not mistaken for a
		// passed constraint test.
		t.Fatalf("expected PostgreSQL constraint error, got %T: %v", err, err)
	}
	if pgErr.Code != "23503" && pgErr.Code != "23514" && pgErr.Code != "55000" {
		t.Fatalf("unexpected SQLSTATE %s: %v", pgErr.Code, err)
	}
}

func awaitingApprovalIntent(t *testing.T, f fixture) intents.Intent {
	t.Helper()
	value := newSiblingIntent(t, f, unique("intent"), unique("client-request"))
	var err error
	value, err = value.Transition(intents.StatusApprovalRequired, fixtureNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateIntent(context.Background(), f.scope, value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestApprovalRequiresExactWalletVersionComponents(t *testing.T) {
	f := createBaseFixture(t, false)
	tests := []struct {
		name      string
		tenantID  func() string
		userID    func() string
		bindingID func() string
		version   func() uint64
		walletID  func() string
		address   func() string
		chainID   func() string
	}{
		{name: "tenant", tenantID: func() string { return unique("other-tenant") }},
		{name: "user", userID: func() string { return unique("other-user") }},
		{name: "binding_id", bindingID: func() string { return unique("other-binding") }},
		{name: "binding_version", version: func() uint64 { return f.binding.Version() + 1 }},
		{name: "wallet_id", walletID: func() string { return unique("other-wallet") }},
		{name: "wallet_address", address: func() string { return unique("other-address") }},
		{name: "chain_id", chainID: func() string { return "5042003" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := awaitingApprovalIntent(t, f)
			tenantID, userID := f.scope.TenantID(), f.scope.ActorID()
			bindingID, version := f.binding.BindingID(), f.binding.Version()
			walletID, address, chainID := f.binding.WalletID(), f.binding.Address(), f.binding.ChainID()
			if test.tenantID != nil {
				tenantID = test.tenantID()
			}
			if test.userID != nil {
				userID = test.userID()
			}
			if test.bindingID != nil {
				bindingID = test.bindingID()
			}
			if test.version != nil {
				version = test.version()
			}
			if test.walletID != nil {
				walletID = test.walletID()
			}
			if test.address != nil {
				address = test.address()
			}
			if test.chainID != nil {
				chainID = test.chainID()
			}
			_, err := integrationPool.Exec(context.Background(), `
				INSERT INTO approvals (tenant_id,approval_id,approval_version,approval_request_id,intent_id,intent_version,intent_digest,user_id,wallet_binding_id,wallet_binding_version,wallet_id,wallet_address,chain_id,status,decision,created_at,expires_at,lifecycle_version)
				VALUES ($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'PENDING','PENDING',$13,$14,1)`,
				tenantID, unique("approval"), unique("approval-request"), intent.IntentID(), int64(intent.Version()), intent.Digest(), userID, bindingID, int64(version), walletID, address, chainID, fixtureNow.Add(3*time.Second), fixtureNow.Add(20*time.Minute))
			requirePostgresConstraint(t, err)
		})
	}
}

func TestExecutionRequestRequiresExactPolicyEvaluation(t *testing.T) {
	f := createBaseFixture(t, true)
	r := f.execution.Request()
	tests := []struct {
		name   string
		mutate func(*string, *uint64, *string, *uint64, *string, *policies.EvaluationStage, *time.Time)
	}{
		{name: "policy_id", mutate: func(id *string, _ *uint64, _ *string, _ *uint64, _ *string, _ *policies.EvaluationStage, _ *time.Time) {
			*id = unique("policy")
		}},
		{name: "policy_version", mutate: func(_ *string, v *uint64, _ *string, _ *uint64, _ *string, _ *policies.EvaluationStage, _ *time.Time) {
			*v++
		}},
		{name: "stage", mutate: func(_ *string, _ *uint64, _ *string, _ *uint64, _ *string, s *policies.EvaluationStage, _ *time.Time) {
			*s = policies.EvaluationStageBeforeApproval
		}},
		{name: "evaluated_at", mutate: func(_ *string, _ *uint64, _ *string, _ *uint64, _ *string, _ *policies.EvaluationStage, at *time.Time) {
			*at = at.Add(time.Microsecond)
		}},
		{name: "intent_id", mutate: func(_ *string, _ *uint64, id *string, _ *uint64, _ *string, _ *policies.EvaluationStage, _ *time.Time) {
			*id = unique("intent")
		}},
		{name: "intent_version", mutate: func(_ *string, _ *uint64, _ *string, v *uint64, _ *string, _ *policies.EvaluationStage, _ *time.Time) {
			*v++
		}},
		{name: "intent_digest", mutate: func(_ *string, _ *uint64, _ *string, _ *uint64, digest *string, _ *policies.EvaluationStage, _ *time.Time) {
			*digest = "sha256:" + fmt.Sprintf("%064d", 7)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policyID, policyVersion := r.PolicyID(), r.PolicyVersion()
			intentID, intentVersion, digest := r.IntentID(), r.IntentVersion(), r.IntentDigest()
			stage, evaluatedAt := r.PolicyEvaluationStage(), r.PolicyEvaluatedAt()
			test.mutate(&policyID, &policyVersion, &intentID, &intentVersion, &digest, &stage, &evaluatedAt)
			_, err := integrationPool.Exec(context.Background(), `
				INSERT INTO execution_requests (tenant_id,request_id,request_key,request_version,execution_id,operation_key,operation_version,intent_id,intent_version,intent_digest,approval_id,approval_version,user_id,policy_id,policy_version,policy_evaluation_key,policy_evaluation_stage,policy_evaluated_at,created_at)
				VALUES ($1,$2,$3,1,$4,$5,1,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
				f.scope.TenantID(), unique("request"), unique("request-key"), unique("execution"), unique("operation"), intentID, int64(intentVersion), digest, r.ApprovalID(), int64(r.ApprovalVersion()), f.scope.ActorID(), policyID, int64(policyVersion), r.PolicyEvaluationKey(), string(stage), evaluatedAt, r.CreatedAt())
			requirePostgresConstraint(t, err)
		})
	}
	otherTenant := unique("tenant")
	if _, err := integrationStore.CreateTenant(context.Background(), storage.Tenant{TenantID: otherTenant, CreatedAt: fixtureNow}); err != nil {
		t.Fatal(err)
	}
	_, err := integrationPool.Exec(context.Background(), `INSERT INTO execution_requests (tenant_id,request_id,request_key,request_version,execution_id,operation_key,operation_version,intent_id,intent_version,intent_digest,approval_id,approval_version,user_id,policy_id,policy_version,policy_evaluation_key,policy_evaluation_stage,policy_evaluated_at,created_at) VALUES ($1,$2,$3,1,$4,$5,1,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, otherTenant, unique("request"), unique("key"), unique("execution"), unique("operation"), r.IntentID(), int64(r.IntentVersion()), r.IntentDigest(), r.ApprovalID(), int64(r.ApprovalVersion()), f.scope.ActorID(), r.PolicyID(), int64(r.PolicyVersion()), r.PolicyEvaluationKey(), string(r.PolicyEvaluationStage()), r.PolicyEvaluatedAt(), r.CreatedAt())
	requirePostgresConstraint(t, err)
}

func TestExecutionRetryIdentityIncludesEveryImmutableField(t *testing.T) {
	base := executionRequestIdentity{requestID: "request", requestKey: "request-key", version: 1, executionID: "execution", operationKey: "operation", operationVersion: 1, intentID: "intent", intentVersion: 1, intentDigest: "digest", approvalID: "approval", approvalVersion: 1, policyID: "policy", policyVersion: 1, policyEvaluationKey: "evaluation", policyEvaluationStage: policies.EvaluationStageBeforeExecution, policyEvaluatedAt: fixtureNow, createdAt: fixtureNow.Add(time.Second)}
	mutations := map[string]func(*executionRequestIdentity){
		"request_id":        func(v *executionRequestIdentity) { v.requestID += "-changed" },
		"request_key":       func(v *executionRequestIdentity) { v.requestKey += "-changed" },
		"request_version":   func(v *executionRequestIdentity) { v.version++ },
		"execution_id":      func(v *executionRequestIdentity) { v.executionID += "-changed" },
		"operation_key":     func(v *executionRequestIdentity) { v.operationKey += "-changed" },
		"operation_version": func(v *executionRequestIdentity) { v.operationVersion++ },
		"intent_id":         func(v *executionRequestIdentity) { v.intentID += "-changed" },
		"intent_version":    func(v *executionRequestIdentity) { v.intentVersion++ },
		"intent_digest":     func(v *executionRequestIdentity) { v.intentDigest += "-changed" },
		"approval_id":       func(v *executionRequestIdentity) { v.approvalID += "-changed" },
		"approval_version":  func(v *executionRequestIdentity) { v.approvalVersion++ },
		"policy_id":         func(v *executionRequestIdentity) { v.policyID += "-changed" },
		"policy_version":    func(v *executionRequestIdentity) { v.policyVersion++ },
		"evaluation_key":    func(v *executionRequestIdentity) { v.policyEvaluationKey += "-changed" },
		"evaluation_stage":  func(v *executionRequestIdentity) { v.policyEvaluationStage = policies.EvaluationStageBeforeApproval },
		"evaluated_at":      func(v *executionRequestIdentity) { v.policyEvaluatedAt = v.policyEvaluatedAt.Add(time.Microsecond) },
		"created_at":        func(v *executionRequestIdentity) { v.createdAt = v.createdAt.Add(time.Microsecond) },
	}
	identical := base
	if base != identical {
		t.Fatal("identical immutable identity did not compare equal")
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if changed == base {
				t.Fatalf("%s is missing from retry identity", name)
			}
		})
	}
}

func createApplicabilityPolicy(t *testing.T, f fixture, id string, version uint64, status policies.Status, validFrom, expiresAt time.Time) policies.Policy {
	t.Helper()
	value, err := policies.NewDraft(policies.Params{PolicyID: id, Version: version, Name: "Applicability policy", Scope: policies.Scope{UserID: f.scope.ActorID(), WalletBindingID: f.binding.BindingID(), IntentTypes: []intents.Type{f.intent.Type()}}, Rules: f.policy.Rules(), CreatedAt: validFrom.Add(-time.Hour), ValidFrom: validFrom, ExpiresAt: expiresAt})
	if err != nil {
		t.Fatal(err)
	}
	if status != policies.StatusDraft {
		transitionAt := validFrom
		if status == policies.StatusDisabled {
			transitionAt = validFrom.Add(time.Second)
		}
		value, err = value.Transition(status, transitionAt)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = integrationStore.CreatePolicy(context.Background(), f.scope, value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestFindApplicablePoliciesUsesDeterministicValidityAndCurrentVersion(t *testing.T) {
	f := createBaseFixture(t, false)
	at := fixtureNow.Add(10 * time.Minute)
	included := createApplicabilityPolicy(t, f, unique("included"), 1, policies.StatusActive, at.Add(-time.Minute), at.Add(time.Hour))
	equality := createApplicabilityPolicy(t, f, unique("valid-equality"), 1, policies.StatusActive, at, at.Add(time.Hour))
	_ = createApplicabilityPolicy(t, f, unique("expiry-equality"), 1, policies.StatusActive, at.Add(-time.Hour), at)
	_ = createApplicabilityPolicy(t, f, unique("inactive"), 1, policies.StatusDraft, at.Add(-time.Minute), at.Add(time.Hour))
	_ = createApplicabilityPolicy(t, f, unique("future"), 1, policies.StatusActive, at.Add(time.Minute), at.Add(time.Hour))
	multiID := unique("multi")
	_ = createApplicabilityPolicy(t, f, multiID, 1, policies.StatusActive, at.Add(-time.Minute), at.Add(time.Hour))
	_ = createApplicabilityPolicy(t, f, multiID, 2, policies.StatusDisabled, at.Add(-time.Minute), at.Add(time.Hour))

	values, err := integrationStore.FindApplicablePolicies(context.Background(), f.scope, policies.Applicability{UserID: f.scope.ActorID(), WalletBindingID: f.binding.BindingID(), IntentType: f.intent.Type(), EvaluatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, value := range values {
		found[value.PolicyID()] = true
	}
	if !found[included.PolicyID()] || !found[equality.PolicyID()] {
		t.Fatalf("applicable policies missing: %#v", found)
	}
	for _, excluded := range []string{multiID} {
		if found[excluded] {
			t.Fatalf("non-current policy %s was returned", excluded)
		}
	}
	if _, err = integrationStore.FindApplicablePolicies(context.Background(), f.scope, policies.Applicability{UserID: f.scope.ActorID(), WalletBindingID: f.binding.BindingID(), IntentType: f.intent.Type()}); apperrors.ToPublic(err).Code != apperrors.CodePolicyInvalid {
		t.Fatalf("missing deterministic evaluation time error = %v", err)
	}
}

func TestPolicyEvaluationRetryUsesUniqueViolationSemantics(t *testing.T) {
	f := createBaseFixture(t, false)
	base := f.policyResult
	base.EvaluatedAt = base.EvaluatedAt.Add(time.Second)
	created, err := integrationStore.CreatePolicyEvaluation(context.Background(), f.scope, base)
	if err != nil || created.Decision != base.Decision {
		t.Fatalf("first create: %v", err)
	}
	if replay, replayErr := integrationStore.CreatePolicyEvaluation(context.Background(), f.scope, base); replayErr != nil || replay.Decision != base.Decision {
		t.Fatalf("identical retry: %v", replayErr)
	}
	conflict := base
	conflict.Decision = policies.DecisionDeny
	if _, err = integrationStore.CreatePolicyEvaluation(context.Background(), f.scope, conflict); apperrors.ToPublic(err).Code != apperrors.CodeExecutionConflict {
		t.Fatalf("conflicting retry error = %v", err)
	}

	concurrent := base
	concurrent.EvaluatedAt = concurrent.EvaluatedAt.Add(time.Second)
	const callers = 6
	var wait sync.WaitGroup
	errs := make(chan error, callers)
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, callErr := integrationStore.CreatePolicyEvaluation(context.Background(), f.scope, concurrent)
			errs <- callErr
		}()
	}
	wait.Wait()
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatalf("concurrent identical create: %v", callErr)
		}
	}

	conflictAt := concurrent.EvaluatedAt.Add(time.Second)
	var successes, conflicts int
	errs = make(chan error, callers)
	for index := 0; index < callers; index++ {
		wait.Add(1)
		decision := policies.DecisionAllow
		if index%2 == 1 {
			decision = policies.DecisionDeny
		}
		go func(decision policies.Decision) {
			defer wait.Done()
			candidate := concurrent
			candidate.EvaluatedAt, candidate.Decision = conflictAt, decision
			_, callErr := integrationStore.CreatePolicyEvaluation(context.Background(), f.scope, candidate)
			errs <- callErr
		}(decision)
	}
	wait.Wait()
	close(errs)
	for callErr := range errs {
		if callErr == nil {
			successes++
		} else if apperrors.ToPublic(callErr).Code == apperrors.CodeExecutionConflict {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrent conflict error: %v", callErr)
		}
	}
	if successes == 0 || conflicts == 0 {
		t.Fatalf("concurrent conflicting creates successes/conflicts = %d/%d", successes, conflicts)
	}
}

func conflictingExecutionValues(t *testing.T, f fixture) (approvals.Approval, execution.Execution, execution.Execution) {
	t.Helper()
	operation, err := intents.NewOperationIdentity(f.intent)
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := f.approval.Consume(fixtureNow.Add(6*time.Second), operation)
	if err != nil {
		t.Fatal(err)
	}
	alternate := f.policyResult
	alternate.EvaluatedAt = alternate.EvaluatedAt.Add(100 * time.Millisecond)
	if _, err = integrationStore.CreatePolicyEvaluation(context.Background(), f.scope, alternate); err != nil {
		t.Fatal(err)
	}
	firstRequest, err := execution.NewRequest(f.intent, consumed, f.policyResult, fixtureNow.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	secondRequest, err := execution.NewRequest(f.intent, consumed, alternate, fixtureNow.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	first, err := execution.New(firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := execution.New(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	return consumed, first, second
}

func TestExecutionCreationRejectsEverySameOperationImmutableMismatch(t *testing.T) {
	f := createBaseFixture(t, false)
	_, first, second := conflictingExecutionValues(t, f)
	created, err := integrationStore.CreateExecution(context.Background(), f.scope, first)
	if err != nil || !created.Created {
		t.Fatalf("first execution create = %#v, %v", created, err)
	}
	if _, err = integrationStore.CreateExecution(context.Background(), f.scope, second); apperrors.ToPublic(err).Code != apperrors.CodeExecutionConflict {
		t.Fatalf("conflicting execution retry = %v", err)
	}
	if replay, err := integrationStore.CreateExecution(context.Background(), f.scope, first); err != nil || replay.Created || !equalExecutionRequest(replay.Execution.Request(), first.Request()) {
		t.Fatalf("identical execution retry = %#v, %v", replay, err)
	}
}

func TestConcurrentConflictingExecutionPreparationCreatesOnlyWinner(t *testing.T) {
	f := createBaseFixture(t, false)
	consumed, first, second := conflictingExecutionValues(t, f)
	values := []execution.Execution{first, second}
	errs := make(chan error, len(values))
	var wait sync.WaitGroup
	for index, value := range values {
		wait.Add(1)
		go func(index int, value execution.Execution) {
			defer wait.Done()
			record := auditRecord(f.scope, unique("event"), audit.EventExecutionCreated, "execution", value.ExecutionID(), fixtureNow.Add(time.Duration(10+index)*time.Second))
			_, _, callErr := integrationStore.ConsumeApprovalAndCreateExecution(context.Background(), f.scope, consumed, f.approval.LifecycleRevision(), value, record)
			errs <- callErr
		}(index, value)
	}
	wait.Wait()
	close(errs)
	var success, conflict int
	for callErr := range errs {
		if callErr == nil {
			success++
		} else if apperrors.ToPublic(callErr).Code == apperrors.CodeExecutionConflict {
			conflict++
		} else {
			t.Fatalf("concurrent execution error: %v", callErr)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent execution success/conflict = %d/%d", success, conflict)
	}
	var count int
	if err := integrationPool.QueryRow(context.Background(), `SELECT count(*) FROM executions WHERE tenant_id=$1 AND execution_id=$2`, f.scope.TenantID(), first.ExecutionID()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("logical execution count/error = %d/%v", count, err)
	}
}

func advanceStoredExecution(t *testing.T, f fixture, statuses ...execution.Status) execution.Execution {
	t.Helper()
	value := f.execution
	for index, status := range statuses {
		next, err := value.Transition(status, fixtureNow.Add(time.Duration(20+index)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = integrationStore.UpdateExecution(context.Background(), f.scope, next, value.Revision()); err != nil {
			t.Fatal(err)
		}
		value = next
	}
	return value
}

func resultForRevision(t *testing.T, value execution.Execution, status execution.Status, revision uint64, suffix string) execution.Result {
	t.Helper()
	result, err := execution.NewResult(execution.ResultParams{ExecutionID: value.ExecutionID(), ExecutionVersion: revision, Status: status, AdapterReference: "evidence-" + suffix, ObservedAt: fixtureNow.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestVerificationEvidenceRequiresCurrentExactExecutionRevision(t *testing.T) {
	f := createBaseFixture(t, true)
	submitted := advanceStoredExecution(t, f, execution.StatusAuthorized, execution.StatusQueued, execution.StatusExecuting, execution.StatusSubmitted)
	correct := resultForRevision(t, submitted, execution.StatusSubmitted, submitted.Revision(), unique("correct"))
	if err := integrationStore.AppendVerificationEvidence(context.Background(), f.scope, correct); err != nil {
		t.Fatalf("correct revision: %v", err)
	}
	confirming, err := submitted.Transition(execution.StatusConfirming, fixtureNow.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.UpdateExecution(context.Background(), f.scope, confirming, submitted.Revision()); err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]execution.Result{
		"stale":  resultForRevision(t, submitted, execution.StatusSubmitted, submitted.Revision(), unique("stale")),
		"future": resultForRevision(t, confirming, execution.StatusConfirming, confirming.Revision()+1, unique("future")),
	} {
		t.Run(name, func(t *testing.T) {
			if appendErr := integrationStore.AppendVerificationEvidence(context.Background(), f.scope, candidate); apperrors.ToPublic(appendErr).Code != apperrors.CodeExecutionConflict {
				t.Fatalf("%s evidence error = %v", name, appendErr)
			}
		})
	}
	other := createBaseFixture(t, true)
	different := resultForRevision(t, other.execution, execution.StatusSubmitted, confirming.Revision(), unique("different"))
	if err = integrationStore.AppendVerificationEvidence(context.Background(), f.scope, different); apperrors.ToPublic(err).Code != apperrors.CodeExecutionConflict {
		t.Fatalf("different execution error = %v", err)
	}
	if err = integrationStore.AppendVerificationEvidence(context.Background(), other.scope, resultForRevision(t, confirming, execution.StatusConfirming, confirming.Revision(), unique("tenant"))); apperrors.ToPublic(err).Code != apperrors.CodeExecutionConflict {
		t.Fatalf("different tenant error = %v", err)
	}
}

func TestEvidenceTransitionRollsBackOnAuditFailure(t *testing.T) {
	f := createBaseFixture(t, true)
	confirmed := advanceStoredExecution(t, f, execution.StatusAuthorized, execution.StatusQueued, execution.StatusExecuting, execution.StatusSubmitted, execution.StatusConfirming, execution.StatusConfirmed)
	verified, err := confirmed.Transition(execution.StatusVerified, fixtureNow.Add(40*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	evidence := resultForRevision(t, verified, execution.StatusVerified, verified.Revision(), unique("rollback"))
	existingAudit := auditRecord(f.scope, unique("event"), audit.EventExecutionVerified, "execution", confirmed.ExecutionID(), fixtureNow.Add(40*time.Second))
	if err = integrationStore.AppendAudit(context.Background(), f.scope, existingAudit); err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.AppendEvidenceAndVerify(context.Background(), f.scope, evidence, verified, confirmed.Revision(), existingAudit); err == nil {
		t.Fatal("duplicate audit did not roll back verification transaction")
	}
	stored, err := integrationStore.FindExecutionByID(context.Background(), f.scope, confirmed.ExecutionID())
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status() != execution.StatusConfirmed || stored.Revision() != confirmed.Revision() {
		t.Fatalf("execution advanced after rollback: %s/%d", stored.Status(), stored.Revision())
	}
	rows, err := integrationStore.FindVerificationEvidence(context.Background(), f.scope, confirmed.ExecutionID())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ExecutionVersion() == verified.Revision() {
			t.Fatal("verification evidence survived rollback")
		}
	}
}

func TestStrictLifecycleRevisionProgressionAndConcurrentWriters(t *testing.T) {
	f := createBaseFixture(t, false)
	ready, err := f.intent.Transition(intents.StatusReadyForExecution, fixtureNow.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.UpdateIntent(context.Background(), f.scope, ready, f.intent.LifecycleRevision()); err != nil {
		t.Fatalf("exact intent increment: %v", err)
	}
	if _, err = integrationStore.UpdateIntent(context.Background(), f.scope, ready, f.intent.LifecycleRevision()); apperrors.ToPublic(err).Code != apperrors.CodeExecutionConflict {
		t.Fatalf("stale intent writer: %v", err)
	}

	policyNext, err := f.policy.Transition(policies.StatusDisabled, fixtureNow.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	const writers = 2
	errs := make(chan error, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, writeErr := integrationStore.UpdatePolicy(context.Background(), f.scope, policyNext, f.policy.LifecycleRevision())
			errs <- writeErr
		}()
	}
	wait.Wait()
	close(errs)
	var succeeded, conflicted int
	for writeErr := range errs {
		if writeErr == nil {
			succeeded++
		} else if apperrors.ToPublic(writeErr).Code == apperrors.CodeExecutionConflict {
			conflicted++
		} else {
			t.Fatalf("policy writer: %v", writeErr)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("policy writers success/conflict = %d/%d", succeeded, conflicted)
	}

	_, err = integrationPool.Exec(context.Background(), `UPDATE intents SET lifecycle_version=lifecycle_version+2 WHERE tenant_id=$1 AND intent_id=$2`, f.scope.TenantID(), f.intent.IntentID())
	requirePostgresConstraint(t, err)
	_, err = integrationPool.Exec(context.Background(), `UPDATE policies SET lifecycle_version=lifecycle_version WHERE tenant_id=$1 AND policy_id=$2 AND policy_version=$3`, f.scope.TenantID(), f.policy.PolicyID(), int64(f.policy.Version()))
	requirePostgresConstraint(t, err)
}

func TestApprovalLifecycleDatabaseConstraints(t *testing.T) {
	f := createBaseFixture(t, false)
	tests := []struct {
		name, status, decision string
		decided, consumed      bool
		operation              bool
	}{
		{name: "pending_with_decision", status: "PENDING", decision: "APPROVED", decided: true},
		{name: "approved_without_time", status: "APPROVED", decision: "APPROVED"},
		{name: "rejected_with_approved_decision", status: "REJECTED", decision: "APPROVED", decided: true},
		{name: "consumed_without_reference", status: "CONSUMED", decision: "APPROVED", decided: true, consumed: true},
		{name: "non_consumed_with_reference", status: "APPROVED", decision: "APPROVED", decided: true, operation: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := awaitingApprovalIntent(t, f)
			var decidedAt, consumedAt any
			if test.decided {
				decidedAt = fixtureNow.Add(4 * time.Second)
			}
			if test.consumed {
				consumedAt = fixtureNow.Add(5 * time.Second)
			}
			var operationKey any
			var operationVersion any
			if test.operation {
				operationKey, operationVersion = unique("operation"), int64(1)
			}
			_, err := integrationPool.Exec(context.Background(), `INSERT INTO approvals (tenant_id,approval_id,approval_version,approval_request_id,intent_id,intent_version,intent_digest,user_id,wallet_binding_id,wallet_binding_version,wallet_id,wallet_address,chain_id,status,decision,created_at,expires_at,decided_at,consumed_at,operation_key,operation_version,lifecycle_version) VALUES ($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,1)`, f.scope.TenantID(), unique("approval"), unique("approval-request"), intent.IntentID(), int64(intent.Version()), intent.Digest(), f.scope.ActorID(), f.binding.BindingID(), int64(f.binding.Version()), f.binding.WalletID(), f.binding.Address(), f.binding.ChainID(), test.status, test.decision, fixtureNow.Add(3*time.Second), fixtureNow.Add(20*time.Minute), decidedAt, consumedAt, operationKey, operationVersion)
			requirePostgresConstraint(t, err)
		})
	}
}

func TestAtomicEntryPointsValidateScopeBeforeDatabaseWork(t *testing.T) {
	f := createBaseFixture(t, true)
	zero := storage.Scope{}
	record := auditRecord(f.scope, unique("event"), audit.EventExecutionCreated, "execution", f.execution.ExecutionID(), fixtureNow)
	if _, err := integrationStore.CreateIntentWithAudit(context.Background(), zero, f.intent, record); err == nil {
		t.Fatal("CreateIntentWithAudit accepted malformed scope")
	}
	if _, err := integrationStore.CreateApprovalWithAudit(context.Background(), zero, f.approval, record); err == nil {
		t.Fatal("CreateApprovalWithAudit accepted malformed scope")
	}
	if _, _, err := integrationStore.ConsumeApprovalAndCreateExecution(context.Background(), zero, f.approval, f.approval.LifecycleRevision(), f.execution, record); err == nil {
		t.Fatal("ConsumeApprovalAndCreateExecution accepted malformed scope")
	}
	if _, err := integrationStore.UpdateExecutionWithAudit(context.Background(), zero, f.execution, f.execution.Revision(), record); err == nil {
		t.Fatal("UpdateExecutionWithAudit accepted malformed scope")
	}
}

func TestSameTenantActorScopeCannotReadOrResumeAnotherActorsRecords(t *testing.T) {
	f := createBaseFixture(t, true)
	other, err := storage.NewScope(f.scope.TenantID(), unique("other-user"), unique("request"), unique("trace"))
	if err != nil {
		t.Fatal(err)
	}
	operations := []func() error{
		func() error {
			_, findErr := integrationStore.FindIdentityByID(context.Background(), other, f.identity.UserID())
			return findErr
		},
		func() error {
			_, findErr := integrationStore.FindBindingByID(context.Background(), other, f.binding.BindingID())
			return findErr
		},
		func() error {
			_, findErr := integrationStore.FindIntentByID(context.Background(), other, f.intent.IntentID())
			return findErr
		},
		func() error {
			_, findErr := integrationStore.FindApprovalByID(context.Background(), other, f.approval.ApprovalID())
			return findErr
		},
		func() error {
			_, findErr := integrationStore.FindPolicyByID(context.Background(), other, f.policy.PolicyID(), f.policy.Version())
			return findErr
		},
		func() error {
			_, findErr := integrationStore.FindPolicyEvaluation(context.Background(), other, execution.PolicyEvaluationKey(f.policyResult))
			return findErr
		},
		func() error {
			_, findErr := integrationStore.FindExecutionByID(context.Background(), other, f.execution.ExecutionID())
			return findErr
		},
	}
	for index, operation := range operations {
		if operationErr := operation(); operationErr == nil || apperrors.ToPublic(operationErr).Code == apperrors.CodeInternalError {
			t.Fatalf("actor-isolation operation %d returned %v", index, operationErr)
		}
	}
	if _, err = integrationStore.CreateExecution(context.Background(), other, f.execution); err == nil {
		t.Fatal("another actor resumed an existing execution")
	}
}

func TestRestrictedApplicationRoleCannotMutateAuditOrTriggers(t *testing.T) {
	f := createBaseFixture(t, false)
	role := unique("wizpay_app_test")
	password := unique("local_disposable_password")
	identifier := pgx.Identifier{role}.Sanitize()
	if _, err := integrationPool.Exec(context.Background(), fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD '%s' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION`, identifier, password)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = integrationPool.Exec(context.Background(), fmt.Sprintf(`DROP ROLE IF EXISTS %s`, identifier))
	})
	if _, err := integrationPool.Exec(context.Background(), fmt.Sprintf(`GRANT CONNECT ON DATABASE %s TO %s`, pgx.Identifier{integrationPool.Config().ConnConfig.Database}.Sanitize(), identifier)); err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(context.Background(), fmt.Sprintf(`GRANT USAGE ON SCHEMA public TO %s`, identifier)); err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(context.Background(), fmt.Sprintf(`GRANT SELECT, INSERT ON audit_records TO %s`, identifier)); err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(integrationURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword(role, password)
	appPool, err := pgxpool.New(context.Background(), parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer appPool.Close()
	eventID := unique("restricted-event")
	appStore, err := NewStore(appPool, 5*time.Second, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	record := auditRecord(f.scope, eventID, audit.EventIntentCreated, "intent", f.intent.IntentID(), fixtureNow)
	if err = appStore.AppendAudit(context.Background(), f.scope, record); err != nil {
		t.Fatalf("restricted audit insert: %v", err)
	}
	rows, err := appStore.FindAuditByResource(context.Background(), f.scope, "intent", f.intent.IntentID())
	if err != nil || len(rows) == 0 {
		t.Fatalf("restricted audit read rows/error = %d/%v", len(rows), err)
	}
	operations := []struct {
		name, statement string
		args            []any
	}{
		{name: "update", statement: `UPDATE audit_records SET new_state='tampered' WHERE tenant_id=$1 AND event_id=$2`, args: []any{f.scope.TenantID(), eventID}},
		{name: "delete", statement: `DELETE FROM audit_records WHERE tenant_id=$1 AND event_id=$2`, args: []any{f.scope.TenantID(), eventID}},
		{name: "disable_trigger", statement: `ALTER TABLE audit_records DISABLE TRIGGER audit_records_no_update`},
		{name: "replace_trigger_function", statement: `CREATE OR REPLACE FUNCTION public.reject_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if _, operationErr := appPool.Exec(context.Background(), operation.statement, operation.args...); operationErr == nil {
				t.Fatalf("restricted role could %s", operation.name)
			}
		})
	}
	var owner string
	if err = integrationPool.QueryRow(context.Background(), `SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='audit_records'::regclass`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner == role {
		t.Fatal("restricted application role owns audit_records")
	}
}

func TestRoleBootstrapIsRepeatableAndKeepsAuditRestricted(t *testing.T) {
	script := filepath.Join("..", "..", "..", "db", "bootstrap", "roles.sql")
	for attempt := 1; attempt <= 2; attempt++ {
		output, err := exec.Command("psql", "--dbname", integrationURL, "--file", script).CombinedOutput()
		if err != nil {
			t.Fatalf("role bootstrap attempt %d: %v: %s", attempt, err, output)
		}
	}
	var owner string
	if err := integrationPool.QueryRow(context.Background(), `SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='audit_records'::regclass`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != "wizpay_mcp_migration_owner" {
		t.Fatalf("audit owner = %s", owner)
	}
	var canUpdate, canDelete, canTrigger bool
	if err := integrationPool.QueryRow(context.Background(), `SELECT has_table_privilege('wizpay_mcp_application','audit_records','UPDATE'), has_table_privilege('wizpay_mcp_application','audit_records','DELETE'), has_table_privilege('wizpay_mcp_application','audit_records','TRIGGER')`).Scan(&canUpdate, &canDelete, &canTrigger); err != nil {
		t.Fatal(err)
	}
	if canUpdate || canDelete || canTrigger {
		t.Fatalf("application audit privileges update/delete/trigger = %t/%t/%t", canUpdate, canDelete, canTrigger)
	}
}
