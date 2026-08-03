package policies

import (
	"fmt"
	"sort"
	"strings"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

type Params struct {
	PolicyID  string
	Version   uint64
	Name      string
	Scope     Scope
	Rules     []Rule
	CreatedAt time.Time
	ValidFrom time.Time
	ExpiresAt time.Time
}

// Policy is an immutable value. Lifecycle transitions return a new value and
// active rule content cannot be replaced in place.
type Policy struct {
	policyID          string
	version           uint64
	name              string
	scope             Scope
	rules             []Rule
	status            Status
	createdAt         time.Time
	validFrom         time.Time
	expiresAt         time.Time
	lifecycleRevision uint64
}

func NewDraft(params Params) (Policy, error) {
	params.PolicyID = strings.TrimSpace(params.PolicyID)
	params.Name = strings.TrimSpace(params.Name)
	params.Scope = normalizeScope(params.Scope)
	params.Rules = cloneRules(params.Rules)
	sort.Slice(params.Rules, func(i, j int) bool { return params.Rules[i].RuleID < params.Rules[j].RuleID })
	params.CreatedAt = params.CreatedAt.UTC()
	params.ValidFrom = params.ValidFrom.UTC()
	params.ExpiresAt = params.ExpiresAt.UTC()
	policy := Policy{
		policyID: params.PolicyID, version: params.Version, name: params.Name, scope: params.Scope,
		rules: params.Rules, status: StatusDraft, createdAt: params.CreatedAt,
		validFrom: params.ValidFrom, expiresAt: params.ExpiresAt, lifecycleRevision: 1,
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func (p Policy) Validate() error {
	if err := validateText("policy ID", p.policyID); err != nil {
		return invalidPolicy(err)
	}
	if err := validateText("policy name", p.name); err != nil {
		return invalidPolicy(err)
	}
	if p.version == 0 {
		return invalidPolicy(fmt.Errorf("policy version must be at least 1"))
	}
	if p.lifecycleRevision == 0 {
		return invalidPolicy(fmt.Errorf("policy lifecycle revision must be at least 1"))
	}
	if err := p.scope.validate(); err != nil {
		return invalidPolicy(err)
	}
	if len(p.rules) == 0 || len(p.rules) > maxRules {
		return invalidPolicy(fmt.Errorf("policy requires 1 to %d rules", maxRules))
	}
	seen := make(map[string]struct{}, len(p.rules))
	previous := ""
	for _, rule := range p.rules {
		if err := rule.validate(); err != nil {
			return invalidPolicy(fmt.Errorf("rule %q: %w", rule.RuleID, err))
		}
		if _, exists := seen[rule.RuleID]; exists {
			return invalidPolicy(fmt.Errorf("duplicate rule ID %q", rule.RuleID))
		}
		if previous != "" && previous > rule.RuleID {
			return invalidPolicy(fmt.Errorf("policy rules are not in canonical order"))
		}
		seen[rule.RuleID], previous = struct{}{}, rule.RuleID
	}
	if !p.status.Valid() {
		return invalidPolicy(fmt.Errorf("invalid policy status %q", p.status))
	}
	if p.createdAt.IsZero() || p.validFrom.IsZero() || p.expiresAt.IsZero() ||
		p.validFrom.Before(p.createdAt) || !p.expiresAt.After(p.validFrom) {
		return invalidPolicy(fmt.Errorf("policy validity window is invalid"))
	}
	return nil
}

func invalidPolicy(cause error) error {
	return apperrors.Wrap(apperrors.CodePolicyInvalid, "Policy is invalid.", false, true, true, cause)
}

// Reference is the immutable policy identity stored in an intent constraint.
func (p Policy) Reference() string { return fmt.Sprintf("%s:%d", p.policyID, p.version) }

func (p Policy) PolicyID() string          { return p.policyID }
func (p Policy) Version() uint64           { return p.version }
func (p Policy) Name() string              { return p.name }
func (p Policy) Scope() Scope              { return normalizeScope(p.scope) }
func (p Policy) Rules() []Rule             { return cloneRules(p.rules) }
func (p Policy) Status() Status            { return p.status }
func (p Policy) CreatedAt() time.Time      { return p.createdAt }
func (p Policy) ValidFrom() time.Time      { return p.validFrom }
func (p Policy) ExpiresAt() time.Time      { return p.expiresAt }
func (p Policy) LifecycleRevision() uint64 { return p.lifecycleRevision }
