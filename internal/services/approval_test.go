package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

var approvalServiceNow = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

type approvalIntentRepository struct{ intent intents.Intent }

func (r approvalIntentRepository) FindIntentByID(context.Context, storage.Scope, string) (intents.Intent, error) {
	return r.intent, nil
}
func (approvalIntentRepository) FindIntentByClientRequestID(context.Context, storage.Scope, string) (intents.Intent, error) {
	panic("unused")
}
func (approvalIntentRepository) FindIntentByOperationKey(context.Context, storage.Scope, string, uint64) (intents.Intent, error) {
	panic("unused")
}
func (approvalIntentRepository) CreateIntent(context.Context, storage.Scope, intents.Intent) (storage.CreateIntentResult, error) {
	panic("unused")
}
func (approvalIntentRepository) FreezeIntent(context.Context, storage.Scope, intents.Intent, uint64) (intents.Intent, error) {
	panic("unused")
}
func (approvalIntentRepository) UpdateIntent(context.Context, storage.Scope, intents.Intent, uint64) (intents.Intent, error) {
	panic("unused")
}

type approvalRepositoryStub struct {
	approval approvals.Approval
	created  int
}

func (r *approvalRepositoryStub) FindApprovalByID(context.Context, storage.Scope, string) (approvals.Approval, error) {
	if r.approval.ApprovalID() == "" {
		return approvals.Approval{}, errors.New("approval not found")
	}
	return r.approval, nil
}
func (r *approvalRepositoryStub) FindApprovalByIntent(context.Context, storage.Scope, string, uint64, string) (approvals.Approval, error) {
	if r.approval.ApprovalID() == "" {
		return approvals.Approval{}, errors.New("approval not found")
	}
	return r.approval, nil
}
func (r *approvalRepositoryStub) CreateApproval(_ context.Context, _ storage.Scope, value approvals.Approval) (storage.CreateApprovalResult, error) {
	r.created++
	if r.approval.ApprovalID() != "" {
		return storage.CreateApprovalResult{Approval: r.approval, Created: false}, nil
	}
	r.approval = value
	return storage.CreateApprovalResult{Approval: value, Created: true}, nil
}
func (r *approvalRepositoryStub) UpdateApproval(context.Context, storage.Scope, approvals.Approval, uint64) (approvals.Approval, error) {
	panic("unused")
}

func approvalServiceFixture(t *testing.T, intentStatus intents.Status) (*PersistedApprovalService, *approvalRepositoryStub, context.Context, intents.Intent) {
	t.Helper()
	params := intents.Params{
		IntentID: "intent_service_1", Version: 1, ClientRequestID: "request_service_1", Nonce: "nonce_service_1", Type: intents.TypePayroll,
		Ownership: intents.Ownership{UserID: "user_1", IdentityProvider: "issuer", ProviderUserReference: "subject_1", WalletBindingID: "binding_1", WalletBindingVersion: 1, WalletID: "wallet_1", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet"},
		Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{Token: intents.Token{ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}, Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}}, Total: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}},
		Route:     intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferencePayroll, Version: intents.RouteVersionPayroll}, Constraints: intents.Constraints{Deadline: approvalServiceNow.Add(10 * time.Minute), PolicyReference: "policy_1"}, CreatedAt: approvalServiceNow.Add(-time.Minute), ExpiresAt: approvalServiceNow.Add(time.Hour),
	}
	intent, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, approvalServiceNow.Add(-30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if intentStatus == intents.StatusApprovalRequired {
		intent, err = intent.Transition(intentStatus, approvalServiceNow.Add(-time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	approvalsRepo := &approvalRepositoryStub{}
	service := &PersistedApprovalService{Approvals: approvalsRepo, Intents: approvalIntentRepository{intent: intent}, Authorizer: auth.NewPermissionAuthorizer(), Now: func() time.Time { return approvalServiceNow }}
	principal, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant_1", ActorID: "user_1", IdentityProvider: "issuer", ProviderSubject: "subject_1", ExpiresAt: approvalServiceNow.Add(time.Hour), Permissions: []auth.Permission{auth.PermissionRequestApproval, auth.PermissionReadApproval}})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentityWithSubject("user_1", "issuer", "subject_1", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	request, err := auth.NewTrustedRequest(principal, identity, auth.RequestMetadata{RequestID: "request_1", TraceID: "trace_1"})
	if err != nil {
		t.Fatal(err)
	}
	return service, approvalsRepo, auth.WithTrustedRequest(context.Background(), request), intent
}

func TestPersistedApprovalServiceRequestCreatesApproval(t *testing.T) {
	service, repository, ctx, intent := approvalServiceFixture(t, intents.StatusApprovalRequired)
	value, err := service.RequestApproval(ctx, intent.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	if repository.created != 1 || value.IntentID() != intent.IntentID() || value.IntentVersion() != intent.Version() || value.IntentDigest() != intent.Digest() {
		t.Fatalf("created=%d approval=%+v intent=%+v", repository.created, value, intent)
	}
}

func TestPersistedApprovalServiceRequestIsIdempotent(t *testing.T) {
	service, repository, ctx, intent := approvalServiceFixture(t, intents.StatusApprovalRequired)
	first, err := service.RequestApproval(ctx, intent.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.RequestApproval(ctx, intent.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	if repository.created != 1 || first.ApprovalID() != second.ApprovalID() {
		t.Fatalf("created=%d first=%q second=%q", repository.created, first.ApprovalID(), second.ApprovalID())
	}
}

func TestPersistedApprovalServiceGetReturnsExistingApproval(t *testing.T) {
	service, repository, ctx, intent := approvalServiceFixture(t, intents.StatusApprovalRequired)
	want, err := approvals.New(approvals.Params{ApprovalID: "approval_existing", Version: 1, ApprovalRequestID: "approval_request_existing", CreatedAt: approvalServiceNow, ExpiresAt: intent.ExpiresAt()}, intent)
	if err != nil {
		t.Fatal(err)
	}
	repository.approval = want
	got, err := service.GetApproval(ctx, want.ApprovalID())
	if err != nil || got.ApprovalID() != want.ApprovalID() {
		t.Fatalf("approval=%q error=%v", got.ApprovalID(), err)
	}
}

func TestPersistedApprovalServiceAuthorizationFailure(t *testing.T) {
	service, _, _, _ := approvalServiceFixture(t, intents.StatusApprovalRequired)
	if _, err := service.GetApproval(context.Background(), "approval_1"); err == nil {
		t.Fatal("GetApproval accepted unauthenticated request")
	}
}

func TestPersistedApprovalServiceRejectsUnfrozenIntent(t *testing.T) {
	service, repository, ctx, intent := approvalServiceFixture(t, intents.StatusCreated)
	if _, err := service.RequestApproval(ctx, intent.IntentID()); err == nil {
		t.Fatal("RequestApproval accepted an intent that was not frozen for approval")
	}
	if repository.created != 0 {
		t.Fatal("RequestApproval persisted an approval for an unfrozen intent")
	}
}
