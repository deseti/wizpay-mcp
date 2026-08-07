package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

var executionServiceNow = time.Date(2026, 8, 7, 15, 0, 0, 0, time.UTC)

type executionIntentRepository struct {
	storage.IntentRepository
	intent intents.Intent
}

func (r executionIntentRepository) FindIntentByID(context.Context, storage.Scope, string) (intents.Intent, error) {
	return r.intent, nil
}

type executionApprovalRepository struct {
	storage.ApprovalRepository
	approval approvals.Approval
}

func (r executionApprovalRepository) FindApprovalByID(context.Context, storage.Scope, string) (approvals.Approval, error) {
	return r.approval, nil
}

type executionPolicyRepository struct {
	storage.PolicyRepository
	finds int
}

func (r *executionPolicyRepository) FindPolicyByID(context.Context, storage.Scope, string, uint64) (policies.Policy, error) {
	r.finds++
	return policies.Policy{}, errors.New("policy lookup must not occur during consumed replay")
}

type executionEvaluationRepository struct {
	storage.PolicyEvaluationRepository
	creates int
}

func (r *executionEvaluationRepository) CreatePolicyEvaluation(context.Context, storage.Scope, policies.Result) (policies.Result, error) {
	r.creates++
	return policies.Result{}, errors.New("policy evaluation must not be created during consumed replay")
}

type executionRepository struct {
	storage.ExecutionRepository
	execution execution.Execution
	findErr   error
	finds     int
	creates   int
}

func (r *executionRepository) FindExecutionByOperationKey(_ context.Context, _ storage.Scope, key string, version uint64) (execution.Execution, error) {
	r.finds++
	if r.findErr != nil {
		return execution.Execution{}, r.findErr
	}
	if r.execution.Request().OperationKey() != key || r.execution.Request().OperationVersion() != version {
		return execution.Execution{}, apperrors.New(apperrors.CodeExecutionNotFound, "Execution is not accessible.", false, true, true)
	}
	return r.execution, nil
}

func (r *executionRepository) CreateExecution(context.Context, storage.Scope, execution.Execution) (storage.CreateExecutionResult, error) {
	r.creates++
	return storage.CreateExecutionResult{}, errors.New("execution creation must not occur during consumed replay")
}

type executionAtomicRepository struct {
	storage.AtomicRepository
	creates int
}

func (r *executionAtomicRepository) ConsumeApprovalAndCreateExecution(context.Context, storage.Scope, approvals.Approval, uint64, execution.Execution, audit.Record) (approvals.Approval, storage.CreateExecutionResult, error) {
	r.creates++
	return approvals.Approval{}, storage.CreateExecutionResult{}, errors.New("atomic execution creation must not occur during consumed replay")
}

type executionWalletRepository struct {
	storage.WalletBindingRepository
}

type consumedReplayFixture struct {
	service     *PersistedExecutionService
	ctx         context.Context
	intent      intents.Intent
	approval    approvals.Approval
	request     execution.Request
	policies    *executionPolicyRepository
	evaluations *executionEvaluationRepository
	executions  *executionRepository
	atomic      *executionAtomicRepository
}

func newConsumedReplayFixture(t *testing.T) consumedReplayFixture {
	t.Helper()
	intent, approval, request := executionReplayArtifacts(t, "intent_execution_1", "approval_execution_1", "policy_execution", 1)
	executionValue, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	policyRepository := &executionPolicyRepository{}
	evaluationRepository := &executionEvaluationRepository{}
	executionRepository := &executionRepository{execution: executionValue}
	atomicRepository := &executionAtomicRepository{}
	principal, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant_1", ActorID: "user_1", IdentityProvider: "issuer", ProviderSubject: "subject_1", ExpiresAt: executionServiceNow.Add(24 * time.Hour), Permissions: []auth.Permission{auth.PermissionPrepareExecution}})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentityWithSubject("user_1", "issuer", "subject_1", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := auth.NewTrustedRequest(principal, identity, auth.RequestMetadata{RequestID: "request_execution_1", TraceID: "trace_execution_1"})
	if err != nil {
		t.Fatal(err)
	}
	service := &PersistedExecutionService{
		Intents: executionIntentRepository{intent: intent}, Approvals: executionApprovalRepository{approval: approval},
		Policies: policyRepository, Evaluations: evaluationRepository, Executions: executionRepository,
		Atomic: atomicRepository, Wallets: executionWalletRepository{}, Authorizer: auth.NewPermissionAuthorizer(),
		Now: func() time.Time { return executionServiceNow.Add(2 * time.Minute) },
	}
	return consumedReplayFixture{service: service, ctx: auth.WithTrustedRequest(context.Background(), trusted), intent: intent, approval: approval, request: request, policies: policyRepository, evaluations: evaluationRepository, executions: executionRepository, atomic: atomicRepository}
}

func executionReplayArtifacts(t *testing.T, intentID, approvalID, policyID string, policyVersion uint64) (intents.Intent, approvals.Approval, execution.Request) {
	t.Helper()
	params := intents.Params{
		IntentID: intentID, Version: 1, ClientRequestID: "request_" + intentID, Nonce: "nonce_" + intentID, Type: intents.TypePayroll,
		Ownership: intents.Ownership{UserID: "user_1", IdentityProvider: "issuer", ProviderUserReference: "subject_1", WalletBindingID: "binding_1", WalletBindingVersion: 1, WalletID: "wallet_1", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet"},
		Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{Token: intents.Token{ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}, Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}}, Total: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}},
		Route:     intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferencePayroll, Version: intents.RouteVersionPayroll}, Constraints: intents.Constraints{Deadline: executionServiceNow.Add(5 * time.Minute), PolicyReference: policyID + ":1"}, CreatedAt: executionServiceNow.Add(-time.Hour), ExpiresAt: executionServiceNow.Add(10 * time.Minute),
	}
	intent, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, executionServiceNow.Add(-50*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusApprovalRequired, executionServiceNow.Add(-40*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.New(approvals.Params{ApprovalID: approvalID, Version: 1, ApprovalRequestID: "approval_request_" + approvalID, CreatedAt: executionServiceNow.Add(-30 * time.Minute), ExpiresAt: executionServiceNow.Add(4 * time.Minute)}, intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Approve(executionServiceNow.Add(-20 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Approve(approval, executionServiceNow.Add(-15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusReadyForExecution, executionServiceNow.Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.ReadyForExecutionConfirmation(executionServiceNow.Add(-5 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := intents.NewOperationIdentity(intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Consume(executionServiceNow, operation)
	if err != nil {
		t.Fatal(err)
	}
	result := policies.Result{PolicyID: policyID, PolicyVersion: policyVersion, IntentID: intent.IntentID(), IntentVersion: intent.Version(), IntentDigest: intent.Digest(), Stage: policies.EvaluationStageBeforeExecution, Decision: policies.DecisionAllow, EvaluatedAt: executionServiceNow.Add(-time.Second)}
	request, err := execution.NewRequest(intent, approval, result, executionServiceNow)
	if err != nil {
		t.Fatal(err)
	}
	return intent, approval, request
}

func TestPersistedExecutionServiceConsumedReplayReturnsExistingRequestWithoutWrites(t *testing.T) {
	fixture := newConsumedReplayFixture(t)
	got, err := fixture.service.PrepareExecution(fixture.ctx, fixture.intent.IntentID(), fixture.approval.ApprovalID(), fixture.request.PolicyID(), fixture.request.PolicyVersion())
	if err != nil {
		t.Fatal(err)
	}
	if got.RequestID() != fixture.request.RequestID() {
		t.Fatalf("request ID = %q, want %q", got.RequestID(), fixture.request.RequestID())
	}
	if fixture.policies.finds != 0 || fixture.evaluations.creates != 0 || fixture.executions.creates != 0 || fixture.atomic.creates != 0 {
		t.Fatalf("replay writes: policy finds=%d evaluations=%d executions=%d atomic=%d", fixture.policies.finds, fixture.evaluations.creates, fixture.executions.creates, fixture.atomic.creates)
	}
}

func TestPersistedExecutionServiceConsumedReplaySurvivesExpiredIntentAndApproval(t *testing.T) {
	fixture := newConsumedReplayFixture(t)
	fixture.service.Now = func() time.Time { return fixture.intent.ExpiresAt().Add(time.Hour) }
	got, err := fixture.service.PrepareExecution(fixture.ctx, fixture.intent.IntentID(), fixture.approval.ApprovalID(), fixture.request.PolicyID(), fixture.request.PolicyVersion())
	if err != nil || got.RequestID() != fixture.request.RequestID() {
		t.Fatalf("expired replay request=%q error=%v", got.RequestID(), err)
	}
}

func TestPersistedExecutionServiceConsumedApprovalRejectsDifferentOperation(t *testing.T) {
	fixture := newConsumedReplayFixture(t)
	otherIntent, _, _ := executionReplayArtifacts(t, "intent_execution_other", "approval_execution_other", fixture.request.PolicyID(), fixture.request.PolicyVersion())
	fixture.service.Intents = executionIntentRepository{intent: otherIntent}
	_, err := fixture.service.PrepareExecution(fixture.ctx, otherIntent.IntentID(), fixture.approval.ApprovalID(), fixture.request.PolicyID(), fixture.request.PolicyVersion())
	if !approvalHasCode(err, apperrors.CodeApprovalAlreadyConsumed) {
		t.Fatalf("different operation error = %v", err)
	}
	if fixture.executions.finds != 0 {
		t.Fatalf("execution lookup occurred for mismatched operation: %d", fixture.executions.finds)
	}
}

func TestPersistedExecutionServiceConsumedApprovalWithoutExecutionRequiresRecovery(t *testing.T) {
	fixture := newConsumedReplayFixture(t)
	fixture.executions.findErr = apperrors.New(apperrors.CodeExecutionNotFound, "Execution is not accessible.", false, true, true)
	_, err := fixture.service.PrepareExecution(fixture.ctx, fixture.intent.IntentID(), fixture.approval.ApprovalID(), fixture.request.PolicyID(), fixture.request.PolicyVersion())
	if !approvalHasCode(err, apperrors.CodeExecutionRecoverable) {
		t.Fatalf("missing execution error = %v", err)
	}
	if fixture.executions.creates != 0 || fixture.atomic.creates != 0 {
		t.Fatal("missing execution replay attempted to create an execution")
	}
}

func TestPersistedExecutionServiceConsumedReplayRejectsMismatchedRequestIdentity(t *testing.T) {
	fixture := newConsumedReplayFixture(t)
	_, otherApproval, otherRequest := executionReplayArtifacts(t, fixture.intent.IntentID(), "approval_execution_other", fixture.request.PolicyID(), fixture.request.PolicyVersion())
	if otherApproval.OperationKey() != fixture.approval.OperationKey() {
		t.Fatal("test fixture operations differ")
	}
	otherExecution, err := execution.New(otherRequest)
	if err != nil {
		t.Fatal(err)
	}
	fixture.executions.execution = otherExecution
	_, err = fixture.service.PrepareExecution(fixture.ctx, fixture.intent.IntentID(), fixture.approval.ApprovalID(), fixture.request.PolicyID(), fixture.request.PolicyVersion())
	if !approvalHasCode(err, apperrors.CodeExecutionConflict) {
		t.Fatalf("mismatched execution error = %v", err)
	}
}
