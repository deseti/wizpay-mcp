package postgres

import (
	"context"
	stderrors "errors"
	"sync"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/audit"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

func TestRepositoryRoundTripsCurrentDomainContracts(t *testing.T) {
	f := createBaseFixture(t, true)
	ctx := context.Background()
	identity, err := integrationStore.FindIdentityByID(ctx, f.scope, f.identity.UserID())
	if err != nil || identity.Status() != f.identity.Status() {
		t.Fatalf("identity round trip = %v, %v", identity, err)
	}
	binding, err := integrationStore.FindBindingByID(ctx, f.scope, f.binding.BindingID())
	if err != nil || binding.Version() != f.binding.Version() {
		t.Fatalf("binding round trip: %v", err)
	}
	intent, err := integrationStore.FindIntentByID(ctx, f.scope, f.intent.IntentID())
	if err != nil || intent.Digest() != f.intent.Digest() || intent.Status() != intents.StatusApproved {
		t.Fatalf("intent round trip: %v", err)
	}
	approval, err := integrationStore.FindApprovalByID(ctx, f.scope, f.approval.ApprovalID())
	if err != nil || approval.Status() != approvals.StatusConsumed || approval.IntentDigest() != intent.Digest() {
		t.Fatalf("approval round trip: %v", err)
	}
	policy, err := integrationStore.FindPolicyByID(ctx, f.scope, f.policy.PolicyID(), f.policy.Version())
	if err != nil || policy.Reference() != f.policy.Reference() {
		t.Fatalf("policy round trip: %v", err)
	}
	evaluation, err := integrationStore.FindPolicyEvaluation(ctx, f.scope, execution.PolicyEvaluationKey(f.policyResult))
	if err != nil || evaluation.Decision != f.policyResult.Decision {
		t.Fatalf("evaluation round trip: %v", err)
	}
	if replay, err := integrationStore.CreatePolicyEvaluation(ctx, f.scope, f.policyResult); err != nil || replay.Decision != f.policyResult.Decision {
		t.Fatalf("evaluation replay: %v", err)
	}
	executionValue, err := integrationStore.FindExecutionByID(ctx, f.scope, f.execution.ExecutionID())
	if err != nil || executionValue.Request().RequestKey() != f.execution.Request().RequestKey() {
		t.Fatalf("execution round trip: %v", err)
	}
}

func TestTenantIsolationAndCrossTenantForeignKeys(t *testing.T) {
	f := createBaseFixture(t, true)
	otherTenant := unique("tenant")
	otherUser := unique("user")
	otherScope, err := storage.NewScope(otherTenant, otherUser, unique("request"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateTenant(context.Background(), storage.Tenant{TenantID: otherTenant, CreatedAt: fixtureNow}); err != nil {
		t.Fatal(err)
	}
	for name, operation := range map[string]func() error{"identity": func() error {
		_, err := integrationStore.FindIdentityByID(context.Background(), otherScope, f.identity.UserID())
		return err
	}, "wallet": func() error {
		_, err := integrationStore.FindBindingByID(context.Background(), otherScope, f.binding.BindingID())
		return err
	}, "intent": func() error {
		_, err := integrationStore.FindIntentByID(context.Background(), otherScope, f.intent.IntentID())
		return err
	}, "approval": func() error {
		_, err := integrationStore.FindApprovalByID(context.Background(), otherScope, f.approval.ApprovalID())
		return err
	}, "policy": func() error {
		_, err := integrationStore.FindPolicyByID(context.Background(), otherScope, f.policy.PolicyID(), f.policy.Version())
		return err
	}, "execution": func() error {
		_, err := integrationStore.FindExecutionByID(context.Background(), otherScope, f.execution.ExecutionID())
		return err
	}, "policy_evaluation": func() error {
		_, err := integrationStore.FindPolicyEvaluation(context.Background(), otherScope, execution.PolicyEvaluationKey(f.policyResult))
		return err
	}} {
		t.Run(name, func(t *testing.T) {
			err := operation()
			if err == nil {
				t.Fatal("cross-tenant read succeeded")
			}
			public := apperrors.ToPublic(err)
			if public.Code == apperrors.CodeInternalError {
				t.Fatalf("unsafe cross-tenant error: %#v", public)
			}
		})
	}
	crossBinding, err := wallet.NewBinding(wallet.BindingParams{BindingID: unique("binding"), Version: 1, UserID: f.identity.UserID(), Provider: f.identity.Provider(), ProviderUserReference: unique("provider-user"), WalletID: unique("wallet"), Address: unique("address"), ChainID: "5042002", Network: "test-network", Status: wallet.BindingStatusPending, CreatedAt: fixtureNow})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateBinding(context.Background(), otherScope, crossBinding); err == nil {
		t.Fatal("cross-tenant foreign key accepted")
	}
	for name, operation := range map[string]func() error{
		"wallet": func() error {
			_, err := integrationStore.UpdateBinding(context.Background(), otherScope, f.binding, f.binding.Version())
			return err
		},
		"intent": func() error {
			_, err := integrationStore.UpdateIntent(context.Background(), otherScope, f.intent, f.intent.LifecycleRevision())
			return err
		},
		"approval": func() error {
			_, err := integrationStore.UpdateApproval(context.Background(), otherScope, f.approval, f.approval.LifecycleRevision())
			return err
		},
		"policy": func() error {
			_, err := integrationStore.UpdatePolicy(context.Background(), otherScope, f.policy, f.policy.LifecycleRevision())
			return err
		},
		"execution": func() error {
			_, err := integrationStore.UpdateExecution(context.Background(), otherScope, f.execution, f.execution.Revision())
			return err
		},
	} {
		t.Run("mutate_"+name, func(t *testing.T) {
			if err := operation(); err == nil {
				t.Fatal("cross-tenant mutation succeeded")
			}
		})
	}
}

func TestOptimisticConcurrencyFailsClosed(t *testing.T) {
	f := createBaseFixture(t, true)
	revoked, err := f.binding.Transition(wallet.BindingStatusRevoked, fixtureNow.Add(time.Minute), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.UpdateBinding(context.Background(), f.scope, revoked, f.binding.Version()); err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.UpdateBinding(context.Background(), f.scope, revoked, f.binding.Version()); apperrors.ToPublic(err).Code != apperrors.CodeExecutionConflict {
		t.Fatalf("stale wallet update error = %v", err)
	}
	authorized, err := f.execution.Transition(execution.StatusAuthorized, fixtureNow.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	event := auditRecord(f.scope, unique("event"), audit.EventExecutionAuthorized, "execution", authorized.ExecutionID(), authorized.UpdatedAt())
	if _, err = integrationStore.UpdateExecutionWithAudit(context.Background(), f.scope, authorized, f.execution.Revision(), event); err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.UpdateExecution(context.Background(), f.scope, authorized, f.execution.Revision()); apperrors.ToPublic(err).Code != apperrors.CodeExecutionConflict {
		t.Fatalf("stale execution update error = %v", err)
	}
}

func TestConcurrentExecutionPreparationCreatesOneLogicalExecution(t *testing.T) {
	f := createBaseFixture(t, false)
	operation, err := intents.NewOperationIdentity(f.intent)
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := f.approval.Consume(fixtureNow.Add(6*time.Second), operation)
	if err != nil {
		t.Fatal(err)
	}
	request, err := execution.NewRequest(f.intent, consumed, f.policyResult, fixtureNow.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	value, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	const callers = 6
	results := make(chan storage.CreateExecutionResult, callers)
	errorsChannel := make(chan error, callers)
	var wait sync.WaitGroup
	for i := 0; i < callers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			record := auditRecord(f.scope, unique("event"), audit.EventExecutionCreated, "execution", value.ExecutionID(), fixtureNow.Add(time.Duration(index+6)*time.Second))
			_, result, err := integrationStore.ConsumeApprovalAndCreateExecution(context.Background(), f.scope, consumed, f.approval.LifecycleRevision(), value, record)
			results <- result
			errorsChannel <- err
		}(i)
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	created := 0
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent preparation: %v", err)
		}
	}
	for result := range results {
		if result.Created {
			created++
		}
		if result.Execution.ExecutionID() != value.ExecutionID() {
			t.Fatal("execution identity changed")
		}
	}
	if created != 1 {
		t.Fatalf("created count = %d, want 1", created)
	}
	var count int
	if err := integrationPool.QueryRow(context.Background(), `SELECT count(*) FROM executions WHERE tenant_id=$1 AND execution_id=$2`, f.scope.TenantID(), value.ExecutionID()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("execution rows = %d", count)
	}
}

func TestAuditAppendOnlyRedactionAndTransactionalRollback(t *testing.T) {
	f := createBaseFixture(t, false)
	record := auditRecord(f.scope, unique("event"), audit.EventIntentCancelled, "intent", f.intent.IntentID(), fixtureNow.Add(time.Minute))
	spoofed := record
	spoofed.Event.EventID = unique("event")
	spoofed.ActorID = unique("other-actor")
	if err := integrationStore.AppendAudit(context.Background(), f.scope, spoofed); apperrors.ToPublic(err).Code != apperrors.CodeAuthorizationRequired {
		t.Fatalf("spoofed audit attribution error = %v", err)
	}
	if err := integrationStore.AppendAudit(context.Background(), f.scope, record); err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(context.Background(), `UPDATE audit_records SET new_state='tampered' WHERE tenant_id=$1 AND event_id=$2`, f.scope.TenantID(), record.Event.EventID); err == nil {
		t.Fatal("audit update succeeded")
	}
	if _, err := integrationPool.Exec(context.Background(), `DELETE FROM audit_records WHERE tenant_id=$1 AND event_id=$2`, f.scope.TenantID(), record.Event.EventID); err == nil {
		t.Fatal("audit delete succeeded")
	}
	if _, err := integrationPool.Exec(context.Background(), `UPDATE intents SET financial='{}'::jsonb WHERE tenant_id=$1 AND intent_id=$2`, f.scope.TenantID(), f.intent.IntentID()); err == nil {
		t.Fatal("immutable intent material update succeeded")
	}
	unsafe := record
	unsafe.Event.EventID = unique("event")
	unsafe.SafeReasonCode = "private_key=unsafe"
	if err := integrationStore.AppendAudit(context.Background(), f.scope, unsafe); err == nil {
		t.Fatal("forbidden audit material accepted")
	}
	newIntent := newSiblingIntent(t, f, unique("intent"), unique("client-request"))
	duplicateAudit := record
	_, err := integrationStore.CreateIntentWithAudit(context.Background(), f.scope, newIntent, duplicateAudit)
	if err == nil {
		t.Fatal("duplicate audit did not fail transaction")
	}
	if _, err := integrationStore.FindIntentByID(context.Background(), f.scope, newIntent.IntentID()); apperrors.ToPublic(err).Code != apperrors.CodeIntentNotFound {
		t.Fatalf("partial intent persisted: %v", err)
	}
}

func TestActorScopedWritesRejectAnotherOwner(t *testing.T) {
	f := createBaseFixture(t, false)
	otherScope, err := storage.NewScope(f.scope.TenantID(), unique("other-user"), unique("request"), unique("trace"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateBinding(context.Background(), otherScope, f.binding); apperrors.ToPublic(err).Code != apperrors.CodeAuthorizationRequired {
		t.Fatalf("cross-owner wallet write error = %v", err)
	}
	record := auditRecord(otherScope, unique("event"), audit.EventIntentCreated, "intent", f.intent.IntentID(), fixtureNow.Add(time.Minute))
	if _, err = integrationStore.CreateIntentWithAudit(context.Background(), otherScope, f.intent, record); apperrors.ToPublic(err).Code != apperrors.CodeAuthorizationRequired {
		t.Fatalf("cross-owner intent write error = %v", err)
	}
}

func TestVerificationEvidenceAndVerifiedTransitionAreAtomic(t *testing.T) {
	f := createBaseFixture(t, true)
	value := f.execution
	var err error
	for index, status := range []execution.Status{execution.StatusAuthorized, execution.StatusQueued, execution.StatusExecuting, execution.StatusSubmitted, execution.StatusConfirming, execution.StatusConfirmed} {
		next, transitionErr := value.Transition(status, fixtureNow.Add(time.Duration(7+index+1)*time.Second))
		if transitionErr != nil {
			t.Fatal(transitionErr)
		}
		if _, err = integrationStore.UpdateExecution(context.Background(), f.scope, next, value.Revision()); err != nil {
			t.Fatal(err)
		}
		value = next
	}
	verified, err := value.Transition(execution.StatusVerified, fixtureNow.Add(15*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := execution.NewResult(execution.ResultParams{ExecutionID: value.ExecutionID(), ExecutionVersion: verified.Revision(), Status: execution.StatusVerified, AdapterReference: "safe-verification-reference", ObservedAt: fixtureNow.Add(15 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	record := auditRecord(f.scope, unique("event"), audit.EventExecutionVerified, "execution", value.ExecutionID(), fixtureNow.Add(15*time.Second))
	if _, err = integrationStore.AppendEvidenceAndVerify(context.Background(), f.scope, evidence, verified, value.Revision(), record); err != nil {
		t.Fatal(err)
	}
	evidenceRows, err := integrationStore.FindVerificationEvidence(context.Background(), f.scope, value.ExecutionID())
	if err != nil || len(evidenceRows) != 1 || evidenceRows[0].Status() != execution.StatusVerified {
		t.Fatalf("evidence = %v, %v", evidenceRows, err)
	}
	stored, err := integrationStore.FindExecutionByID(context.Background(), f.scope, value.ExecutionID())
	if err != nil || stored.Status() != execution.StatusVerified {
		t.Fatalf("verified execution = %s, %v", stored.Status(), err)
	}
}

func TestContextCancellationMapsSafely(t *testing.T) {
	f := createBaseFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := integrationStore.FindIntentByID(ctx, f.scope, f.intent.IntentID())
	if err == nil {
		t.Fatal("cancelled query succeeded")
	}
	public := apperrors.ToPublic(err)
	if public.Code != apperrors.CodeInternalError || !public.Retryable {
		t.Fatalf("cancelled error = %#v", public)
	}
	if !stderrors.Is(err, context.Canceled) && !stderrors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation cause not retained: %v", err)
	}
}

func newSiblingIntent(t *testing.T, f fixture, intentID, clientRequestID string) intents.Intent {
	t.Helper()
	financial := f.intent.Financial()
	params := intents.Params{IntentID: intentID, Version: 1, ClientRequestID: clientRequestID, Nonce: unique("nonce"), Type: f.intent.Type(), Ownership: f.intent.Ownership(), Financial: financial, Route: f.intent.Route(), Constraints: f.intent.Constraints(), CreatedAt: f.intent.CreatedAt(), ExpiresAt: f.intent.ExpiresAt()}
	value, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	value, err = value.Transition(intents.StatusCreated, fixtureNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return value
}
