// Package audit defines event names and metadata contracts only. Phase 4 does
// not persist or emit audit records.
package audit

import "time"

type EventType string

const (
	EventIntentCreated          EventType = "intent_created"
	EventIntentApprovalRequired EventType = "intent_approval_required"
	EventIntentApproved         EventType = "intent_approved"
	EventIntentReady            EventType = "intent_ready_for_execution"
	EventIntentExpired          EventType = "intent_expired"
	EventIntentCancelled        EventType = "intent_cancelled"
	EventApprovalRequested      EventType = "approval_requested"
	EventApprovalGranted        EventType = "approval_granted"
	EventApprovalRejected       EventType = "approval_rejected"
	EventApprovalExpired        EventType = "approval_expired"
	EventApprovalConsumed       EventType = "approval_consumed"
	EventPolicyEvaluated        EventType = "policy_evaluated"
	EventPolicyAllowed          EventType = "policy_allowed"
	EventPolicyDenied           EventType = "policy_denied"
	EventPolicyReviewRequired   EventType = "policy_review_required"
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
	WalletBindingID      string
	WalletBindingVersion uint64
	UserID               string
	OperationKey         string
	OperationVersion     uint64
}
