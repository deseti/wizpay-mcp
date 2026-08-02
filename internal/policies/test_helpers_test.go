package policies

import (
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

var policyTestNow = time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)

var policyTestToken = intents.Token{
	ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6,
}

func policyTestTokenReference() TokenReference {
	return TokenReference{ChainID: policyTestToken.ChainID, Standard: policyTestToken.Standard, Address: policyTestToken.Address, Decimals: policyTestToken.Decimals}
}

func spendingRule(id, maximum string, decision Decision) Rule {
	return Rule{
		RuleID: id, OnViolation: decision,
		SpendingLimit: &SpendingLimitRule{
			IntentTypes: []intents.Type{intents.TypePayroll}, Token: policyTestTokenReference(),
			Maximum: intents.Amount{Decimal: maximum, BaseUnits: maximum + "000000", Decimals: 6},
		},
	}
}

func policyParams(rules []Rule) Params {
	return Params{
		PolicyID: "policy_001", Version: 1, Name: "Primary payment policy",
		Scope: Scope{UserID: "user_001", WalletBindingID: "bind_001"}, Rules: rules,
		CreatedAt: policyTestNow.Add(-time.Hour), ValidFrom: policyTestNow.Add(-30 * time.Minute), ExpiresAt: policyTestNow.Add(24 * time.Hour),
	}
}

func mustPolicy(t *testing.T, rules []Rule) Policy {
	t.Helper()
	policy, err := NewDraft(policyParams(rules))
	if err != nil {
		t.Fatalf("NewDraft() error = %v", err)
	}
	return policy
}

func mustActivePolicy(t *testing.T, rules []Rule) Policy {
	t.Helper()
	policy := mustPolicy(t, rules)
	active, err := policy.Transition(StatusActive, policyTestNow.Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("Transition(ACTIVE) error = %v", err)
	}
	return active
}

func createdPayrollIntent(t *testing.T, policyReference, decimal, baseUnits string) intents.Intent {
	t.Helper()
	params := intents.Params{
		IntentID: "int_001", Version: 1, ClientRequestID: "req_001", Nonce: "nonce_001", Type: intents.TypePayroll,
		Ownership: intents.Ownership{
			UserID: "user_001", IdentityProvider: "circle", ProviderUserReference: "provider_user_001",
			WalletBindingID: "bind_001", WalletBindingVersion: 2, WalletID: "wallet_001",
			WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet",
		},
		Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{
			Token:      policyTestToken,
			Recipients: []intents.Recipient{{Address: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: decimal, BaseUnits: baseUnits, Decimals: 6}}},
			Total:      intents.Amount{Decimal: decimal, BaseUnits: baseUnits, Decimals: 6},
		}},
		Route:       intents.Route{Type: intents.RouteAllowlistedContract, Reference: "route_001", Version: 1},
		Constraints: intents.Constraints{Deadline: policyTestNow.Add(50 * time.Minute), PolicyReference: policyReference},
		CreatedAt:   policyTestNow, ExpiresAt: policyTestNow.Add(time.Hour),
	}
	intent, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, policyTestNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func approvedPayrollIntent(t *testing.T, policyReference, decimal, baseUnits string) intents.Intent {
	t.Helper()
	intent := createdPayrollIntent(t, policyReference, decimal, baseUnits)
	var err error
	intent, err = intent.Transition(intents.StatusApprovalRequired, policyTestNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.New(approvals.Params{ApprovalID: "apr_001", Version: 1, ApprovalRequestID: "apr_req_001", CreatedAt: policyTestNow.Add(3 * time.Second), ExpiresAt: policyTestNow.Add(40 * time.Minute)}, intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Approve(policyTestNow.Add(4 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Approve(approval, policyTestNow.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func evaluationContext(t *testing.T) (auth.IdentityContext, wallet.Binding) {
	t.Helper()
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
		Status: wallet.BindingStatusActive, VerificationReference: "verification_001", CreatedAt: policyTestNow.Add(-2 * time.Hour), VerifiedAt: policyTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return identityContext, binding
}
