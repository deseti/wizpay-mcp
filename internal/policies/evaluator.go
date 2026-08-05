package policies

import (
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

// Evaluate applies the required pre-execution check to an approved intent.
func Evaluate(policy Policy, intent intents.Intent, identity auth.IdentityContext, binding wallet.Binding, at time.Time) (Result, error) {
	return evaluate(policy, intent, identity, binding, at, EvaluationStageBeforeExecution)
}

// EvaluateForApproval applies the Phase 0 pre-approval check to a frozen,
// CREATED intent. ALLOW never substitutes for explicit user approval.
func EvaluateForApproval(policy Policy, intent intents.Intent, identity auth.IdentityContext, binding wallet.Binding, at time.Time) (Result, error) {
	return evaluate(policy, intent, identity, binding, at, EvaluationStageBeforeApproval)
}

func evaluate(policy Policy, intent intents.Intent, identity auth.IdentityContext, binding wallet.Binding, at time.Time, stage EvaluationStage) (Result, error) {
	if err := policy.Validate(); err != nil {
		return Result{}, err
	}
	if err := intent.Validate(); err != nil {
		return Result{}, err
	}
	if at.IsZero() {
		return Result{}, invalidPolicy(fmt.Errorf("evaluation time is required"))
	}
	at = at.UTC()
	expectedStatus := intents.StatusApproved
	if stage == EvaluationStageBeforeApproval {
		expectedStatus = intents.StatusCreated
	}
	if intent.Status() != expectedStatus {
		if stage == EvaluationStageBeforeExecution {
			return Result{}, apperrors.New(apperrors.CodeApprovalRequired, "Approved intent is required for policy evaluation.", false, true, false)
		}
		return Result{}, invalidPolicy(fmt.Errorf("created intent is required for pre-approval policy evaluation"))
	}
	if !at.Before(intent.ExpiresAt()) || !at.Before(intent.Constraints().Deadline) {
		return Result{}, apperrors.New(apperrors.CodeIntentExpired, "Intent has expired.", false, true, true)
	}
	resolved := identity.Identity()
	if err := resolved.EnsureAuthorizable(); err != nil {
		return Result{}, err
	}
	if err := binding.EnsureAuthorizable(resolved.UserID()); err != nil {
		return Result{}, err
	}
	owner := intent.Ownership()
	if resolved.UserID() != owner.UserID || resolved.Provider() != owner.IdentityProvider ||
		binding.BindingID() != owner.WalletBindingID || binding.Version() != owner.WalletBindingVersion ||
		binding.Provider() != owner.IdentityProvider || binding.ProviderUserReference() != owner.ProviderUserReference ||
		binding.WalletID() != owner.WalletID || binding.Address() != owner.WalletAddress ||
		binding.ChainID() != owner.ChainID || binding.Network() != owner.Network {
		return Result{}, apperrors.New(apperrors.CodeWalletMismatch, "Identity and wallet context do not match the approved intent.", false, true, true)
	}

	result := Result{
		PolicyID: policy.PolicyID(), PolicyVersion: policy.Version(), IntentID: intent.IntentID(),
		IntentVersion: intent.Version(), IntentDigest: intent.Digest(), Stage: stage, Decision: DecisionAllow, EvaluatedAt: at,
	}
	if policy.Status() == StatusDisabled {
		return lifecycleDecision(result, ReasonPolicyDisabled), nil
	}
	if policy.Status() == StatusExpired || !at.Before(policy.ExpiresAt()) {
		return lifecycleDecision(result, ReasonPolicyExpired), nil
	}
	if policy.Status() != StatusActive {
		return lifecycleDecision(result, ReasonPolicyDraft), nil
	}
	if at.Before(policy.ValidFrom()) {
		return lifecycleDecision(result, ReasonPolicyNotEffective), nil
	}
	if intent.Constraints().PolicyReference != policy.Reference() {
		return denyDecision(result, ReasonPolicyReferenceMismatch), nil
	}
	scope := policy.Scope()
	if scope.UserID != owner.UserID || (scope.WalletBindingID != "" && scope.WalletBindingID != owner.WalletBindingID) || !scope.includes(intent.Type()) {
		return denyDecision(result, ReasonScopeMismatch), nil
	}

	view, err := newIntentView(intent)
	if err != nil {
		return Result{}, invalidPolicy(err)
	}
	for _, rule := range policy.Rules() {
		violated, reason, err := evaluateRule(rule, view, at)
		if err != nil {
			return Result{}, invalidPolicy(err)
		}
		if !violated {
			continue
		}
		result.Findings = append(result.Findings, Finding{RuleID: rule.RuleID, RuleType: rule.Type(), Decision: rule.OnViolation, Reason: reason})
		result.Decision = combineDecision(result.Decision, rule.OnViolation)
	}
	return result, nil
}

func lifecycleDecision(result Result, reason Reason) Result {
	result.Decision = DecisionDeny
	result.Findings = []Finding{{Decision: DecisionDeny, Reason: reason}}
	return result
}

func denyDecision(result Result, reason Reason) Result { return lifecycleDecision(result, reason) }

func combineDecision(current, next Decision) Decision {
	if current == DecisionDeny || next == DecisionDeny {
		return DecisionDeny
	}
	if current == DecisionRequireReview || next == DecisionRequireReview {
		return DecisionRequireReview
	}
	return DecisionAllow
}

// Error maps a completed decision to the safe public error taxonomy.
func (r Result) Error() error {
	if r.Decision == DecisionAllow {
		return nil
	}
	if len(r.Findings) > 0 {
		switch r.Findings[0].Reason {
		case ReasonPolicyExpired:
			return apperrors.New(apperrors.CodePolicyExpired, "Policy has expired.", false, true, true)
		case ReasonPolicyDisabled, ReasonPolicyDraft, ReasonPolicyNotEffective:
			return apperrors.New(apperrors.CodePolicyDisabled, "Policy is not active.", false, true, true)
		}
	}
	if r.Decision == DecisionRequireReview {
		return apperrors.New(apperrors.CodeReviewRequired, "Policy requires explicit review.", false, true, false)
	}
	return apperrors.New(apperrors.CodePolicyDenied, "Policy denied the intent.", false, true, true)
}

type intentView struct {
	kind              intents.Type
	spendToken        TokenReference
	spendAmount       intents.Amount
	chains            []string
	tokens            []TokenReference
	recipients        []string
	createdAt         time.Time
	effectiveDeadline time.Time
}

func newIntentView(intent intents.Intent) (intentView, error) {
	financial := intent.Financial()
	view := intentView{kind: intent.Type(), createdAt: intent.CreatedAt(), effectiveDeadline: intent.ExpiresAt()}
	if deadline := intent.Constraints().Deadline; deadline.Before(view.effectiveDeadline) {
		view.effectiveDeadline = deadline
	}
	switch intent.Type() {
	case intents.TypePayroll:
		source := financial.Payroll.SourceToken()
		view.spendToken, view.spendAmount = tokenReference(source), financial.Payroll.Total
		view.chains = []string{source.ChainID}
		view.tokens = []TokenReference{tokenReference(source)}
		for _, recipient := range financial.Payroll.Recipients {
			view.recipients = append(view.recipients, recipient.Address)
		}
	case intents.TypeSwap:
		view.spendToken, view.spendAmount = tokenReference(financial.Swap.InputToken), financial.Swap.InputAmount
		view.chains = []string{financial.Swap.InputToken.ChainID, financial.Swap.OutputToken.ChainID}
		view.tokens = []TokenReference{tokenReference(financial.Swap.InputToken), tokenReference(financial.Swap.OutputToken)}
	case intents.TypeBridge:
		view.spendToken, view.spendAmount = tokenReference(financial.Bridge.SourceToken), financial.Bridge.SourceAmount
		view.chains = []string{financial.Bridge.SourceChainID, financial.Bridge.DestinationChainID}
		view.tokens = []TokenReference{tokenReference(financial.Bridge.SourceToken)}
		view.recipients = []string{financial.Bridge.DestinationAddress}
	case intents.TypeANSRegistration:
		view.spendToken, view.spendAmount = tokenReference(financial.ANS.CostToken), financial.ANS.Cost
		view.chains = []string{financial.ANS.CostToken.ChainID}
		view.tokens = []TokenReference{tokenReference(financial.ANS.CostToken)}
		view.recipients = []string{financial.ANS.Controller}
	default:
		return intentView{}, fmt.Errorf("unsupported intent type %q", intent.Type())
	}
	view.chains = uniqueSorted(view.chains)
	view.recipients = uniqueSorted(view.recipients)
	sort.Slice(view.tokens, func(i, j int) bool { return view.tokens[i].key() < view.tokens[j].key() })
	uniqueTokens := view.tokens[:0]
	for _, token := range view.tokens {
		if len(uniqueTokens) == 0 || uniqueTokens[len(uniqueTokens)-1].key() != token.key() {
			uniqueTokens = append(uniqueTokens, token)
		}
	}
	view.tokens = uniqueTokens
	return view, nil
}

func uniqueSorted(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func evaluateRule(rule Rule, view intentView, at time.Time) (bool, Reason, error) {
	switch rule.Type() {
	case RuleTypeOperationAllowlist:
		return !containsIntentType(rule.OperationAllowlist.Allowed, view.kind), ReasonOperationNotAllowed, nil
	case RuleTypeSpendingLimit:
		if !containsIntentType(rule.SpendingLimit.IntentTypes, view.kind) {
			return false, "", nil
		}
		if rule.SpendingLimit.Token.key() != view.spendToken.key() {
			return true, ReasonTokenNotAllowed, nil
		}
		maximum, ok := new(big.Int).SetString(rule.SpendingLimit.Maximum.BaseUnits, 10)
		if !ok {
			return false, "", fmt.Errorf("invalid spending limit base units")
		}
		amount, ok := new(big.Int).SetString(view.spendAmount.BaseUnits, 10)
		if !ok {
			return false, "", fmt.Errorf("invalid intent amount base units")
		}
		return amount.Cmp(maximum) > 0, ReasonSpendingLimitExceeded, nil
	case RuleTypeChainAllowlist:
		return !allStringsAllowed(view.chains, rule.ChainAllowlist.Allowed), ReasonChainNotAllowed, nil
	case RuleTypeTokenAllowlist:
		return !allTokensAllowed(view.tokens, rule.TokenAllowlist.Allowed), ReasonTokenNotAllowed, nil
	case RuleTypeRecipient:
		return !allStringsAllowed(view.recipients, rule.Recipient.Allowed), ReasonRecipientNotAllowed, nil
	case RuleTypeExpiration:
		lifetime := view.effectiveDeadline.Sub(view.createdAt)
		remaining := view.effectiveDeadline.Sub(at)
		if rule.Expiration.MaxLifetimeSeconds > 0 && lifetime > time.Duration(rule.Expiration.MaxLifetimeSeconds)*time.Second {
			return true, ReasonExpirationConstraintViolated, nil
		}
		if rule.Expiration.MinimumRemainingSeconds > 0 && remaining < time.Duration(rule.Expiration.MinimumRemainingSeconds)*time.Second {
			return true, ReasonExpirationConstraintViolated, nil
		}
		return false, "", nil
	default:
		return false, "", fmt.Errorf("unsupported policy rule type %q", rule.Type())
	}
}

func containsIntentType(values []intents.Type, target intents.Type) bool {
	index := sort.Search(len(values), func(i int) bool { return values[i] >= target })
	return index < len(values) && values[index] == target
}

func allStringsAllowed(values, allowed []string) bool {
	for _, value := range values {
		index := sort.SearchStrings(allowed, value)
		if index >= len(allowed) || allowed[index] != value {
			return false
		}
	}
	return true
}

func allTokensAllowed(values, allowed []TokenReference) bool {
	for _, value := range values {
		index := sort.Search(len(allowed), func(i int) bool { return allowed[i].key() >= value.key() })
		if index >= len(allowed) || allowed[index].key() != value.key() {
			return false
		}
	}
	return true
}
