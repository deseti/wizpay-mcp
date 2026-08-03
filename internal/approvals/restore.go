package approvals

import "time"

type RestoreParams struct {
	ApprovalID           string
	Version              uint64
	ApprovalRequestID    string
	IntentID             string
	IntentVersion        uint64
	IntentDigest         string
	UserID               string
	WalletBindingID      string
	WalletBindingVersion uint64
	WalletID             string
	WalletAddress        string
	ChainID              string
	Status               Status
	Decision             Decision
	CreatedAt            time.Time
	ExpiresAt            time.Time
	DecidedAt            time.Time
	ConsumedAt           time.Time
	OperationKey         string
	OperationVersion     uint64
	LifecycleRevision    uint64
}

// Restore reconstructs and validates an approval artifact from durable fields.
func Restore(p RestoreParams) (Approval, error) {
	value := Approval{approvalID: p.ApprovalID, version: p.Version, approvalRequestID: p.ApprovalRequestID, intentID: p.IntentID, intentVersion: p.IntentVersion, intentDigest: p.IntentDigest, userID: p.UserID, walletBindingID: p.WalletBindingID, walletBindingVersion: p.WalletBindingVersion, walletID: p.WalletID, walletAddress: p.WalletAddress, chainID: p.ChainID, status: p.Status, decision: p.Decision, createdAt: p.CreatedAt.UTC(), expiresAt: p.ExpiresAt.UTC(), decidedAt: p.DecidedAt.UTC(), consumedAt: p.ConsumedAt.UTC(), operationKey: p.OperationKey, operationVersion: p.OperationVersion, lifecycleRevision: p.LifecycleRevision}
	if err := value.Validate(); err != nil {
		return Approval{}, err
	}
	return value, nil
}
