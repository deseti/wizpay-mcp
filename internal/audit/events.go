// Package audit defines safe append-only audit contracts.
package audit

import "time"

type EventType string

const (
	EventIntentCreated              EventType = "intent_created"
	EventIntentApprovalRequired     EventType = "intent_approval_required"
	EventIntentApproved             EventType = "intent_approved"
	EventIntentReady                EventType = "intent_ready_for_execution"
	EventIntentExpired              EventType = "intent_expired"
	EventIntentCancelled            EventType = "intent_cancelled"
	EventApprovalRequested          EventType = "approval_requested"
	EventApprovalGranted            EventType = "approval_granted"
	EventApprovalRejected           EventType = "approval_rejected"
	EventApprovalExpired            EventType = "approval_expired"
	EventApprovalConsumed           EventType = "approval_consumed"
	EventPolicyEvaluated            EventType = "policy_evaluated"
	EventPolicyAllowed              EventType = "policy_allowed"
	EventPolicyDenied               EventType = "policy_denied"
	EventPolicyReviewRequired       EventType = "policy_review_required"
	EventExecutionCreated           EventType = "execution_created"
	EventExecutionAuthorized        EventType = "execution_authorized"
	EventExecutionQueued            EventType = "execution_queued"
	EventExecutionExecuting         EventType = "execution_executing"
	EventExecutionSubmitted         EventType = "execution_submitted"
	EventExecutionConfirming        EventType = "execution_confirming"
	EventExecutionConfirmed         EventType = "execution_confirmed"
	EventExecutionVerified          EventType = "execution_verified"
	EventExecutionCompleted         EventType = "execution_completed"
	EventExecutionFailed            EventType = "execution_failed"
	EventExecutionRecoveryRequired  EventType = "execution_recovery_required"
	EventExecutionRecovered         EventType = "execution_recovered"
	EventExecutionCancelled         EventType = "execution_cancelled"
	EventAutonomyScheduleCreated    EventType = "autonomy_schedule_created"
	EventAutonomyScheduleVersioned  EventType = "autonomy_schedule_versioned"
	EventAutonomySchedulePaused     EventType = "autonomy_schedule_paused"
	EventAutonomyScheduleResumed    EventType = "autonomy_schedule_resumed"
	EventAutonomyScheduleRevoked    EventType = "autonomy_schedule_revoked"
	EventAutonomyOccurrenceCreated  EventType = "autonomy_occurrence_created"
	EventAutonomyOccurrenceClaimed  EventType = "autonomy_occurrence_claimed"
	EventAutonomyOccurrenceSkipped  EventType = "autonomy_occurrence_skipped"
	EventAutonomyPolicyAllowed      EventType = "autonomy_policy_allowed"
	EventAutonomyPolicyDenied       EventType = "autonomy_policy_denied"
	EventAutonomyStepUpRequired     EventType = "autonomy_step_up_required"
	EventAutonomyDelegationUsed     EventType = "autonomy_delegation_used"
	EventAutonomyDelegationRejected EventType = "autonomy_delegation_rejected"
	EventAutonomyEmergencyStop      EventType = "autonomy_emergency_stop_changed"
	EventAutonomyDispatchBlocked    EventType = "autonomy_dispatch_blocked"
	EventAutonomyDispatchPrepared   EventType = "autonomy_dispatch_prepared"
	EventAutonomyDispatchReconcile  EventType = "autonomy_dispatch_reconcile"
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

// Record adds tenant-independent actor and correlation context to a typed
// event. Metadata is intentionally an allowlisted struct rather than an
// arbitrary map that could capture secrets or unsafe errors.
type Record struct {
	Event           Event
	ActorType       string
	ActorID         string
	RequestID       string
	TraceID         string
	ResourceType    string
	ResourceID      string
	PreviousState   string
	NewState        string
	SafeReasonCode  string
	SourceComponent string
}
