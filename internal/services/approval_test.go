package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

var approvalServiceNow = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

func approvalHasCode(err error, code apperrors.Code) bool {
	var value *apperrors.Error
	return errors.As(err, &value) && value.Code == code
}

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
	listed   []approvals.Approval
	options  storage.ApprovalListOptions
}

type approvalWalletRepository struct{ binding wallet.Binding }

func (r approvalWalletRepository) FindBindingByID(context.Context, storage.Scope, string) (wallet.Binding, error) {
	return r.binding, nil
}
func (r approvalWalletRepository) FindBindingByWallet(context.Context, storage.Scope, string, string, string) (wallet.Binding, error) {
	return r.binding, nil
}
func (r approvalWalletRepository) CreateBinding(context.Context, storage.Scope, wallet.Binding) (storage.CreateBindingResult, error) {
	panic("unused")
}
func (r approvalWalletRepository) UpdateBinding(context.Context, storage.Scope, wallet.Binding, uint64) (wallet.Binding, error) {
	panic("unused")
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
func (r *approvalRepositoryStub) ListApprovals(_ context.Context, _ storage.Scope, options storage.ApprovalListOptions) ([]approvals.Approval, error) {
	r.options = options
	if r.listed != nil {
		return r.listed, nil
	}
	if r.approval.ApprovalID() == "" {
		return nil, errors.New("approval not found")
	}
	return []approvals.Approval{r.approval}, nil
}
func (r *approvalRepositoryStub) CreateApproval(_ context.Context, _ storage.Scope, value approvals.Approval) (storage.CreateApprovalResult, error) {
	r.created++
	if r.approval.ApprovalID() != "" {
		return storage.CreateApprovalResult{Approval: r.approval, Created: false}, nil
	}
	r.approval = value
	return storage.CreateApprovalResult{Approval: value, Created: true}, nil
}
func (r *approvalRepositoryStub) UpdateApproval(_ context.Context, _ storage.Scope, value approvals.Approval, _ uint64) (approvals.Approval, error) {
	r.approval = value
	return value, nil
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
	binding, err := wallet.NewBinding(wallet.BindingParams{BindingID: "binding_1", Version: 1, UserID: "user_1", Provider: "issuer", ProviderUserReference: "subject_1", WalletID: "wallet_1", Address: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet", Status: wallet.BindingStatusActive, VerificationReference: "verified", CreatedAt: approvalServiceNow.Add(-2 * time.Minute), VerifiedAt: approvalServiceNow.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	service := &PersistedApprovalService{Approvals: approvalsRepo, Intents: approvalIntentRepository{intent: intent}, Wallets: approvalWalletRepository{binding: binding}, Authorizer: auth.NewPermissionAuthorizer(), Now: func() time.Time { return approvalServiceNow }}
	principal, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant_1", ActorID: "user_1", IdentityProvider: "issuer", ProviderSubject: "subject_1", ExpiresAt: approvalServiceNow.Add(time.Hour), Permissions: []auth.Permission{auth.PermissionRequestApproval, auth.PermissionReadApproval, auth.PermissionPrepareExecution}})
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

func TestPersistedApprovalServiceAuthorizeExecutionRequiresApprovalAndBinding(t *testing.T) {
	service, repository, ctx, intent := approvalServiceFixture(t, intents.StatusApprovalRequired)
	approval, err := service.RequestApproval(ctx, intent.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthorizeExecution(ctx, approval.ApprovalID(), intent.IntentID(), approval.WalletBindingID(), approval.WalletBindingVersion()); !approvalHasCode(err, apperrors.CodeApprovalRequired) {
		t.Fatalf("pending authorization error = %v", err)
	}
	approved, err := service.DecideApproval(ctx, approval.ApprovalID(), approvals.DecisionApproved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthorizeExecution(ctx, approved.ApprovalID(), intent.IntentID(), "wrong-binding", approved.WalletBindingVersion()); !approvalHasCode(err, apperrors.CodeWalletMismatch) {
		t.Fatalf("mismatch error = %v", err)
	}
	value, err := service.AuthorizeExecution(ctx, approved.ApprovalID(), intent.IntentID(), approved.WalletBindingID(), approved.WalletBindingVersion())
	if err != nil || value.Status != approvals.StatusReadyForExecutionConfirmation || repository.approval.Status() != approvals.StatusReadyForExecutionConfirmation {
		t.Fatalf("authorization = %#v err=%v", value, err)
	}
}

func TestPersistedApprovalServiceAuthorizeExecutionRejectsUnauthorizedAndExpired(t *testing.T) {
	service, _, ctx, intent := approvalServiceFixture(t, intents.StatusApprovalRequired)
	approval, err := service.RequestApproval(ctx, intent.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthorizeExecution(context.Background(), approval.ApprovalID(), intent.IntentID(), approval.WalletBindingID(), approval.WalletBindingVersion()); err == nil {
		t.Fatal("missing authentication was accepted")
	}
	if _, err := service.DecideApproval(ctx, approval.ApprovalID(), approvals.DecisionApproved); err != nil {
		t.Fatal(err)
	}
	service.Now = func() time.Time { return intent.ExpiresAt().Add(time.Second) }
	if _, err := service.AuthorizeExecution(ctx, approval.ApprovalID(), intent.IntentID(), approval.WalletBindingID(), approval.WalletBindingVersion()); !approvalHasCode(err, apperrors.CodeApprovalExpired) {
		t.Fatalf("expired authorization error = %v", err)
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

func TestPersistedApprovalServiceListIsScopedAndPaginated(t *testing.T) {
	service, repository, ctx, intent := approvalServiceFixture(t, intents.StatusApprovalRequired)
	approval, err := service.RequestApproval(ctx, intent.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	repository.listed = []approvals.Approval{approval, approval, approval}
	page, err := service.ListApprovals(ctx, 2, "cursor_1", string(approvals.StatusApproved))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Approvals) != 2 || page.NextCursor != approval.ApprovalID() {
		t.Fatalf("page=%#v", page)
	}
	if repository.options.Cursor != "cursor_1" || repository.options.Status != string(approvals.StatusApproved) || repository.options.Limit != 3 {
		t.Fatalf("options=%#v", repository.options)
	}
	if _, err := service.ListApprovals(context.Background(), 2, "", ""); err == nil {
		t.Fatal("unauthorized listing was accepted")
	}
}

func TestPersistedApprovalServiceListRejectsInvalidStatus(t *testing.T) {
	service, _, ctx, _ := approvalServiceFixture(t, intents.StatusApprovalRequired)
	if _, err := service.ListApprovals(ctx, 10, "", "NOT_A_STATUS"); !approvalHasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("invalid status error=%v", err)
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

func TestPersistedApprovalServiceApproveAndReject(t *testing.T) {
	for _, decision := range []approvals.Decision{approvals.DecisionApproved, approvals.DecisionRejected} {
		t.Run(string(decision), func(t *testing.T) {
			service, _, ctx, intent := approvalServiceFixture(t, intents.StatusApprovalRequired)
			pending, err := service.RequestApproval(ctx, intent.IntentID())
			if err != nil {
				t.Fatal(err)
			}
			updated, err := service.DecideApproval(ctx, pending.ApprovalID(), decision)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Decision() != decision || updated.Status() != approvals.Status(decision) {
				t.Fatalf("status=%s decision=%s", updated.Status(), updated.Decision())
			}
		})
	}
}
