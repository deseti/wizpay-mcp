package runtime

import (
	"context"
	stderrors "errors"
	"fmt"
	"sync"
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
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

var testNow = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
var runtimeNow = testNow.Add(10 * time.Minute)

type fakeAdapter struct {
	mu          sync.Mutex
	executeCall int
	statusCall  int
	execute     execution.Result
	status      execution.Result
	executeErr  error
	statusErr   error
}

func (a *fakeAdapter) Execute(context.Context, execution.Request) (execution.Result, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.executeCall++
	return a.execute, a.executeErr
}
func (a *fakeAdapter) GetStatus(context.Context, string) (execution.Result, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statusCall++
	return a.status, a.statusErr
}

type fakeVerifier struct {
	calls  int
	result VerificationResult
	err    error
}

func (v *fakeVerifier) Verify(context.Context, execution.Execution, string) (VerificationResult, error) {
	v.calls++
	return v.result, v.err
}

type memoryRepository struct {
	mu                sync.Mutex
	value             execution.Execution
	created           bool
	claim             storage.ExecutionClaim
	evidence          []execution.Result
	submissionStarted bool
}

func (r *memoryRepository) FindExecutionByID(context.Context, storage.Scope, string) (execution.Execution, error) {
	return r.value, nil
}
func (r *memoryRepository) FindExecutionByRequestKey(context.Context, storage.Scope, string, uint64) (execution.Execution, error) {
	return r.value, nil
}
func (r *memoryRepository) FindExecutionByOperationKey(context.Context, storage.Scope, string, uint64) (execution.Execution, error) {
	return r.value, nil
}
func (r *memoryRepository) CreateExecution(_ context.Context, _ storage.Scope, value execution.Execution) (storage.CreateExecutionResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.created {
		if r.value.Request().RequestKey() != value.Request().RequestKey() {
			return storage.CreateExecutionResult{}, apperrors.New(apperrors.CodeExecutionConflict, "conflict", false, true, true)
		}
		return storage.CreateExecutionResult{Execution: r.value}, nil
	}
	r.value, r.created = value, true
	return storage.CreateExecutionResult{Execution: value, Created: true}, nil
}
func (r *memoryRepository) UpdateExecution(context.Context, storage.Scope, execution.Execution, uint64) (execution.Execution, error) {
	panic("unused")
}
func (r *memoryRepository) FindVerificationEvidence(context.Context, storage.Scope, string) ([]execution.Result, error) {
	return append([]execution.Result(nil), r.evidence...), nil
}
func (r *memoryRepository) AppendVerificationEvidence(_ context.Context, _ storage.Scope, value execution.Result) error {
	r.evidence = append(r.evidence, value)
	return nil
}
func (r *memoryRepository) ClaimExecutionWork(_ context.Context, scope storage.Scope, _ string, owner string, now time.Time, duration time.Duration) (storage.ExecutionClaim, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claim.LeaseOwner != "" {
		return storage.ExecutionClaim{}, false, nil
	}
	r.claim = storage.ExecutionClaim{Scope: scope, Execution: r.value, LeaseOwner: owner, FencingToken: 1, LeaseExpiresAt: now.Add(duration), SubmissionStarted: r.submissionStarted}
	return r.claim, true, nil
}
func (r *memoryRepository) ClaimNextExecutionWork(context.Context, string, time.Time, time.Duration) (storage.ExecutionClaim, bool, error) {
	panic("unused")
}
func (r *memoryRepository) MarkSubmissionStarted(_ context.Context, claim storage.ExecutionClaim, _ time.Time) (storage.ExecutionClaim, bool, error) {
	if r.submissionStarted {
		return claim, false, nil
	}
	r.submissionStarted = true
	claim.SubmissionStarted = true
	r.claim = claim
	return claim, true, nil
}
func (r *memoryRepository) ResetSubmissionStarted(_ context.Context, claim storage.ExecutionClaim, _ time.Time) (storage.ExecutionClaim, bool, error) {
	if !r.submissionStarted {
		return claim, false, nil
	}
	r.submissionStarted = false
	claim.SubmissionStarted = false
	r.claim = claim
	return claim, true, nil
}
func (r *memoryRepository) UpdateClaimedExecution(_ context.Context, claim storage.ExecutionClaim, value execution.Execution, expected uint64, _ audit.Record, _ time.Time) (execution.Execution, error) {
	if claim.FencingToken != r.claim.FencingToken || r.value.Revision() != expected {
		return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "stale", true, false, false)
	}
	r.value = value
	r.claim.Execution = value
	return value, nil
}
func (r *memoryRepository) PersistClaimedObservation(ctx context.Context, claim storage.ExecutionClaim, evidence execution.Result, value execution.Execution, expected uint64, record audit.Record, now time.Time) (execution.Execution, error) {
	stored, err := r.UpdateClaimedExecution(ctx, claim, value, expected, record, now)
	if err != nil {
		return execution.Execution{}, err
	}
	r.evidence = append(r.evidence, evidence)
	return stored, nil
}
func (r *memoryRepository) PersistClaimedEvidence(_ context.Context, claim storage.ExecutionClaim, evidence execution.Result, value execution.Execution, expected uint64, _ audit.Record, _ time.Time) (execution.Execution, error) {
	if claim.FencingToken != r.claim.FencingToken || r.value.Revision() != expected {
		return execution.Execution{}, apperrors.New(apperrors.CodeExecutionConflict, "stale", true, false, false)
	}
	r.value = value
	r.claim.Execution = value
	r.evidence = append(r.evidence, evidence)
	return value, nil
}
func (r *memoryRepository) ReleaseExecutionWork(context.Context, storage.ExecutionClaim, time.Time) (bool, error) {
	r.claim = storage.ExecutionClaim{}
	return true, nil
}

func TestStartReplayReturnsSameExecution(t *testing.T) {
	repository := &memoryRepository{}
	service := testService(t, repository, &fakeAdapter{}, &fakeVerifier{})
	request := testRequest(t)
	first, err := service.Start(context.Background(), testScope(t), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Start(context.Background(), testScope(t), request)
	if err != nil || first.ExecutionID() != second.ExecutionID() || first.Revision() != second.Revision() {
		t.Fatalf("replay = (%s,%d,%v)", second.ExecutionID(), second.Revision(), err)
	}
}

func TestProcessExecutesOnceThenVerifiesBeforeCompletion(t *testing.T) {
	request := testRequest(t)
	submitted := mustResult(t, request, execution.StatusSubmitted, "adapter-ref", "", testNow.Add(time.Minute))
	repository := &memoryRepository{}
	adapter := &fakeAdapter{execute: submitted}
	verifier := &fakeVerifier{result: VerificationResult{Outcome: VerificationVerified, Reference: "verified-ref", ObservedAt: testNow.Add(2 * time.Minute)}}
	service := testService(t, repository, adapter, verifier)
	if _, err := service.Start(context.Background(), testScope(t), request); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8 && !repository.value.Terminal(); i++ {
		if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
			t.Fatal(err)
		}
	}
	if repository.value.Status() != execution.StatusCompleted || adapter.executeCall != 1 || verifier.calls != 1 {
		t.Fatalf("status=%s execute=%d verify=%d", repository.value.Status(), adapter.executeCall, verifier.calls)
	}
	if len(repository.evidence) < 2 || repository.evidence[len(repository.evidence)-1].Status() != execution.StatusVerified {
		t.Fatalf("evidence = %+v", repository.evidence)
	}
}

func TestPersistedSubmissionMarkerReconcilesWithoutExecute(t *testing.T) {
	request := testRequest(t)
	repository := &memoryRepository{submissionStarted: true}
	value, _ := execution.New(request)
	value = advance(t, value, execution.StatusAuthorized, 1)
	value = advance(t, value, execution.StatusQueued, 2)
	value = advance(t, value, execution.StatusExecuting, 3)
	repository.value, repository.created = value, true
	adapter := &fakeAdapter{status: mustResult(t, request, execution.StatusSubmitted, "reconciled-ref", "", testNow.Add(time.Minute))}
	service := testService(t, repository, adapter, &fakeVerifier{})
	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	if adapter.executeCall != 0 || adapter.statusCall != 1 || repository.value.Status() != execution.StatusSubmitted {
		t.Fatalf("execute=%d status=%d lifecycle=%s", adapter.executeCall, adapter.statusCall, repository.value.Status())
	}
}

func TestPersistedSubmissionMarkerTransientReconciliationRemainsReconciliationOnly(t *testing.T) {
	request := testRequest(t)
	repository := executingRepository(t, request, true)
	adapter := &fakeAdapter{statusErr: NewAdapterError(AdapterFailureTransient, "ADAPTER_TEMPORARY")}
	service := testService(t, repository, adapter, &fakeVerifier{})

	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	if !repository.submissionStarted || repository.value.Status() != execution.StatusFailed {
		t.Fatalf("after transient reconciliation: marker=%t status=%s", repository.submissionStarted, repository.value.Status())
	}

	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	adapter.statusErr = nil
	adapter.status = mustResult(t, request, execution.StatusSubmitted, "reconciled-ref", "", runtimeNow.Add(time.Minute))
	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	if !repository.submissionStarted || adapter.executeCall != 0 || adapter.statusCall != 2 || repository.value.Status() != execution.StatusSubmitted {
		t.Fatalf("after retry: marker=%t execute=%d status calls=%d lifecycle=%s", repository.submissionStarted, adapter.executeCall, adapter.statusCall, repository.value.Status())
	}
}

func TestPersistedSubmissionMarkerUnclassifiedReconciliationRemainsReconciliationOnly(t *testing.T) {
	request := testRequest(t)
	repository := executingRepository(t, request, true)
	adapter := &fakeAdapter{statusErr: stderrors.New("unclassified provider detail")}
	service := testService(t, repository, adapter, &fakeVerifier{})

	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	if !repository.submissionStarted || repository.value.Status() != execution.StatusRecoveryRequired {
		t.Fatalf("after ambiguous reconciliation: marker=%t status=%s", repository.submissionStarted, repository.value.Status())
	}

	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	adapter.statusErr = nil
	adapter.status = mustResult(t, request, execution.StatusSubmitted, "reconciled-ref", "", runtimeNow.Add(time.Minute))
	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	if !repository.submissionStarted || adapter.executeCall != 0 || adapter.statusCall != 2 || repository.value.Status() != execution.StatusSubmitted {
		t.Fatalf("after retry: marker=%t execute=%d status calls=%d lifecycle=%s", repository.submissionStarted, adapter.executeCall, adapter.statusCall, repository.value.Status())
	}
}

func TestAdapterFailuresAreClassifiedDeterministically(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want execution.Status
	}{
		{"transient before submission", NewAdapterError(AdapterFailureTransient, "ADAPTER_TEMPORARY"), execution.StatusFailed},
		{"permanent", NewAdapterError(AdapterFailurePermanent, "ADAPTER_REJECTED"), execution.StatusFailed},
		{"ambiguous", stderrors.New("unclassified provider detail"), execution.StatusRecoveryRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := testRequest(t)
			repository := &memoryRepository{}
			service := testService(t, repository, &fakeAdapter{executeErr: test.err}, &fakeVerifier{})
			if _, err := service.Start(context.Background(), testScope(t), request); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 4 && repository.value.Status() != test.want; i++ {
				_, _ = service.Process(context.Background(), testScope(t), request.ExecutionID())
			}
			if repository.value.Status() != test.want {
				t.Fatalf("status = %s, want %s", repository.value.Status(), test.want)
			}
			if test.err == nil || test.name == "ambiguous" {
				return
			}
			failure, ok := repository.value.Failure()
			if !ok || (test.name == "permanent" && failure.Eligibility != execution.RecoveryTerminal) || (test.name != "permanent" && failure.Eligibility != execution.RecoveryRecoverable) {
				t.Fatalf("failure = %+v", failure)
			}
		})
	}
}

func TestTransientExecuteFailureRemainsReconciliationOnly(t *testing.T) {
	request := testRequest(t)
	repository := &memoryRepository{}
	adapter := &fakeAdapter{executeErr: NewAdapterError(AdapterFailureTransient, "ADAPTER_TEMPORARY")}
	service := testService(t, repository, adapter, &fakeVerifier{})
	if _, err := service.Start(context.Background(), testScope(t), request); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
			t.Fatal(err)
		}
	}
	if repository.value.Status() != execution.StatusExecuting || adapter.executeCall != 1 {
		t.Fatalf("recovery resumed incorrectly: status=%s execute=%d", repository.value.Status(), adapter.executeCall)
	}
	if !repository.submissionStarted {
		t.Fatal("submission marker was reset after transient Execute error")
	}
	adapter.executeErr = nil
	adapter.status = mustResult(t, request, execution.StatusSubmitted, "reconciled-ref", "", runtimeNow.Add(time.Minute))
	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	if repository.value.Status() != execution.StatusSubmitted || adapter.executeCall != 1 || adapter.statusCall != 1 {
		t.Fatalf("ambiguous submission retried incorrectly: status=%s execute=%d status calls=%d", repository.value.Status(), adapter.executeCall, adapter.statusCall)
	}
}

func TestVerificationPendingAndTransientNeverComplete(t *testing.T) {
	for _, result := range []struct {
		verification VerificationResult
		err          error
	}{
		{verification: VerificationResult{Outcome: VerificationPending, Reference: "pending-ref", ObservedAt: testNow.Add(time.Minute)}},
		{err: NewVerificationError("VERIFIER_UNAVAILABLE")},
	} {
		request := testRequest(t)
		repository := confirmingRepository(t, request)
		service := testService(t, repository, &fakeAdapter{}, &fakeVerifier{result: result.verification, err: result.err})
		if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil && result.err == nil {
			t.Fatal(err)
		}
		if repository.value.Status() == execution.StatusVerified || repository.value.Status() == execution.StatusCompleted {
			t.Fatalf("verification incorrectly completed: %s", repository.value.Status())
		}
	}
}

func TestConfirmedRestartRequiresVerifierAndAtomicEvidence(t *testing.T) {
	request := testRequest(t)
	repository := confirmingRepository(t, request)
	confirmed := advance(t, repository.value, execution.StatusConfirmed, 6)
	repository.value = confirmed
	repository.claim.Execution = confirmed
	verifier := &fakeVerifier{result: VerificationResult{Outcome: VerificationVerified, Reference: "restart-verified-ref", ObservedAt: runtimeNow}}
	service := testService(t, repository, &fakeAdapter{}, verifier)
	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); err != nil {
		t.Fatal(err)
	}
	if repository.value.Status() != execution.StatusVerified || verifier.calls != 1 {
		t.Fatalf("status=%s verifier calls=%d", repository.value.Status(), verifier.calls)
	}
	if len(repository.evidence) < 2 || repository.evidence[len(repository.evidence)-1].Status() != execution.StatusVerified {
		t.Fatalf("verified evidence missing: %+v", repository.evidence)
	}
}

func TestVerifiedStateWithoutMatchingEvidenceCannotComplete(t *testing.T) {
	request := testRequest(t)
	repository := confirmingRepository(t, request)
	confirmed := advance(t, repository.value, execution.StatusConfirmed, 6)
	verified := advance(t, confirmed, execution.StatusVerified, 7)
	repository.value = verified
	repository.evidence = nil
	service := testService(t, repository, &fakeAdapter{}, &fakeVerifier{})
	if _, err := service.Process(context.Background(), testScope(t), request.ExecutionID()); apperrors.ToPublic(err).Code != apperrors.CodeExecutionRecoverable {
		t.Fatalf("missing evidence error = %v", err)
	}
	if repository.value.Status() != execution.StatusVerified {
		t.Fatalf("status = %s", repository.value.Status())
	}
}

func testService(t *testing.T, repository *memoryRepository, adapter execution.Adapter, verifier Verifier) *Service {
	t.Helper()
	service, err := NewService(repository, repository, repository, adapter, verifier, Config{WorkerID: "worker-test", LeaseDuration: time.Minute, RetryInterval: time.Second}, func() time.Time { return runtimeNow })
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func confirmingRepository(t *testing.T, request execution.Request) *memoryRepository {
	t.Helper()
	value, _ := execution.New(request)
	for i, status := range []execution.Status{execution.StatusAuthorized, execution.StatusQueued, execution.StatusExecuting, execution.StatusSubmitted, execution.StatusConfirming} {
		value = advance(t, value, status, i+1)
	}
	return &memoryRepository{value: value, created: true, submissionStarted: true, evidence: []execution.Result{mustEvidence(t, value, "adapter-ref")}}
}

func executingRepository(t *testing.T, request execution.Request, submissionStarted bool) *memoryRepository {
	t.Helper()
	value, _ := execution.New(request)
	for i, status := range []execution.Status{execution.StatusAuthorized, execution.StatusQueued, execution.StatusExecuting} {
		value = advance(t, value, status, i+1)
	}
	return &memoryRepository{value: value, created: true, submissionStarted: submissionStarted}
}

func mustEvidence(t *testing.T, value execution.Execution, reference string) execution.Result {
	t.Helper()
	result, err := execution.NewResult(execution.ResultParams{ExecutionID: value.ExecutionID(), ExecutionVersion: value.Revision(), Status: value.Status(), AdapterReference: reference, ObservedAt: value.UpdatedAt()})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func mustResult(t *testing.T, request execution.Request, status execution.Status, reference, code string, at time.Time) execution.Result {
	t.Helper()
	result, err := execution.NewResult(execution.ResultParams{ExecutionID: request.ExecutionID(), ExecutionVersion: request.Version(), Status: status, AdapterReference: reference, ErrorCode: code, ObservedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func advance(t *testing.T, value execution.Execution, status execution.Status, seconds int) execution.Execution {
	t.Helper()
	next, err := value.Transition(status, testNow.Add(time.Duration(8+seconds)*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func testScope(t *testing.T) storage.Scope {
	t.Helper()
	scope, err := storage.NewScope("tenant-test", "user-test", "runtime-test", "")
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func testRequest(t *testing.T) execution.Request {
	t.Helper()
	policy, err := policies.NewDraft(policies.Params{PolicyID: "policy-test", Version: 1, Name: "Runtime test", Scope: policies.Scope{UserID: "user-test", WalletBindingID: "binding-test"}, Rules: []policies.Rule{{RuleID: "operations", OnViolation: policies.DecisionDeny, OperationAllowlist: &policies.OperationAllowlistRule{Allowed: []intents.Type{intents.TypePayroll}}}}, CreatedAt: testNow.Add(-time.Hour), ValidFrom: testNow.Add(-30 * time.Minute), ExpiresAt: testNow.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	policy, _ = policy.Transition(policies.StatusActive, policy.ValidFrom())
	identity, _ := auth.NewIdentity("user-test", "test-provider", auth.IdentityStatusActive)
	identityContext, _ := auth.NewIdentityContext(identity, auth.RequestMetadata{RequestID: "request-test"})
	binding, err := wallet.NewBinding(wallet.BindingParams{BindingID: "binding-test", Version: 1, UserID: "user-test", Provider: "test-provider", ProviderUserReference: "provider-user-test", WalletID: "wallet-test", Address: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "test-network", Status: wallet.BindingStatusActive, VerificationReference: "binding-verification", CreatedAt: testNow.Add(-2 * time.Hour), VerifiedAt: testNow.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	token := intents.Token{ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}
	intent, err := intents.NewDraft(intents.Params{IntentID: "intent-test", Version: 1, ClientRequestID: "client-test", Nonce: "nonce-test", Type: intents.TypePayroll, Ownership: intents.Ownership{UserID: "user-test", IdentityProvider: "test-provider", ProviderUserReference: "provider-user-test", WalletBindingID: "binding-test", WalletBindingVersion: 1, WalletID: "wallet-test", WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network()}, Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{Token: token, Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}}, Total: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}}, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: "route-test", Version: 1}, Constraints: intents.Constraints{Deadline: testNow.Add(50 * time.Minute), PolicyReference: policy.Reference()}, CreatedAt: testNow, ExpiresAt: testNow.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	intent, _ = intent.Transition(intents.StatusCreated, testNow.Add(time.Second))
	intent, _ = intent.Transition(intents.StatusApprovalRequired, testNow.Add(2*time.Second))
	approval, _ := approvals.New(approvals.Params{ApprovalID: "approval-test", Version: 1, ApprovalRequestID: "approval-request-test", CreatedAt: testNow.Add(3 * time.Second), ExpiresAt: testNow.Add(40 * time.Minute)}, intent)
	approval, _ = approval.Approve(testNow.Add(4 * time.Second))
	intent, _ = intent.Approve(approval, testNow.Add(5*time.Second))
	policyResult, err := policies.Evaluate(policy, intent, identityContext, binding, testNow.Add(6*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	operation, _ := intents.NewOperationIdentity(intent)
	approval, _ = approval.Consume(testNow.Add(7*time.Second), operation)
	request, err := execution.NewRequest(intent, approval, policyResult, testNow.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return request
}

var _ execution.Adapter = (*fakeAdapter)(nil)
var _ Verifier = (*fakeVerifier)(nil)
var _ storage.ExecutionRepository = (*memoryRepository)(nil)
var _ storage.ExecutionRuntimeRepository = (*memoryRepository)(nil)
var _ storage.VerificationEvidenceRepository = (*memoryRepository)(nil)
var _ = fmt.Sprintf
