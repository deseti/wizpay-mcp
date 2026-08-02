package policies

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/intents"
)

type SpendingLimitRule struct {
	IntentTypes []intents.Type
	Token       TokenReference
	Maximum     intents.Amount
}

type OperationAllowlistRule struct{ Allowed []intents.Type }
type ChainAllowlistRule struct{ Allowed []string }
type TokenAllowlistRule struct{ Allowed []TokenReference }
type RecipientRule struct{ Allowed []string }

type ExpirationRule struct {
	MaxLifetimeSeconds      uint64
	MinimumRemainingSeconds uint64
}

// Rule is a closed discriminated union. Exactly one typed rule body is required.
type Rule struct {
	RuleID             string
	OnViolation        Decision
	SpendingLimit      *SpendingLimitRule
	OperationAllowlist *OperationAllowlistRule
	ChainAllowlist     *ChainAllowlistRule
	TokenAllowlist     *TokenAllowlistRule
	Recipient          *RecipientRule
	Expiration         *ExpirationRule
}

func (r Rule) Type() RuleType {
	switch {
	case r.SpendingLimit != nil:
		return RuleTypeSpendingLimit
	case r.OperationAllowlist != nil:
		return RuleTypeOperationAllowlist
	case r.ChainAllowlist != nil:
		return RuleTypeChainAllowlist
	case r.TokenAllowlist != nil:
		return RuleTypeTokenAllowlist
	case r.Recipient != nil:
		return RuleTypeRecipient
	case r.Expiration != nil:
		return RuleTypeExpiration
	default:
		return ""
	}
}

func (r Rule) validate() error {
	if err := validateText("rule ID", r.RuleID); err != nil {
		return err
	}
	if !r.OnViolation.violationDecision() {
		return fmt.Errorf("rule violation decision must be DENY or REQUIRE_REVIEW")
	}
	count := 0
	for _, present := range []bool{r.SpendingLimit != nil, r.OperationAllowlist != nil, r.ChainAllowlist != nil, r.TokenAllowlist != nil, r.Recipient != nil, r.Expiration != nil} {
		if present {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("rule must contain exactly one typed rule body")
	}
	switch r.Type() {
	case RuleTypeSpendingLimit:
		return r.SpendingLimit.validate()
	case RuleTypeOperationAllowlist:
		return validateIntentTypes(r.OperationAllowlist.Allowed, false)
	case RuleTypeChainAllowlist:
		return validateChains(r.ChainAllowlist.Allowed)
	case RuleTypeTokenAllowlist:
		return validateTokens(r.TokenAllowlist.Allowed)
	case RuleTypeRecipient:
		return validateStrings("recipient", r.Recipient.Allowed)
	case RuleTypeExpiration:
		if r.Expiration.MaxLifetimeSeconds == 0 && r.Expiration.MinimumRemainingSeconds == 0 {
			return fmt.Errorf("expiration rule requires at least one positive constraint")
		}
		const maxDurationSeconds = uint64((1<<63 - 1) / int64(1_000_000_000))
		if r.Expiration.MaxLifetimeSeconds > maxDurationSeconds || r.Expiration.MinimumRemainingSeconds > maxDurationSeconds {
			return fmt.Errorf("expiration rule duration exceeds supported range")
		}
		return nil
	default:
		return fmt.Errorf("invalid rule type")
	}
}

func (r SpendingLimitRule) validate() error {
	if err := validateIntentTypes(r.IntentTypes, false); err != nil {
		return err
	}
	if err := r.Token.validate(); err != nil {
		return err
	}
	if err := r.Maximum.Validate(); err != nil {
		return err
	}
	if r.Maximum.BaseUnits == "0" {
		return fmt.Errorf("spending limit must be positive")
	}
	if r.Maximum.Decimals != r.Token.Decimals {
		return fmt.Errorf("spending limit decimals do not match token")
	}
	return nil
}

func validateIntentTypes(values []intents.Type, allowEmpty bool) error {
	if !allowEmpty && len(values) == 0 {
		return fmt.Errorf("intent type list cannot be empty")
	}
	seen := make(map[intents.Type]struct{}, len(values))
	for _, value := range values {
		if !value.Valid() {
			return fmt.Errorf("invalid intent type %q", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate intent type %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateChains(values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("chain allowlist cannot be empty")
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateChainID(value); err != nil {
			return err
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate chain %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateTokens(values []TokenReference) error {
	if len(values) == 0 {
		return fmt.Errorf("token allowlist cannot be empty")
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := value.validate(); err != nil {
			return err
		}
		if _, exists := seen[value.key()]; exists {
			return fmt.Errorf("duplicate token reference")
		}
		seen[value.key()] = struct{}{}
	}
	return nil
}

func validateStrings(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s allowlist cannot be empty", name)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateText(name, value); err != nil {
			return err
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate %s %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func normalizeRule(rule Rule) Rule {
	rule.RuleID = strings.TrimSpace(rule.RuleID)
	if rule.SpendingLimit != nil {
		value := *rule.SpendingLimit
		value.IntentTypes = append([]intents.Type(nil), value.IntentTypes...)
		sort.Slice(value.IntentTypes, func(i, j int) bool { return value.IntentTypes[i] < value.IntentTypes[j] })
		rule.SpendingLimit = &value
	}
	if rule.OperationAllowlist != nil {
		value := *rule.OperationAllowlist
		value.Allowed = append([]intents.Type(nil), value.Allowed...)
		sort.Slice(value.Allowed, func(i, j int) bool { return value.Allowed[i] < value.Allowed[j] })
		rule.OperationAllowlist = &value
	}
	if rule.ChainAllowlist != nil {
		value := *rule.ChainAllowlist
		value.Allowed = append([]string(nil), value.Allowed...)
		sort.Strings(value.Allowed)
		rule.ChainAllowlist = &value
	}
	if rule.TokenAllowlist != nil {
		value := *rule.TokenAllowlist
		value.Allowed = append([]TokenReference(nil), value.Allowed...)
		sort.Slice(value.Allowed, func(i, j int) bool { return value.Allowed[i].key() < value.Allowed[j].key() })
		rule.TokenAllowlist = &value
	}
	if rule.Recipient != nil {
		value := *rule.Recipient
		value.Allowed = append([]string(nil), value.Allowed...)
		sort.Strings(value.Allowed)
		rule.Recipient = &value
	}
	if rule.Expiration != nil {
		value := *rule.Expiration
		rule.Expiration = &value
	}
	return rule
}

func cloneRules(rules []Rule) []Rule {
	result := make([]Rule, len(rules))
	for index, rule := range rules {
		result[index] = normalizeRule(rule)
	}
	return result
}
