package services

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/wallet"
	"testing"
	"time"
)

type testHumanAuthenticator struct{ request auth.TrustedRequest }

func (v testHumanAuthenticator) AuthenticateHuman(context.Context) (auth.TrustedRequest, error) {
	return v.request, nil
}
func trustedHumanFixture(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	r, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = auth.AuthenticateHumanContext(ctx, testHumanAuthenticator{r})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}
func TestMCPPermissionStringsCannotDecideOrConfirm(t *testing.T) {
	s, r, ctx, i := approvalServiceFixture(t, intents.StatusApprovalRequired)
	a, err := s.RequestApproval(ctx, i.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	// Fixture includes both privileged permission strings but no human attestation.
	for _, d := range []approvals.Decision{approvals.DecisionApproved, approvals.DecisionRejected} {
		if _, err = s.DecideApproval(ctx, a.ApprovalID(), d); err == nil {
			t.Fatal("MCP decided approval")
		}
	}
	if _, err = s.AuthorizeExecution(ctx, a.ApprovalID(), i.IntentID(), a.WalletBindingID(), a.WalletBindingVersion()); err == nil {
		t.Fatal("MCP confirmed execution")
	}
	if r.approval.Status() != approvals.StatusPending {
		t.Fatal("denial mutated approval")
	}
}
func TestHumanDecisionRejectsChangedBinding(t *testing.T) {
	for _, kind := range []string{"owner", "version", "pending", "revoked", "provider"} {
		t.Run(kind, func(t *testing.T) {
			s, r, ctx, i := approvalServiceFixture(t, intents.StatusApprovalRequired)
			a, err := s.RequestApproval(ctx, i.IntentID())
			if err != nil {
				t.Fatal(err)
			}
			ctx = trustedHumanFixture(t, ctx)
			b := s.Wallets.(approvalWalletRepository).binding
			p := wallet.BindingParams{BindingID: b.BindingID(), Version: b.Version(), UserID: b.OwnerUserID(), Provider: b.Provider(), ProviderUserReference: b.ProviderUserReference(), WalletID: b.WalletID(), Address: b.Address(), ChainID: b.ChainID(), Network: b.Network(), Status: b.Status(), VerificationReference: b.VerificationReference(), CreatedAt: b.CreatedAt(), VerifiedAt: b.VerifiedAt()}
			switch kind {
			case "owner":
				p.UserID = "other"
			case "version":
				p.Version++
			case "provider":
				p.Provider = "other"
			case "pending":
				p.Status = wallet.BindingStatusPending
				p.VerifiedAt = time.Time{}
				p.VerificationReference = ""
			case "revoked":
				p.Status = wallet.BindingStatusRevoked
				p.RevokedAt = approvalServiceNow
			}
			b, err = wallet.NewBinding(p)
			if err != nil {
				t.Fatal(err)
			}
			s.Wallets = approvalWalletRepository{b}
			if _, err = s.DecideApproval(ctx, a.ApprovalID(), approvals.DecisionApproved); err == nil {
				t.Fatal("changed binding accepted")
			}
			if r.approval.Status() != approvals.StatusPending {
				t.Fatal("denial mutated approval")
			}
		})
	}
}
func TestHumanDecisionRejectsExpiredAndRejected(t *testing.T) {
	s, _, ctx, i := approvalServiceFixture(t, intents.StatusApprovalRequired)
	a, err := s.RequestApproval(ctx, i.IntentID())
	if err != nil {
		t.Fatal(err)
	}
	ctx = trustedHumanFixture(t, ctx)
	if _, err = s.DecideApproval(ctx, a.ApprovalID(), approvals.DecisionRejected); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecideApproval(ctx, a.ApprovalID(), approvals.DecisionApproved); err == nil {
		t.Fatal("rejected approval accepted")
	}
	s.Now = func() time.Time { return a.ExpiresAt().Add(time.Second) }
	if _, err = s.DecideApproval(ctx, a.ApprovalID(), approvals.DecisionRejected); err == nil {
		t.Fatal("expired decision accepted")
	}
}
