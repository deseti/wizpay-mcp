package policies

import (
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/wallet"
	"testing"
	"time"
)

func TestSeparatedAuthenticationIssuerAndWalletProvider(t *testing.T) {
	policy := mustActivePolicy(t, []Rule{spendingRule("cap", "500", DecisionDeny)})
	old := createdPayrollIntent(t, policy.Reference(), "1", "1000000")
	o := old.Ownership()
	o.IdentityProvider = "WIZPAY_OAUTH"
	o.WalletProvider = "EXTERNAL_EVM"
	p := intents.Params{IntentID: old.IntentID(), Version: old.Version(), ClientRequestID: old.ClientRequestID(), Nonce: old.Nonce(), Type: old.Type(), Ownership: o, Financial: old.Financial(), Route: old.Route(), Constraints: old.Constraints(), CreatedAt: old.CreatedAt(), ExpiresAt: old.ExpiresAt()}
	i, err := intents.NewDraft(p)
	if err != nil {
		t.Fatal(err)
	}
	i, err = i.Transition(intents.StatusCreated, policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentityWithSubject(o.UserID, "WIZPAY_OAUTH", "human-subject", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	identityCtx, err := auth.NewIdentityContext(identity, auth.RequestMetadata{RequestID: "request"})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"valid", "owner", "version", "provider", "pending", "revoked"} {
		t.Run(kind, func(t *testing.T) {
			b := wallet.BindingParams{BindingID: o.WalletBindingID, Version: o.WalletBindingVersion, UserID: o.UserID, Provider: o.WalletProvider, ProviderUserReference: o.ProviderUserReference, WalletID: o.WalletID, Address: o.WalletAddress, ChainID: o.ChainID, Network: o.Network, Status: wallet.BindingStatusActive, VerificationReference: "fixture-evidence", CreatedAt: policyTestNow.Add(-time.Hour), VerifiedAt: policyTestNow.Add(-time.Minute)}
			switch kind {
			case "owner":
				b.UserID = "other"
			case "version":
				b.Version++
			case "provider":
				b.Provider = "other"
			case "pending":
				b.Status = wallet.BindingStatusPending
				b.VerifiedAt = time.Time{}
				b.VerificationReference = ""
			case "revoked":
				b.Status = wallet.BindingStatusRevoked
				b.RevokedAt = policyTestNow
			}
			binding, err := wallet.NewBinding(b)
			if err != nil {
				t.Fatal(err)
			}
			result, err := EvaluateForApproval(policy, i, identityCtx, binding, policyTestNow.Add(time.Second))
			if kind == "valid" {
				if err != nil || result.Error() != nil {
					t.Fatalf("separated identity rejected: %v %+v", err, result)
				}
			} else if err == nil {
				t.Fatal("invalid relationship accepted")
			}
		})
	}
}
