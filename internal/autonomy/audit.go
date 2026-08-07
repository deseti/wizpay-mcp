package autonomy

// Stable event names are intentionally strings so audit storage can accept
// them without importing the autonomous runtime's mutable state.
type AuditEvent string

const (
	AuditScheduleCreated    AuditEvent = "autonomy.schedule.created"
	AuditScheduleVersioned  AuditEvent = "autonomy.schedule.versioned"
	AuditSchedulePaused     AuditEvent = "autonomy.schedule.paused"
	AuditScheduleResumed    AuditEvent = "autonomy.schedule.resumed"
	AuditScheduleRevoked    AuditEvent = "autonomy.schedule.revoked"
	AuditOccurrenceCreated  AuditEvent = "autonomy.occurrence.created"
	AuditOccurrenceClaimed  AuditEvent = "autonomy.occurrence.claimed"
	AuditOccurrenceSkipped  AuditEvent = "autonomy.occurrence.skipped"
	AuditPolicyAllowed      AuditEvent = "autonomy.policy.allowed"
	AuditPolicyDenied       AuditEvent = "autonomy.policy.denied"
	AuditStepUpRequired     AuditEvent = "autonomy.step_up.required"
	AuditDelegationUsed     AuditEvent = "autonomy.delegation.used"
	AuditDelegationRejected AuditEvent = "autonomy.delegation.rejected"
	AuditEmergencyStop      AuditEvent = "autonomy.emergency_stop.changed"
	AuditDispatchBlocked    AuditEvent = "autonomy.dispatch.blocked"
	AuditDispatchPrepared   AuditEvent = "autonomy.dispatch.prepared"
	AuditDispatchReconcile  AuditEvent = "autonomy.dispatch.reconcile"
)

type Attribution struct{ TenantID, UserID, AgentID, DelegationID, WalletBindingID, ScheduleID, OccurrenceID, IntentID, GrantID, ExecutionID string }
