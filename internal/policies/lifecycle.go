package policies

import (
	"fmt"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

type Status string

const (
	StatusDraft    Status = "DRAFT"
	StatusActive   Status = "ACTIVE"
	StatusDisabled Status = "DISABLED"
	StatusExpired  Status = "EXPIRED"
)

func (s Status) Valid() bool {
	return s == StatusDraft || s == StatusActive || s == StatusDisabled || s == StatusExpired
}
func (s Status) Terminal() bool { return s == StatusDisabled || s == StatusExpired }

// Transition returns a new policy. Repeated delivery is idempotent; DISABLED
// and EXPIRED are terminal, and expiration cannot be recorded early.
func (p Policy) Transition(next Status, at time.Time) (Policy, error) {
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	if next == p.status {
		return p, nil
	}
	if !next.Valid() {
		return Policy{}, invalidPolicy(fmt.Errorf("invalid policy status %q", next))
	}
	if at.IsZero() || at.Before(p.createdAt) {
		return Policy{}, invalidPolicy(fmt.Errorf("policy transition time is invalid"))
	}
	allowed := false
	switch p.status {
	case StatusDraft:
		allowed = next == StatusActive || next == StatusDisabled || next == StatusExpired
	case StatusActive:
		allowed = next == StatusDisabled || next == StatusExpired
	}
	if !allowed {
		return Policy{}, invalidPolicy(fmt.Errorf("policy transition %s -> %s is not allowed", p.status, next))
	}
	if next == StatusExpired {
		if at.Before(p.expiresAt) {
			return Policy{}, invalidPolicy(fmt.Errorf("policy has not reached its expiration time"))
		}
	} else if !at.Before(p.expiresAt) {
		return Policy{}, apperrors.New(apperrors.CodePolicyExpired, "Policy has expired.", false, true, true)
	}
	if next == StatusActive && at.Before(p.validFrom) {
		return Policy{}, invalidPolicy(fmt.Errorf("policy cannot activate before its validity window"))
	}
	nextPolicy := p
	nextPolicy.status = next
	return nextPolicy, nextPolicy.Validate()
}
