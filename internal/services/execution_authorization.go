package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

// ExecutionAuthorization is a non-secret handoff record. It is deliberately
// insufficient to sign or submit a transaction.
type ExecutionAuthorization struct {
	AuthorizationID      string
	ApprovalID           string
	IntentID             string
	WalletBindingID      string
	WalletBindingVersion uint64
	Status               approvals.Status
	Amount               string
	Token                string
	WalletReference      string
	Recipient            string
	AgentIdentity        string
}

func (s *PersistedApprovalService) AuthorizeExecution(ctx context.Context, approvalID, intentID, walletBindingID string, walletBindingVersion uint64) (ExecutionAuthorization, error) {
	scope, err := s.scope(ctx, auth.PermissionPrepareExecution)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if s.Approvals == nil || s.Intents == nil || s.Wallets == nil || s.Now == nil {
		return ExecutionAuthorization{}, fmt.Errorf("approval service is not configured")
	}
	current, err := s.Approvals.FindApprovalByID(ctx, scope, approvalID)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if current.UserID() != scope.ActorID() {
		return ExecutionAuthorization{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Approval is not accessible.", false, true, true)
	}
	intent, err := s.Intents.FindIntentByID(ctx, scope, intentID)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if current.IntentID() != intent.IntentID() || current.IntentVersion() != intent.Version() || current.IntentDigest() != intent.Digest() {
		return ExecutionAuthorization{}, apperrors.New(apperrors.CodeApprovalRequired, "Approval does not match the intent.", false, true, true)
	}
	if walletBindingID != current.WalletBindingID() || walletBindingVersion != current.WalletBindingVersion() {
		return ExecutionAuthorization{}, apperrors.New(apperrors.CodeWalletMismatch, "Wallet binding does not match the approval.", false, true, true)
	}
	binding, err := s.Wallets.FindBindingByID(ctx, scope, walletBindingID)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if binding.Version() != current.WalletBindingVersion() {
		return ExecutionAuthorization{}, apperrors.New(apperrors.CodeWalletMismatch, "Wallet binding version has changed.", false, true, true)
	}
	if err := binding.EnsureMatches(wallet.Reference{UserID: current.UserID(), WalletID: current.WalletID(), Address: current.WalletAddress(), ChainID: current.ChainID()}); err != nil {
		return ExecutionAuthorization{}, err
	}
	if err := binding.EnsureAuthorizable(current.UserID()); err != nil {
		return ExecutionAuthorization{}, err
	}
	now := s.Now().UTC()
	if err := current.EnsureAuthorizes(intent, now); err != nil && current.Status() != approvals.StatusReadyForExecutionConfirmation {
		return ExecutionAuthorization{}, err
	}
	next, err := current.ReadyForExecutionConfirmation(now)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if next.Status() != current.Status() {
		if _, err = s.Approvals.UpdateApproval(ctx, scope, next, current.LifecycleRevision()); err != nil {
			return ExecutionAuthorization{}, err
		}
	}
	return executionAuthorizationOf(next, intent), nil
}

// GetExecutionConfirmation returns the safe, provider-neutral review data.
// It does not change lifecycle state.
func (s *PersistedApprovalService) GetExecutionConfirmation(ctx context.Context, approvalID string) (ExecutionAuthorization, error) {
	approval, err := s.GetApproval(ctx, approvalID)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if s.Intents == nil {
		return ExecutionAuthorization{}, fmt.Errorf("approval service is not configured")
	}
	scope, err := s.scope(ctx, auth.PermissionReadApproval)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	intent, err := s.Intents.FindIntentByID(ctx, scope, approval.IntentID())
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	return executionAuthorizationOf(approval, intent), nil
}

func executionAuthorizationOf(approval approvals.Approval, intent intents.Intent) ExecutionAuthorization {
	sum := sha256.Sum256([]byte("execution-authorization/" + approval.ApprovalID() + "/" + fmt.Sprint(approval.LifecycleRevision())))
	result := ExecutionAuthorization{AuthorizationID: "eauth_" + hex.EncodeToString(sum[:12]), ApprovalID: approval.ApprovalID(), IntentID: approval.IntentID(), WalletBindingID: approval.WalletBindingID(), WalletBindingVersion: approval.WalletBindingVersion(), Status: approval.Status(), WalletReference: approval.WalletID() + " / " + approval.WalletAddress(), AgentIdentity: approval.UserID()}
	financial := intent.Financial()
	if financial.Payroll != nil {
		p := financial.Payroll
		token := p.SourceToken()
		result.Amount = p.Total.Decimal
		result.Token = token.Symbol
		if len(p.Recipients) > 0 {
			result.Recipient = p.Recipients[0].Address
		}
	} else if financial.Swap != nil {
		result.Amount = financial.Swap.InputAmount.Decimal
		result.Token = financial.Swap.InputToken.Symbol
		result.Recipient = financial.Swap.Recipient
	}
	return result
}
