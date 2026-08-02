// Package audit defines event names and metadata contracts only. Phase 5 does
// not persist or emit audit records.
package audit

import "time"

type EventType string

const (
	EventIntentCreated             EventType = "intent_created"
	EventIntentApprovalRequired    EventType = "intent_approval_required"
	EventIntentApproved            EventType = "intent_approved"
	EventIntentReady               EventType = "intent_ready_for_execution"
	EventIntentExpired             EventType = "intent_expired"
	EventIntentCancelled           EventType = "intent_cancelled"
	EventApprovalRequested         EventType = "approval_requested"
	EventApprovalGranted           EventType = "approval_granted"
	EventApprovalRejected          EventType = "approval_rejected"
	EventApprovalExpired           EventType = "approval_expired"
	EventApprovalConsumed          EventType = "approval_consumed"
	EventPolicyEvaluated           EventType = "policy_evaluated"
	EventPolicyAllowed             EventType = "policy_allowed"
	EventPolicyDenied              EventType = "policy_denied"
	EventPolicyReviewRequired      EventType = "policy_review_required"
	EventExecutionCreated          EventType = "execution_created"
	EventExecutionAuthorized       EventType = "execution_authorized"
	EventExecutionQueued           EventType = "execution_queued"
	EventExecutionSubmitted        EventType = "execution_submitted"
	EventExecutionConfirming       EventType = "execution_confirming"
	EventExecutionConfirmed        EventType = "execution_confirmed"
	EventExecutionVerified         EventType = "execution_verified"
	EventExecutionCompleted        EventType = "execution_completed"
	EventExecutionFailed           EventType = "execution_failed"
	EventExecutionRecoveryRequired EventType = "execution_recovery_required"
	EventExecutionRecovered        EventType = "execution_recovered"
	EventExecutionCancelled        EventType = "execution_cancelled"
)

// Event is a provider-neutral reference envelope. Details intentionally remain
// typed references rather than arbitrary payload maps.
type Event struct {
	EventID              string
	Type                 EventType
	OccurredAt           time.Time
	IntentID             string
	IntentVersion        uint64
	IntentDigest         string
	ApprovalID           string
	PolicyID             string
	PolicyVersion        uint64
	PolicyDecision       string
	PolicyEvaluationKey  string
	ExecutionID          string
	ExecutionRevision    uint64
	ExecutionRequestID   string
	ExecutionRequestKey  string
	ExecutionStatus      string
	RecoveryReasonCode   string
	WalletBindingID      string
	WalletBindingVersion uint64
	UserID               string
	OperationKey         string
	OperationVersion     uint64
}
