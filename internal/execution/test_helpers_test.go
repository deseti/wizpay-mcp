package execution

import (
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

var executionTestNow = time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)

func requestParts(t *testing.T) (intents.Intent, approvals.Approval, policies.Result) {
	t.Helper()
	policy, err := policies.NewDraft(policies.Params{
		PolicyID: "policy_001", Version: 1, Name: "Execution test policy",
		Scope:     policies.Scope{UserID: "user_001", WalletBindingID: "bind_001"},
		Rules:     []policies.Rule{{RuleID: "operations", OnViolation: policies.DecisionDeny, OperationAllowlist: &policies.OperationAllowlistRule{Allowed: []intents.Type{intents.TypePayroll}}}},
		CreatedAt: executionTestNow.Add(-time.Hour), ValidFrom: executionTestNow.Add(-30 * time.Minute), ExpiresAt: executionTestNow.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err = policy.Transition(policies.StatusActive, policy.ValidFrom())
	if err != nil {
		t.Fatal(err)
	}

	token := intents.Token{ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}
	intent, err := intents.NewDraft(intents.Params{
		IntentID: "int_001", Version: 1, ClientRequestID: "req_001", Nonce: "nonce_001", Type: intents.TypePayroll,
		Ownership: intents.Ownership{
			UserID: "user_001", IdentityProvider: "circle", ProviderUserReference: "provider_user_001",
			WalletBindingID: "bind_001", WalletBindingVersion: 2, WalletID: "wallet_001",
			WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet",
		},
		Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{
			Token:      token,
			Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "100", BaseUnits: "100000000", Decimals: 6}}},
			Total:      intents.Amount{Decimal: "100", BaseUnits: "100000000", Decimals: 6},
		}},
		Route:       intents.Route{Type: intents.RouteAllowlistedContract, Reference: "route_001", Version: 1},
		Constraints: intents.Constraints{Deadline: executionTestNow.Add(50 * time.Minute), PolicyReference: policy.Reference()},
		CreatedAt:   executionTestNow, ExpiresAt: executionTestNow.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, executionTestNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusApprovalRequired, executionTestNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.New(approvals.Params{
		ApprovalID: "apr_001", Version: 1, ApprovalRequestID: "apr_req_001",
		CreatedAt: executionTestNow.Add(3 * time.Second), ExpiresAt: executionTestNow.Add(40 * time.Minute),
	}, intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Approve(executionTestNow.Add(4 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Approve(approval, executionTestNow.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	identity, err := auth.NewIdentity("user_001", "circle", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	identityContext, err := auth.NewIdentityContext(identity, auth.RequestMetadata{RequestID: "request_001"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := wallet.NewBinding(wallet.BindingParams{
		BindingID: "bind_001", Version: 2, UserID: "user_001", Provider: "circle", ProviderUserReference: "provider_user_001",
		WalletID: "wallet_001", Address: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet",
		Status: wallet.BindingStatusActive, VerificationReference: "verification_001",
		CreatedAt: executionTestNow.Add(-2 * time.Hour), VerifiedAt: executionTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	policyResult, err := policies.Evaluate(policy, intent, identityContext, binding, executionTestNow.Add(6*time.Second))
	if err != nil || policyResult.Decision != policies.DecisionAllow {
		t.Fatalf("policy result = (%+v, %v)", policyResult, err)
	}
	operation, err := intents.NewOperationIdentity(intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Consume(executionTestNow.Add(7*time.Second), operation)
	if err != nil {
		t.Fatal(err)
	}
	return intent, approval, policyResult
}

func mustRequest(t *testing.T) Request {
	t.Helper()
	intent, approval, policyResult := requestParts(t)
	request, err := NewRequest(intent, approval, policyResult, executionTestNow.Add(8*time.Second))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	return request
}

func mustExecution(t *testing.T) Execution {
	t.Helper()
	value, err := New(mustRequest(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return value
}

func advanceExecution(t *testing.T, value Execution, status Status, at time.Time) Execution {
	t.Helper()
	next, err := value.Transition(status, at)
	if err != nil {
		t.Fatalf("Transition(%s) error = %v", status, err)
	}
	return next
}
