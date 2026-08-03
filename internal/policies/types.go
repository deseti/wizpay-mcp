// Package policies defines deterministic, provider-neutral authorization rules
// for approved intents. It performs no I/O and cannot execute financial actions.
package policies

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/deseti/wizpay-mcp/internal/intents"
)

const (
	maxPolicyTextLength = 256
	maxRules            = 100
)

// Decision is the complete policy-evaluation outcome set.
type Decision string

const (
	DecisionAllow         Decision = "ALLOW"
	DecisionDeny          Decision = "DENY"
	DecisionRequireReview Decision = "REQUIRE_REVIEW"
)

func (d Decision) Valid() bool {
	return d == DecisionAllow || d == DecisionDeny || d == DecisionRequireReview
}

func (d Decision) violationDecision() bool {
	return d == DecisionDeny || d == DecisionRequireReview
}

type EvaluationStage string

const (
	EvaluationStageBeforeApproval  EvaluationStage = "BEFORE_APPROVAL"
	EvaluationStageBeforeExecution EvaluationStage = "BEFORE_EXECUTION"
)

type RuleType string

const (
	RuleTypeSpendingLimit      RuleType = "SPENDING_LIMIT"
	RuleTypeOperationAllowlist RuleType = "OPERATION_ALLOWLIST"
	RuleTypeChainAllowlist     RuleType = "CHAIN_ALLOWLIST"
	RuleTypeTokenAllowlist     RuleType = "TOKEN_ALLOWLIST"
	RuleTypeRecipient          RuleType = "RECIPIENT_RULE"
	RuleTypeExpiration         RuleType = "EXPIRATION_RULE"
)

func (t RuleType) Valid() bool {
	switch t {
	case RuleTypeSpendingLimit, RuleTypeOperationAllowlist, RuleTypeChainAllowlist,
		RuleTypeTokenAllowlist, RuleTypeRecipient, RuleTypeExpiration:
		return true
	default:
		return false
	}
}

// Scope binds a policy to one identity and optionally one wallet binding and a
// subset of intent types. An empty IntentTypes slice means all known types.
type Scope struct {
	UserID          string
	WalletBindingID string
	IntentTypes     []intents.Type
}

func (s Scope) validate() error {
	if err := validateText("policy user ID", s.UserID); err != nil {
		return err
	}
	if s.WalletBindingID != "" {
		if err := validateText("policy wallet binding ID", s.WalletBindingID); err != nil {
			return err
		}
	}
	seen := make(map[intents.Type]struct{}, len(s.IntentTypes))
	for _, kind := range s.IntentTypes {
		if !kind.Valid() {
			return fmt.Errorf("invalid scoped intent type %q", kind)
		}
		if _, exists := seen[kind]; exists {
			return fmt.Errorf("duplicate scoped intent type %q", kind)
		}
		seen[kind] = struct{}{}
	}
	return nil
}

func normalizeScope(scope Scope) Scope {
	scope.UserID = strings.TrimSpace(scope.UserID)
	scope.WalletBindingID = strings.TrimSpace(scope.WalletBindingID)
	scope.IntentTypes = append([]intents.Type(nil), scope.IntentTypes...)
	sort.Slice(scope.IntentTypes, func(i, j int) bool { return scope.IntentTypes[i] < scope.IntentTypes[j] })
	return scope
}

func (s Scope) includes(kind intents.Type) bool {
	if len(s.IntentTypes) == 0 {
		return true
	}
	index := sort.Search(len(s.IntentTypes), func(i int) bool { return s.IntentTypes[i] >= kind })
	return index < len(s.IntentTypes) && s.IntentTypes[index] == kind
}

// TokenReference identifies policy authority by chain, standard, and address;
// symbols never determine token identity.
type TokenReference struct {
	ChainID  string
	Standard string
	Address  string
	Decimals uint8
}

func (t TokenReference) validate() error {
	if err := validateChainID(t.ChainID); err != nil {
		return err
	}
	if err := validateText("token standard", t.Standard); err != nil {
		return err
	}
	if err := validateText("token address", t.Address); err != nil {
		return err
	}
	if t.Decimals > 36 {
		return fmt.Errorf("token decimals cannot exceed 36")
	}
	return nil
}

func tokenReference(token intents.Token) TokenReference {
	return TokenReference{ChainID: token.ChainID, Standard: token.Standard, Address: token.Address, Decimals: token.Decimals}
}

func (t TokenReference) key() string {
	return t.ChainID + "\x00" + t.Standard + "\x00" + t.Address + "\x00" + strconv.Itoa(int(t.Decimals))
}

// Reason is a stable, non-sensitive explanation suitable for audit metadata.
type Reason string

const (
	ReasonPolicyDraft                  Reason = "POLICY_DRAFT"
	ReasonPolicyNotEffective           Reason = "POLICY_NOT_EFFECTIVE"
	ReasonPolicyDisabled               Reason = "POLICY_DISABLED"
	ReasonPolicyExpired                Reason = "POLICY_EXPIRED"
	ReasonPolicyReferenceMismatch      Reason = "POLICY_REFERENCE_MISMATCH"
	ReasonScopeMismatch                Reason = "SCOPE_MISMATCH"
	ReasonOperationNotAllowed          Reason = "OPERATION_NOT_ALLOWED"
	ReasonSpendingLimitExceeded        Reason = "SPENDING_LIMIT_EXCEEDED"
	ReasonChainNotAllowed              Reason = "CHAIN_NOT_ALLOWED"
	ReasonTokenNotAllowed              Reason = "TOKEN_NOT_ALLOWED"
	ReasonRecipientNotAllowed          Reason = "RECIPIENT_NOT_ALLOWED"
	ReasonExpirationConstraintViolated Reason = "EXPIRATION_CONSTRAINT_VIOLATED"
)

type Finding struct {
	RuleID   string
	RuleType RuleType
	Decision Decision
	Reason   Reason
}

// Result binds a deterministic decision to exact policy and intent versions.
type Result struct {
	PolicyID      string
	PolicyVersion uint64
	IntentID      string
	IntentVersion uint64
	IntentDigest  string
	Stage         EvaluationStage
	Decision      Decision
	EvaluatedAt   time.Time
	Findings      []Finding
}

// Applicability is the typed lookup input for a future policy repository.
type Applicability struct {
	UserID          string
	WalletBindingID string
	IntentType      intents.Type
	EvaluatedAt     time.Time
}

func validateText(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is required and must not contain surrounding whitespace", name)
	}
	if len(value) > maxPolicyTextLength {
		return fmt.Errorf("%s exceeds %d characters", name, maxPolicyTextLength)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s contains control characters", name)
	}
	return nil
}

func validateChainID(value string) error {
	if value == "" || len(value) > 20 || value[0] == '0' {
		return fmt.Errorf("chain ID must be a canonical positive decimal string")
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return fmt.Errorf("chain ID must be a canonical positive decimal string")
		}
	}
	return nil
}
