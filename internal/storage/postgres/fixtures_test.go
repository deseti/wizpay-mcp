package postgres

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

var fixtureSequence atomic.Uint64
var fixtureNow = time.Date(2026, 8, 3, 2, 0, 0, 0, time.UTC)

type fixture struct {
	scope        storage.Scope
	identity     auth.Identity
	binding      wallet.Binding
	intent       intents.Intent
	approval     approvals.Approval
	policy       policies.Policy
	policyResult policies.Result
	execution    execution.Execution
}

func unique(prefix string) string { return fmt.Sprintf("%s_%d", prefix, fixtureSequence.Add(1)) }
func auditRecord(scope storage.Scope, eventID string, eventType audit.EventType, resourceType, resourceID string, at time.Time) audit.Record {
	return audit.Record{Event: audit.Event{EventID: eventID, Type: eventType, OccurredAt: at}, ActorType: "user", ActorID: scope.ActorID(), RequestID: scope.RequestID(), TraceID: scope.TraceID(), ResourceType: resourceType, ResourceID: resourceID, SourceComponent: "postgres_integration_test"}
}
func createBaseFixture(t *testing.T, withExecution bool) fixture {
	t.Helper()
	ctx := context.Background()
	tenantID := unique("tenant")
	userID := unique("user")
	scope, err := storage.NewScope(tenantID, userID, unique("request"), unique("trace"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := integrationStore.CreateTenant(ctx, storage.Tenant{TenantID: tenantID, CreatedAt: fixtureNow}); err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentity(userID, "test-provider", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateIdentity(ctx, scope, identity); err != nil {
		t.Fatal(err)
	}
	binding, err := wallet.NewBinding(wallet.BindingParams{BindingID: unique("binding"), Version: 2, UserID: userID, Provider: "test-provider", ProviderUserReference: unique("provider-user"), WalletID: unique("wallet"), Address: unique("address"), ChainID: "5042002", Network: "test-network", Status: wallet.BindingStatusActive, VerificationReference: unique("verification"), CreatedAt: fixtureNow.Add(-2 * time.Hour), VerifiedAt: fixtureNow.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreateBinding(ctx, scope, binding); err != nil {
		t.Fatal(err)
	}
	policyID := unique("policy")
	policy, err := policies.NewDraft(policies.Params{PolicyID: policyID, Version: 1, Name: "Test policy", Scope: policies.Scope{UserID: userID, WalletBindingID: binding.BindingID(), IntentTypes: []intents.Type{intents.TypePayroll}}, Rules: []policies.Rule{{RuleID: "operations", OnViolation: policies.DecisionDeny, OperationAllowlist: &policies.OperationAllowlistRule{Allowed: []intents.Type{intents.TypePayroll}}}}, CreatedAt: fixtureNow.Add(-2 * time.Hour), ValidFrom: fixtureNow.Add(-time.Hour), ExpiresAt: fixtureNow.Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	policy, err = policy.Transition(policies.StatusActive, fixtureNow.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.CreatePolicy(ctx, scope, policy); err != nil {
		t.Fatal(err)
	}
	token := intents.Token{ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}
	intentID := unique("intent")
	intent, err := intents.NewDraft(intents.Params{IntentID: intentID, Version: 1, ClientRequestID: unique("client-request"), Nonce: unique("nonce"), Type: intents.TypePayroll, Ownership: intents.Ownership{UserID: userID, IdentityProvider: binding.Provider(), ProviderUserReference: binding.ProviderUserReference(), WalletBindingID: binding.BindingID(), WalletBindingVersion: binding.Version(), WalletID: binding.WalletID(), WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network()}, Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{Token: token, Recipients: []intents.Recipient{{Address: unique("recipient"), Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}}, Total: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}}, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: unique("route"), Version: 1}, Constraints: intents.Constraints{Deadline: fixtureNow.Add(50 * time.Minute), PolicyReference: policy.Reference()}, CreatedAt: fixtureNow, ExpiresAt: fixtureNow.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	draft := intent
	if _, err = integrationStore.CreateIntent(ctx, scope, draft); err != nil {
		t.Fatal(err)
	}
	intent, err = draft.Transition(intents.StatusCreated, fixtureNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.FreezeIntent(ctx, scope, intent, draft.LifecycleRevision()); err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusApprovalRequired, fixtureNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.UpdateIntent(ctx, scope, intent, intent.LifecycleRevision()-1); err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.New(approvals.Params{ApprovalID: unique("approval"), Version: 1, ApprovalRequestID: unique("approval-request"), CreatedAt: fixtureNow.Add(3 * time.Second), ExpiresAt: fixtureNow.Add(30 * time.Minute)}, intent)
	if err != nil {
		t.Fatal(err)
	}
	approvalAudit := auditRecord(scope, unique("event"), audit.EventApprovalRequested, "approval", approval.ApprovalID(), fixtureNow.Add(3*time.Second))
	if _, err = integrationStore.CreateApprovalWithAudit(ctx, scope, approval, approvalAudit); err != nil {
		t.Fatal(err)
	}
	approved, err := approval.Approve(fixtureNow.Add(4 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.UpdateApproval(ctx, scope, approved, approval.LifecycleRevision()); err != nil {
		t.Fatal(err)
	}
	approvedIntent, err := intent.Approve(approved, fixtureNow.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = integrationStore.UpdateIntent(ctx, scope, approvedIntent, intent.LifecycleRevision()); err != nil {
		t.Fatal(err)
	}
	result := policies.Result{PolicyID: policy.PolicyID(), PolicyVersion: policy.Version(), IntentID: approvedIntent.IntentID(), IntentVersion: approvedIntent.Version(), IntentDigest: approvedIntent.Digest(), Stage: policies.EvaluationStageBeforeExecution, Decision: policies.DecisionAllow, EvaluatedAt: fixtureNow.Add(5500 * time.Millisecond)}
	if _, err = integrationStore.CreatePolicyEvaluation(ctx, scope, result); err != nil {
		t.Fatal(err)
	}
	f := fixture{scope: scope, identity: identity, binding: binding, intent: approvedIntent, approval: approved, policy: policy, policyResult: result}
	if !withExecution {
		return f
	}
	operation, err := intents.NewOperationIdentity(approvedIntent)
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := approved.Consume(fixtureNow.Add(6*time.Second), operation)
	if err != nil {
		t.Fatal(err)
	}
	request, err := execution.NewRequest(approvedIntent, consumed, result, fixtureNow.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	executionValue, err := execution.New(request)
	if err != nil {
		t.Fatal(err)
	}
	event := auditRecord(scope, unique("event"), audit.EventExecutionCreated, "execution", executionValue.ExecutionID(), fixtureNow.Add(6*time.Second))
	storedApproval, created, err := integrationStore.ConsumeApprovalAndCreateExecution(ctx, scope, consumed, approved.LifecycleRevision(), executionValue, event)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created {
		t.Fatal("fixture execution was not created")
	}
	f.approval = storedApproval
	f.execution = created.Execution
	return f
}
