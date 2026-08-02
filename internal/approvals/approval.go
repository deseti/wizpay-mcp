// Package approvals defines explicit approval artifacts bound to frozen intents.
// It contains no signatures, provider challenges, UI, or execution capability.
package approvals

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

const maxFieldLength = 256

type Params struct {
	ApprovalID        string
	Version           uint64
	ApprovalRequestID string
	CreatedAt         time.Time
	ExpiresAt         time.Time
}

// Approval is an immutable, explicit user-decision artifact. Intent and wallet
// binding fields are derived from the frozen intent rather than caller input.
type Approval struct {
	approvalID           string
	version              uint64
	approvalRequestID    string
	intentID             string
	intentVersion        uint64
	intentDigest         string
	userID               string
	walletBindingID      string
	walletBindingVersion uint64
	walletID             string
	walletAddress        string
	chainID              string
	status               Status
	decision             Decision
	createdAt            time.Time
	expiresAt            time.Time
	decidedAt            time.Time
	consumedAt           time.Time
	operationKey         string
	operationVersion     uint64
}

func New(params Params, intent intents.Intent) (Approval, error) {
	if err := intent.Validate(); err != nil {
		return Approval{}, err
	}
	if intent.Status() != intents.StatusApprovalRequired {
		return Approval{}, apperrors.New(apperrors.CodeValidationError, "Intent is not awaiting approval.", false, true, true)
	}
	params.ApprovalID = strings.TrimSpace(params.ApprovalID)
	params.ApprovalRequestID = strings.TrimSpace(params.ApprovalRequestID)
	params.CreatedAt = params.CreatedAt.UTC()
	params.ExpiresAt = params.ExpiresAt.UTC()
	owner := intent.Ownership()
	a := Approval{
		approvalID: params.ApprovalID, version: params.Version, approvalRequestID: params.ApprovalRequestID,
		intentID: intent.IntentID(), intentVersion: intent.Version(), intentDigest: intent.Digest(),
		userID: owner.UserID, walletBindingID: owner.WalletBindingID,
		walletBindingVersion: owner.WalletBindingVersion, walletID: owner.WalletID,
		walletAddress: owner.WalletAddress, chainID: owner.ChainID,
		status: StatusPending, decision: DecisionPending,
		createdAt: params.CreatedAt, expiresAt: params.ExpiresAt,
	}
	if err := a.Validate(); err != nil {
		return Approval{}, err
	}
	if a.createdAt.Before(intent.CreatedAt()) || a.expiresAt.After(intent.ExpiresAt()) {
		return Approval{}, apperrors.New(apperrors.CodeValidationError, "Approval validity must be contained by intent validity.", false, true, true)
	}
	return a, nil
}

func (a Approval) Validate() error {
	for _, field := range []struct{ name, value string }{
		{"approval ID", a.approvalID}, {"approval request ID", a.approvalRequestID},
		{"intent ID", a.intentID}, {"intent digest", a.intentDigest}, {"user ID", a.userID},
		{"wallet binding ID", a.walletBindingID}, {"wallet ID", a.walletID},
		{"wallet address", a.walletAddress}, {"chain ID", a.chainID},
	} {
		if err := validateText(field.name, field.value); err != nil {
			return apperrors.Wrap(apperrors.CodeValidationError, "Approval is invalid.", false, true, true, err)
		}
	}
	if a.version == 0 || a.intentVersion == 0 || a.walletBindingVersion == 0 {
		return apperrors.New(apperrors.CodeValidationError, "Approval versions must be at least 1.", false, true, true)
	}
	if !a.status.Valid() || !a.decision.Valid() {
		return apperrors.New(apperrors.CodeValidationError, "Approval lifecycle value is invalid.", false, true, true)
	}
	if a.createdAt.IsZero() || a.expiresAt.IsZero() || !a.expiresAt.After(a.createdAt) {
		return apperrors.New(apperrors.CodeValidationError, "Approval validity period is invalid.", false, true, true)
	}
	switch a.status {
	case StatusPending:
		if a.decision != DecisionPending || !a.decidedAt.IsZero() || !a.consumedAt.IsZero() || a.operationKey != "" || a.operationVersion != 0 {
			return invalidLifecycle()
		}
	case StatusApproved:
		if a.decision != DecisionApproved || a.decidedAt.IsZero() || !a.consumedAt.IsZero() || a.operationKey != "" || a.operationVersion != 0 {
			return invalidLifecycle()
		}
	case StatusRejected:
		if a.decision != DecisionRejected || a.decidedAt.IsZero() || !a.consumedAt.IsZero() || a.operationKey != "" || a.operationVersion != 0 {
			return invalidLifecycle()
		}
	case StatusExpired:
		if !a.consumedAt.IsZero() || a.operationKey != "" || a.operationVersion != 0 {
			return invalidLifecycle()
		}
	case StatusConsumed:
		if a.decision != DecisionApproved || a.decidedAt.IsZero() || a.consumedAt.IsZero() || a.operationKey == "" || a.operationVersion == 0 {
			return invalidLifecycle()
		}
	}
	return nil
}

func invalidLifecycle() error {
	return apperrors.New(apperrors.CodeValidationError, "Approval lifecycle metadata is invalid.", false, true, true)
}

// EnsureAuthorizes fails closed unless every intent, identity, and wallet
// binding field still matches and the approval is currently effective.
func (a Approval) EnsureAuthorizes(intent intents.Intent, at time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := intent.Validate(); err != nil {
		return err
	}
	owner := intent.Ownership()
	if a.intentID != intent.IntentID() || a.intentVersion != intent.Version() || a.intentDigest != intent.Digest() ||
		a.userID != owner.UserID || a.walletBindingID != owner.WalletBindingID ||
		a.walletBindingVersion != owner.WalletBindingVersion || a.walletID != owner.WalletID ||
		a.walletAddress != owner.WalletAddress || a.chainID != owner.ChainID {
		return apperrors.New(apperrors.CodeApprovalRequired, "Approval does not match the intent and wallet binding.", false, true, true)
	}
	if at.IsZero() || !at.Before(a.expiresAt) {
		return apperrors.New(apperrors.CodeApprovalExpired, "Approval has expired.", false, true, true)
	}
	switch a.status {
	case StatusApproved:
		return nil
	case StatusRejected:
		return apperrors.New(apperrors.CodeApprovalRejected, "Approval was rejected.", false, true, true)
	case StatusConsumed:
		return apperrors.New(apperrors.CodeApprovalAlreadyConsumed, "Approval has already been consumed.", false, true, true)
	default:
		return apperrors.New(apperrors.CodeApprovalRequired, "Explicit approval is required.", false, true, false)
	}
}

func validateText(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maxFieldLength {
		return fmt.Errorf("%s exceeds %d characters", name, maxFieldLength)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s contains control characters", name)
	}
	return nil
}

func (a Approval) ApprovalID() string           { return a.approvalID }
func (a Approval) Version() uint64              { return a.version }
func (a Approval) ApprovalRequestID() string    { return a.approvalRequestID }
func (a Approval) IntentID() string             { return a.intentID }
func (a Approval) IntentVersion() uint64        { return a.intentVersion }
func (a Approval) IntentDigest() string         { return a.intentDigest }
func (a Approval) UserID() string               { return a.userID }
func (a Approval) WalletBindingID() string      { return a.walletBindingID }
func (a Approval) WalletBindingVersion() uint64 { return a.walletBindingVersion }
func (a Approval) WalletID() string             { return a.walletID }
func (a Approval) WalletAddress() string        { return a.walletAddress }
func (a Approval) ChainID() string              { return a.chainID }
func (a Approval) Status() Status               { return a.status }
func (a Approval) Decision() Decision           { return a.decision }
func (a Approval) CreatedAt() time.Time         { return a.createdAt }
func (a Approval) ExpiresAt() time.Time         { return a.expiresAt }
func (a Approval) DecidedAt() time.Time         { return a.decidedAt }
func (a Approval) ConsumedAt() time.Time        { return a.consumedAt }
func (a Approval) OperationKey() string         { return a.operationKey }
func (a Approval) OperationVersion() uint64     { return a.operationVersion }
