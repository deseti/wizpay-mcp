package policies

import (
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

func sendPolicyContext(t *testing.T, policyReference string) (intents.Intent, auth.IdentityContext, wallet.Binding) {
	t.Helper()
	resource, _ := contracts.CanonicalToken("USDC")
	token := intents.Token{ChainID: resource.ChainID, Standard: "ERC20", Address: resource.Address, Symbol: resource.Symbol, Decimals: resource.Decimals}
	params := intents.Params{IntentID: "send_policy", Version: 1, ClientRequestID: "send_client", Nonce: "send_nonce", Type: intents.TypeSend,
		Ownership: intents.Ownership{UserID: "user_001", IdentityProvider: "circle", ProviderUserReference: "provider_user_001", WalletBindingID: "bind_001", WalletBindingVersion: 2, WalletID: "wallet_001", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet},
		Financial: intents.FinancialParameters{Send: &intents.SendParameters{Token: token, Recipient: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "100", BaseUnits: "100000000", Decimals: 6}}},
		Route:     intents.Route{Type: intents.RouteDirectWallet, Reference: intents.RouteReferenceSend, Version: intents.RouteVersionSend}, Constraints: intents.Constraints{Deadline: policyTestNow.Add(50 * time.Minute), PolicyReference: policyReference}, CreatedAt: policyTestNow, ExpiresAt: policyTestNow.Add(time.Hour)}
	intent, err := intents.NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, policyTestNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusApprovalRequired, policyTestNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.New(approvals.Params{ApprovalID: "send_approval", Version: 1, ApprovalRequestID: "send_approval_request", CreatedAt: policyTestNow.Add(3 * time.Second), ExpiresAt: policyTestNow.Add(40 * time.Minute)}, intent)
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
	identity, _ := auth.NewIdentity("user_001", "circle", auth.IdentityStatusActive)
	identityContext, err := auth.NewIdentityContext(identity, auth.RequestMetadata{RequestID: "request"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := wallet.NewBinding(wallet.BindingParams{BindingID: "bind_001", Version: 2, UserID: "user_001", Provider: "circle", ProviderUserReference: "provider_user_001", WalletID: "wallet_001", Address: params.Ownership.WalletAddress, ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet, Status: wallet.BindingStatusActive, VerificationReference: "verified", CreatedAt: policyTestNow.Add(-time.Hour), VerifiedAt: policyTestNow.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	return intent, identityContext, binding
}

func TestSendPolicyAppliesOperationSpendingAndRecipientRules(t *testing.T) {
	resource, _ := contracts.CanonicalToken("USDC")
	token := TokenReference{ChainID: resource.ChainID, Standard: "ERC20", Address: resource.Address, Decimals: resource.Decimals}
	rules := []Rule{
		{RuleID: "operation", OnViolation: DecisionDeny, OperationAllowlist: &OperationAllowlistRule{Allowed: []intents.Type{intents.TypeSend}}},
		{RuleID: "spend", OnViolation: DecisionDeny, SpendingLimit: &SpendingLimitRule{IntentTypes: []intents.Type{intents.TypeSend}, Token: token, Maximum: intents.Amount{Decimal: "99", BaseUnits: "99000000", Decimals: 6}}},
		{RuleID: "recipient", OnViolation: DecisionDeny, Recipient: &RecipientRule{Allowed: []string{"0x3333333333333333333333333333333333333333"}}},
	}
	policy := mustActivePolicy(t, rules)
	intent, identity, binding := sendPolicyContext(t, policy.Reference())
	result, err := Evaluate(policy, intent, identity, binding, policyTestNow.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionDeny || len(result.Findings) != 1 || result.Findings[0].Reason != ReasonSpendingLimitExceeded {
		t.Fatalf("result = %+v", result)
	}
}
