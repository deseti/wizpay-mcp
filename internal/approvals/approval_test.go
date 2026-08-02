package approvals

import (
	stderrors "errors"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

var approvalNow = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

func approvalIntent(t *testing.T, id string) intents.Intent {
	t.Helper()
	token := intents.Token{ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}
	params := intents.Params{
		IntentID: id, Version: 1, ClientRequestID: "req_" + id, Nonce: "nonce_" + id, Type: intents.TypePayroll,
		Ownership:   intents.Ownership{UserID: "user_001", IdentityProvider: "circle", ProviderUserReference: "provider_user_001", WalletBindingID: "bind_001", WalletBindingVersion: 4, WalletID: "wallet_001", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet"},
		Financial:   intents.FinancialParameters{Payroll: &intents.PayrollParameters{Token: token, Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}}, Total: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}},
		Route:       intents.Route{Type: intents.RouteAllowlistedContract, Reference: "route_001", Version: 1},
		Constraints: intents.Constraints{Deadline: approvalNow.Add(20 * time.Minute), PolicyReference: "policy_v1"}, CreatedAt: approvalNow, ExpiresAt: approvalNow.Add(30 * time.Minute),
	}
	value, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	value, err = value.Transition(intents.StatusCreated, approvalNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	value, err = value.Transition(intents.StatusApprovalRequired, approvalNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func newApproval(t *testing.T, intent intents.Intent) Approval {
	t.Helper()
	value, err := New(Params{ApprovalID: "apr_001", Version: 1, ApprovalRequestID: "apr_req_001", CreatedAt: approvalNow.Add(3 * time.Second), ExpiresAt: approvalNow.Add(15 * time.Minute)}, intent)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return value
}

func TestApprovalBindsExactIntentIdentityAndWallet(t *testing.T) {
	intent := approvalIntent(t, "int_001")
	approval := newApproval(t, intent)
	owner := intent.Ownership()
	if approval.IntentDigest() != intent.Digest() || approval.IntentVersion() != intent.Version() {
		t.Fatal("approval intent binding mismatch")
	}
	if approval.UserID() != owner.UserID || approval.WalletBindingID() != owner.WalletBindingID || approval.WalletBindingVersion() != owner.WalletBindingVersion {
		t.Fatal("approval ownership binding mismatch")
	}
	if approval.Status() != StatusPending || approval.Decision() != DecisionPending {
		t.Fatal("new approval is not pending")
	}
}

func TestApprovalLifecycleAndConsumptionAreIdempotent(t *testing.T) {
	intent := approvalIntent(t, "int_001")
	approval := newApproval(t, intent)
	approved, err := approval.Approve(approvalNow.Add(4 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := approved.EnsureAuthorizes(intent, approvalNow.Add(5*time.Second)); err != nil {
		t.Fatalf("EnsureAuthorizes() error = %v", err)
	}
	approvedIntent, err := intent.Approve(approved, approvalNow.Add(5*time.Second))
	if err != nil || approvedIntent.Status() != intents.StatusApproved {
		t.Fatalf("intent.Approve() = (%s, %v)", approvedIntent.Status(), err)
	}
	operation, err := intents.NewOperationIdentity(intent)
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := approved.Consume(approvalNow.Add(6*time.Second), operation)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := consumed.Consume(approvalNow.Add(7*time.Second), operation)
	if err != nil || replay.OperationKey() != consumed.OperationKey() {
		t.Fatalf("consume replay = (%q, %v)", replay.OperationKey(), err)
	}
	if consumed.OperationVersion() != intents.OperationIdentityVersion {
		t.Fatal("consumed operation version mismatch")
	}
	if err := consumed.EnsureAuthorizes(intent, approvalNow.Add(8*time.Second)); !approvalHasCode(err, apperrors.CodeApprovalAlreadyConsumed) {
		t.Fatalf("consumed authorization error = %v", err)
	}
}

func TestApprovalRejectAndExpireAreTerminal(t *testing.T) {
	approval := newApproval(t, approvalIntent(t, "int_001"))
	rejected, err := approval.Reject(approvalNow.Add(4 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !rejected.Status().Terminal() {
		t.Fatal("rejected approval is not terminal")
	}
	if _, err := rejected.Approve(approvalNow.Add(5 * time.Second)); !approvalHasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("rejected-to-approved error = %v", err)
	}

	expired, err := approval.Expire(approval.ExpiresAt())
	if err != nil {
		t.Fatal(err)
	}
	if expired.Status() != StatusExpired {
		t.Fatalf("status = %s", expired.Status())
	}
	if _, err := approval.Approve(approval.ExpiresAt()); !approvalHasCode(err, apperrors.CodeApprovalExpired) {
		t.Fatalf("late approval error = %v", err)
	}
}

func TestApprovalRejectsDifferentIntent(t *testing.T) {
	first := approvalIntent(t, "int_001")
	second := approvalIntent(t, "int_002")
	approved, err := newApproval(t, first).Approve(approvalNow.Add(4 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := approved.EnsureAuthorizes(second, approvalNow.Add(5*time.Second)); !approvalHasCode(err, apperrors.CodeApprovalRequired) {
		t.Fatalf("mismatch error = %v", err)
	}
}

func approvalHasCode(err error, code apperrors.Code) bool {
	var appErr *apperrors.Error
	return stderrors.As(err, &appErr) && appErr.Code == code
}
