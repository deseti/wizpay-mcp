package auth

import "fmt"

// Permission is a closed capability-access decision. It is not financial
// approval or execution authority.
type Permission string

const (
	PermissionCreateIntent     Permission = "intent:create"
	PermissionReadIntent       Permission = "intent:read"
	PermissionRequestApproval  Permission = "approval:request"
	PermissionReadApproval     Permission = "approval:read"
	PermissionEvaluatePolicy   Permission = "policy:evaluate"
	PermissionPrepareExecution Permission = "execution:prepare"
	PermissionAutonomyRead     Permission = "autonomy:read"
	PermissionAutonomyControl  Permission = "autonomy:control"
)

func (p Permission) Valid() bool {
	switch p {
	case PermissionCreateIntent, PermissionReadIntent, PermissionRequestApproval,
		PermissionReadApproval, PermissionEvaluatePolicy, PermissionPrepareExecution,
		PermissionAutonomyRead, PermissionAutonomyControl:
		return true
	default:
		return false
	}
}

func validatePermission(p Permission) error {
	if !p.Valid() {
		return fmt.Errorf("unsupported permission %q", p)
	}
	return nil
}
