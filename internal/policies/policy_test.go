package policies

import (
	stderrors "errors"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestValidPolicyCreationCanonicalizesTypedRules(t *testing.T) {
	rules := []Rule{
		{RuleID: "token", OnViolation: DecisionDeny, TokenAllowlist: &TokenAllowlistRule{Allowed: []TokenReference{policyTestTokenReference()}}},
		{RuleID: "operation", OnViolation: DecisionDeny, OperationAllowlist: &OperationAllowlistRule{Allowed: []intents.Type{intents.TypePayroll}}},
		{RuleID: "chain", OnViolation: DecisionDeny, ChainAllowlist: &ChainAllowlistRule{Allowed: []string{"5042002"}}},
		spendingRule("spending", "500", DecisionDeny),
		{RuleID: "recipient", OnViolation: DecisionRequireReview, Recipient: &RecipientRule{Allowed: []string{"0x3333333333333333333333333333333333333333"}}},
		{RuleID: "expiration", OnViolation: DecisionDeny, Expiration: &ExpirationRule{MaxLifetimeSeconds: 3600, MinimumRemainingSeconds: 60}},
	}
	policy := mustPolicy(t, rules)
	if policy.Status() != StatusDraft || policy.Reference() != "policy_001:1" {
		t.Fatalf("policy = (%s, %s)", policy.Status(), policy.Reference())
	}
	got := policy.Rules()
	for index := 1; index < len(got); index++ {
		if got[index-1].RuleID > got[index].RuleID {
			t.Fatal("rules are not canonically ordered")
		}
	}
}

func TestInvalidPolicyRulesAreRejected(t *testing.T) {
	tests := []struct {
		name  string
		rules []Rule
	}{
		{"no rules", nil},
		{"allow on violation", []Rule{{RuleID: "operation", OnViolation: DecisionAllow, OperationAllowlist: &OperationAllowlistRule{Allowed: []intents.Type{intents.TypePayroll}}}}},
		{"multiple bodies", []Rule{{RuleID: "mixed", OnViolation: DecisionDeny, OperationAllowlist: &OperationAllowlistRule{Allowed: []intents.Type{intents.TypePayroll}}, ChainAllowlist: &ChainAllowlistRule{Allowed: []string{"5042002"}}}}},
		{"empty allowlist", []Rule{{RuleID: "chain", OnViolation: DecisionDeny, ChainAllowlist: &ChainAllowlistRule{}}}},
		{"duplicate IDs", []Rule{spendingRule("duplicate", "10", DecisionDeny), spendingRule("duplicate", "20", DecisionDeny)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewDraft(policyParams(test.rules))
			if !policyErrorCode(err, apperrors.CodePolicyInvalid) {
				t.Fatalf("error = %v, want policy_invalid", err)
			}
		})
	}
}

func TestPolicyLifecycleTransitionsAndTerminalBehavior(t *testing.T) {
	policy := mustPolicy(t, []Rule{spendingRule("spending", "500", DecisionDeny)})
	active, err := policy.Transition(StatusActive, policyTestNow.Add(-30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := active.Transition(StatusDisabled, policyTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if !disabled.Status().Terminal() {
		t.Fatal("disabled policy is not terminal")
	}
	if _, err := disabled.Transition(StatusActive, policyTestNow.Add(time.Minute)); !policyErrorCode(err, apperrors.CodePolicyInvalid) {
		t.Fatalf("terminal transition error = %v", err)
	}

	expiringParams := policyParams([]Rule{spendingRule("spending", "500", DecisionDeny)})
	expiringParams.ExpiresAt = policyTestNow.Add(time.Hour)
	expiring, err := NewDraft(expiringParams)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := expiring.Transition(StatusExpired, policyTestNow); !policyErrorCode(err, apperrors.CodePolicyInvalid) {
		t.Fatalf("early expiration error = %v", err)
	}
	expired, err := expiring.Transition(StatusExpired, expiring.ExpiresAt())
	if err != nil || expired.Status() != StatusExpired || !expired.Status().Terminal() {
		t.Fatalf("expired = (%s, %v)", expired.Status(), err)
	}
}

func policyErrorCode(err error, code apperrors.Code) bool {
	var appError *apperrors.Error
	return stderrors.As(err, &appError) && appError.Code == code
}
