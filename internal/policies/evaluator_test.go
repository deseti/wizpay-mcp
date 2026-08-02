package policies

import (
	"reflect"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func evaluatePayroll(t *testing.T, policy Policy, amount, baseUnits string) Result {
	t.Helper()
	identity, binding := evaluationContext(t)
	intent := approvedPayrollIntent(t, policy.Reference(), amount, baseUnits)
	result, err := Evaluate(policy, intent, identity, binding, policyTestNow.Add(10*time.Second))
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	return result
}

func TestEvaluateAllowsIntentWithinSpendingLimit(t *testing.T) {
	policy := mustActivePolicy(t, []Rule{spendingRule("spending", "500", DecisionDeny)})
	result := evaluatePayroll(t, policy, "100", "100000000")
	if result.Decision != DecisionAllow || len(result.Findings) != 0 || result.Error() != nil {
		t.Fatalf("result = %+v", result)
	}
}

func TestEvaluateForApprovalAllowsWithoutBypassingApproval(t *testing.T) {
	policy := mustActivePolicy(t, []Rule{spendingRule("spending", "500", DecisionDeny)})
	identity, binding := evaluationContext(t)
	intent := createdPayrollIntent(t, policy.Reference(), "100", "100000000")
	result, err := EvaluateForApproval(policy, intent, identity, binding, policyTestNow.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionAllow || result.Stage != EvaluationStageBeforeApproval {
		t.Fatalf("result = %+v", result)
	}
	if intent.Status() != "CREATED" {
		t.Fatalf("policy evaluation changed intent status to %s", intent.Status())
	}
}

func TestEvaluateDeniesIntentAboveSpendingLimit(t *testing.T) {
	policy := mustActivePolicy(t, []Rule{spendingRule("spending", "50", DecisionDeny)})
	result := evaluatePayroll(t, policy, "100", "100000000")
	if result.Decision != DecisionDeny || len(result.Findings) != 1 || result.Findings[0].Reason != ReasonSpendingLimitExceeded {
		t.Fatalf("result = %+v", result)
	}
	if !policyErrorCode(result.Error(), apperrors.CodePolicyDenied) {
		t.Fatalf("result error = %v", result.Error())
	}
}

func TestEvaluateRequiresReview(t *testing.T) {
	policy := mustActivePolicy(t, []Rule{spendingRule("spending", "50", DecisionRequireReview)})
	result := evaluatePayroll(t, policy, "100", "100000000")
	if result.Decision != DecisionRequireReview {
		t.Fatalf("decision = %s", result.Decision)
	}
	if !policyErrorCode(result.Error(), apperrors.CodeReviewRequired) {
		t.Fatalf("result error = %v", result.Error())
	}
}

func TestEvaluateMultipleRulesUsesDeterministicDenyPrecedence(t *testing.T) {
	policy := mustActivePolicy(t, []Rule{
		spendingRule("z_spending", "50", DecisionDeny),
		{RuleID: "a_recipient", OnViolation: DecisionRequireReview, Recipient: &RecipientRule{Allowed: []string{"0x9999999999999999999999999999999999999999"}}},
		{RuleID: "m_chain", OnViolation: DecisionDeny, ChainAllowlist: &ChainAllowlistRule{Allowed: []string{"5042002"}}},
	})
	identity, binding := evaluationContext(t)
	intent := approvedPayrollIntent(t, policy.Reference(), "100", "100000000")
	at := policyTestNow.Add(10 * time.Second)
	first, err := Evaluate(policy, intent, identity, binding, at)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Evaluate(policy, intent, identity, binding, at)
	if err != nil {
		t.Fatal(err)
	}
	if first.Decision != DecisionDeny || len(first.Findings) != 2 {
		t.Fatalf("result = %+v", first)
	}
	if first.Findings[0].RuleID != "a_recipient" || first.Findings[1].RuleID != "z_spending" {
		t.Fatalf("finding order = %+v", first.Findings)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("evaluation is not deterministic:\n%+v\n%+v", first, second)
	}
}

func TestDisabledAndExpiredPoliciesCannotAuthorize(t *testing.T) {
	active := mustActivePolicy(t, []Rule{spendingRule("spending", "500", DecisionDeny)})
	disabled, err := active.Transition(StatusDisabled, policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	result := evaluatePayroll(t, disabled, "100", "100000000")
	if result.Decision != DecisionDeny || !policyErrorCode(result.Error(), apperrors.CodePolicyDisabled) {
		t.Fatalf("disabled result = %+v, error = %v", result, result.Error())
	}

	params := policyParams([]Rule{spendingRule("spending", "500", DecisionDeny)})
	params.ExpiresAt = policyTestNow.Add(6 * time.Second)
	expiring, err := NewDraft(params)
	if err != nil {
		t.Fatal(err)
	}
	expiring, err = expiring.Transition(StatusActive, policyTestNow.Add(-30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	result = evaluatePayroll(t, expiring, "100", "100000000")
	if result.Decision != DecisionDeny || !policyErrorCode(result.Error(), apperrors.CodePolicyExpired) {
		t.Fatalf("expired result = %+v, error = %v", result, result.Error())
	}
}
