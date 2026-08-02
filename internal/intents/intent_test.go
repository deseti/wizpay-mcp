package intents

import (
	stderrors "errors"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

var testNow = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

func testToken(chain string) Token {
	return Token{ChainID: chain, Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}
}

func testAmount(decimal, base string) Amount {
	return Amount{Decimal: decimal, BaseUnits: base, Decimals: 6}
}

func testParams() Params {
	token := testToken("5042002")
	return Params{
		IntentID: "int_001", Version: 1, ClientRequestID: "req_001", Nonce: "nonce_001", Type: TypePayroll,
		Ownership:   Ownership{UserID: "user_001", IdentityProvider: "circle", ProviderUserReference: "provider_user_001", WalletBindingID: "bind_001", WalletBindingVersion: 4, WalletID: "wallet_001", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet"},
		Financial:   FinancialParameters{Payroll: &PayrollParameters{Token: token, Recipients: []Recipient{{Address: "0x3333333333333333333333333333333333333333", Amount: testAmount("1.25", "1250000")}, {Address: "0x4444444444444444444444444444444444444444", Amount: testAmount("2", "2000000")}}, Total: testAmount("3.25", "3250000")}},
		Route:       Route{Type: RouteAllowlistedContract, Reference: "route_payroll_v1", Version: 1},
		Constraints: Constraints{Deadline: testNow.Add(20 * time.Minute), PolicyReference: "policy_v1"},
		CreatedAt:   testNow, ExpiresAt: testNow.Add(30 * time.Minute),
	}
}

func mustDraft(t *testing.T, params Params) Intent {
	t.Helper()
	intent, err := NewDraft(params)
	if err != nil {
		t.Fatalf("NewDraft() error = %v", err)
	}
	return intent
}

func mustTransition(t *testing.T, intent Intent, status Status, at time.Time) Intent {
	t.Helper()
	next, err := intent.Transition(status, at)
	if err != nil {
		t.Fatalf("Transition(%s) error = %v", status, err)
	}
	return next
}

type approvalStub struct{ err error }

func (a approvalStub) EnsureAuthorizes(Intent, time.Time) error { return a.err }

func mustApprove(t *testing.T, intent Intent, at time.Time) Intent {
	t.Helper()
	next, err := intent.Approve(approvalStub{}, at)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	return next
}

func TestNewDraftAcceptsEveryTypedIntent(t *testing.T) {
	tests := []struct {
		name      string
		kind      Type
		financial FinancialParameters
	}{
		{"payroll", TypePayroll, testParams().Financial},
		{"swap", TypeSwap, FinancialParameters{Swap: &SwapParameters{InputToken: testToken("5042002"), OutputToken: Token{ChainID: "5042002", Standard: "ERC20", Address: "0x5555555555555555555555555555555555555555", Symbol: "EURC", Decimals: 6}, InputAmount: testAmount("10", "10000000"), ExpectedOutput: testAmount("9.5", "9500000"), MinimumOutput: testAmount("9", "9000000"), QuoteReference: "quote_001", MaxSlippageBPS: 100}}},
		{"bridge", TypeBridge, FinancialParameters{Bridge: &BridgeParameters{SourceChainID: "5042002", DestinationChainID: "1", SourceToken: testToken("5042002"), SourceAmount: testAmount("10", "10000000"), DestinationAmount: testAmount("9.9", "9900000"), DestinationAddress: "0x6666666666666666666666666666666666666666", PlanReference: "plan_001"}}},
		{"ans", TypeANSRegistration, FinancialParameters{ANS: &ANSParameters{NormalizedName: "alice.arc", NameVersion: "v1", TermSeconds: 31_536_000, Controller: "0x7777777777777777777777777777777777777777", CostToken: testToken("5042002"), Cost: testAmount("1", "1000000")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := testParams()
			params.Type, params.Financial = tt.kind, tt.financial
			if _, err := NewDraft(params); err != nil {
				t.Fatalf("NewDraft() error = %v", err)
			}
		})
	}
}

func TestCanonicalJSONUsesRFC8785OrderingAndEscaping(t *testing.T) {
	value := struct {
		B uint64 `json:"b"`
		A string `json:"a"`
	}{B: 2, A: "<é\n"}
	canonical, err := canonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(canonical), "{\"a\":\"<é\\n\",\"b\":2}"; got != want {
		t.Fatalf("canonical JSON = %q, want %q", got, want)
	}
}

func TestNewDraftRejectsInvalidTypeAndMismatchedParameters(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Params)
	}{
		{"invalid enum", func(p *Params) { p.Type = Type("TRANSFER") }},
		{"mismatched payload", func(p *Params) { p.Type = TypeSwap }},
		{"multiple payloads", func(p *Params) { p.Financial.Swap = &SwapParameters{} }},
		{"inexact amount", func(p *Params) { p.Financial.Payroll.Recipients[0].Amount.BaseUnits = "1" }},
		{"wrong chain", func(p *Params) { p.Ownership.ChainID = "1" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := testParams()
			tt.mutate(&params)
			if _, err := NewDraft(params); !hasCode(err, apperrors.CodeValidationError) {
				t.Fatalf("error = %v, want validation_error", err)
			}
		})
	}
}

func TestIntentLifecycleAndInvalidTransition(t *testing.T) {
	intent := mustDraft(t, testParams())
	intent = mustTransition(t, intent, StatusCreated, testNow.Add(time.Second))
	if intent.Digest() == "" {
		t.Fatal("created intent has empty digest")
	}
	if len(intent.Digest()) != len("sha256:")+64 {
		t.Fatalf("digest = %q", intent.Digest())
	}
	intent = mustTransition(t, intent, StatusApprovalRequired, testNow.Add(2*time.Second))
	if _, err := intent.Transition(StatusApproved, testNow.Add(3*time.Second)); !hasCode(err, apperrors.CodeApprovalRequired) {
		t.Fatalf("direct approval transition error = %v", err)
	}
	intent = mustApprove(t, intent, testNow.Add(3*time.Second))
	intent = mustTransition(t, intent, StatusReadyForExecution, testNow.Add(4*time.Second))
	if intent.Status().Terminal() {
		t.Fatalf("ready status must remain expirable before execution")
	}
	cancelled := mustTransition(t, intent, StatusCancelled, testNow.Add(5*time.Second))
	if !cancelled.Status().Terminal() {
		t.Fatal("cancelled intent is not terminal")
	}
	if _, err := cancelled.Transition(StatusExpired, cancelled.ExpiresAt()); !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("terminal transition error = %v", err)
	}
}

func TestApprovedIntentCannotBeMateriallyChanged(t *testing.T) {
	intent := mustDraft(t, testParams())
	intent = mustTransition(t, intent, StatusCreated, testNow.Add(time.Second))
	intent = mustTransition(t, intent, StatusApprovalRequired, testNow.Add(2*time.Second))
	intent = mustApprove(t, intent, testNow.Add(3*time.Second))

	if replay, err := intent.ReviseDraft(testParams()); err != nil || replay.Digest() != intent.Digest() {
		t.Fatalf("exact replay = (%v, %v)", replay.Digest(), err)
	}
	changed := testParams()
	changed.Financial.Payroll.Recipients[0].Amount = testAmount("1.5", "1500000")
	changed.Financial.Payroll.Total = testAmount("3.5", "3500000")
	if _, err := intent.ReviseDraft(changed); !hasCode(err, apperrors.CodeIntentMutated) {
		t.Fatalf("mutation error = %v, want intent_mutated", err)
	}
}

func TestIntentExpirationAndOperationIdempotency(t *testing.T) {
	created := mustTransition(t, mustDraft(t, testParams()), StatusCreated, testNow.Add(time.Second))
	if _, err := created.Transition(StatusApprovalRequired, created.ExpiresAt()); !hasCode(err, apperrors.CodeIntentExpired) {
		t.Fatalf("late transition error = %v", err)
	}
	expired := mustTransition(t, created, StatusExpired, created.ExpiresAt())
	if expired.Status() != StatusExpired {
		t.Fatalf("status = %s", expired.Status())
	}

	first, err := NewOperationIdentity(created)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewOperationIdentity(created)
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationKey() != second.OperationKey() {
		t.Fatal("same frozen intent produced different operation keys")
	}
	if first.Version() != OperationIdentityVersion || second.Version() != OperationIdentityVersion {
		t.Fatal("operation identity version mismatch")
	}
	if err := first.EnsureMatches(created); err != nil {
		t.Fatalf("EnsureMatches() error = %v", err)
	}
}

func hasCode(err error, code apperrors.Code) bool {
	var appErr *apperrors.Error
	return stderrors.As(err, &appErr) && appErr.Code == code
}
