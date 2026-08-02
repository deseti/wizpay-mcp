package wallet

import "fmt"

// BindingStatus is independent from identity status. The absence of a binding
// record represents the unbound state.
type BindingStatus string

const (
	BindingStatusPending BindingStatus = "PENDING"
	BindingStatusActive  BindingStatus = "ACTIVE"
	BindingStatusRevoked BindingStatus = "REVOKED"
)

func (s BindingStatus) Valid() bool {
	switch s {
	case BindingStatusPending, BindingStatusActive, BindingStatusRevoked:
		return true
	default:
		return false
	}
}

func validateTransition(current, next BindingStatus) error {
	if !current.Valid() || !next.Valid() {
		return fmt.Errorf("invalid wallet binding state transition %s -> %s", current, next)
	}
	if current == next {
		return nil
	}

	allowed := false
	switch current {
	case BindingStatusPending:
		allowed = next == BindingStatusActive || next == BindingStatusRevoked
	case BindingStatusActive:
		allowed = next == BindingStatusRevoked
	case BindingStatusRevoked:
		allowed = false
	}
	if !allowed {
		return fmt.Errorf("wallet binding transition %s -> %s is not allowed", current, next)
	}
	return nil
}
