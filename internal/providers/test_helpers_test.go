package providers

import (
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

var providerTestNow = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

const (
	testChainID     = "5042002"
	testNetwork     = "ARC-TESTNET"
	testWalletID    = "wallet-test"
	testSourceAddr  = "0x2222222222222222222222222222222222222222"
	testDestination = "0x3333333333333333333333333333333333333333"
	testHash        = "0x1111111111111111111111111111111111111111111111111111111111111111"
)

// testRequest builds a fully authorized Phase 9 execution request. Provider
// idempotency is derived from this identity, so the fixture must be real rather
// than synthesized.
func testRequest(t *testing.T, intentID string) execution.Request {
	t.Helper()
	policy, err := policies.NewDraft(policies.Params{
		PolicyID: "policy-test", Version: 1, Name: "Provider test",
		Scope: policies.Scope{UserID: "user-test", WalletBindingID: "binding-test"},
		Rules: []policies.Rule{{RuleID: "operations", OnViolation: policies.DecisionDeny,
			OperationAllowlist: &policies.OperationAllowlistRule{Allowed: []intents.Type{intents.TypePayroll}}}},
		CreatedAt: providerTestNow.Add(-time.Hour), ValidFrom: providerTestNow.Add(-30 * time.Minute),
		ExpiresAt: providerTestNow.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err = policy.Transition(policies.StatusActive, policy.ValidFrom())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentity("user-test", "circle", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	identityContext, err := auth.NewIdentityContext(identity, auth.RequestMetadata{RequestID: "request-test"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := wallet.NewBinding(wallet.BindingParams{
		BindingID: "binding-test", Version: 1, UserID: "user-test", Provider: "circle",
		ProviderUserReference: "provider-user-test", WalletID: testWalletID, Address: testSourceAddr,
		ChainID: testChainID, Network: testNetwork, Status: wallet.BindingStatusActive,
		VerificationReference: "binding-verification",
		CreatedAt:             providerTestNow.Add(-2 * time.Hour), VerifiedAt: providerTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	token := intents.Token{ChainID: testChainID, Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}
	intent, err := intents.NewDraft(intents.Params{
		IntentID: intentID, Version: 1, ClientRequestID: "client-" + intentID, Nonce: "nonce-" + intentID,
		Type: intents.TypePayroll,
		Ownership: intents.Ownership{
			UserID: "user-test", IdentityProvider: "circle", ProviderUserReference: "provider-user-test",
			WalletBindingID: "binding-test", WalletBindingVersion: 1, WalletID: testWalletID,
			WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network(),
		},
		Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{
			Token:      token,
			Recipients: []intents.Recipient{{Address: testDestination, Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}},
			Total:      intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6},
		}},
		Route:       intents.Route{Type: intents.RouteAllowlistedContract, Reference: "route-test", Version: 1},
		Constraints: intents.Constraints{Deadline: providerTestNow.Add(50 * time.Minute), PolicyReference: policy.Reference()},
		CreatedAt:   providerTestNow, ExpiresAt: providerTestNow.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, providerTestNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusApprovalRequired, providerTestNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.New(approvals.Params{
		ApprovalID: "approval-" + intentID, Version: 1, ApprovalRequestID: "approval-request-" + intentID,
		CreatedAt: providerTestNow.Add(3 * time.Second), ExpiresAt: providerTestNow.Add(40 * time.Minute),
	}, intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Approve(providerTestNow.Add(4 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Approve(approval, providerTestNow.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	policyResult, err := policies.Evaluate(policy, intent, identityContext, binding, providerTestNow.Add(6*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := intents.NewOperationIdentity(intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Consume(providerTestNow.Add(7*time.Second), operation)
	if err != nil {
		t.Fatal(err)
	}
	request, err := execution.NewRequest(intent, approval, policyResult, providerTestNow.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func testExecution(t *testing.T) execution.Execution {
	t.Helper()
	value, err := execution.New(testRequest(t, "intent-test"))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func fixedClock() func() time.Time {
	return func() time.Time { return providerTestNow }
}
