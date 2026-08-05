package intents

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func phase12PayrollParams() Params {
	tokenIn := testToken("5042002")
	tokenOut := Token{ChainID: "5042002", Standard: "ERC20", Address: "0x5555555555555555555555555555555555555555", Symbol: "EURC", Decimals: 6}
	return Params{
		IntentID: "int_payroll_p12", Version: 1, ClientRequestID: "req_payroll_p12", Nonce: "nonce_payroll_p12", Type: TypePayroll,
		Ownership: Ownership{
			UserID: "user_001", IdentityProvider: "circle", ProviderUserReference: "provider_user_001",
			WalletBindingID: "bind_001", WalletBindingVersion: 4, WalletID: "wallet_001",
			WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet",
		},
		Financial: FinancialParameters{Payroll: &PayrollParameters{
			SchemaVersion: FinancialSchemaPhase12,
			Variant:       PayrollVariantBatchSingleTokenOut,
			TokenIn:       tokenIn,
			Recipients: []Recipient{
				{Address: "0x3333333333333333333333333333333333333333", TokenOut: tokenOut, AmountIn: testAmount("1.25", "1250000"), MinAmountOut: testAmount("1.2", "1200000")},
				{Address: "0x4444444444444444444444444444444444444444", TokenOut: tokenOut, AmountIn: testAmount("2", "2000000"), MinAmountOut: testAmount("1.9", "1900000")},
			},
			Total:       testAmount("3.25", "3250000"),
			ReferenceID: "payroll-batch-001",
		}},
		Route:       Route{Type: RouteAllowlistedContract, Reference: RouteReferencePayroll, Version: RouteVersionPayroll},
		Constraints: Constraints{Deadline: testNow.Add(20 * time.Minute), PolicyReference: "policy_v1"},
		CreatedAt:   testNow, ExpiresAt: testNow.Add(30 * time.Minute),
	}
}

func phase12SwapParams() Params {
	quoteExpiry := testNow.Add(15 * time.Minute)
	deadline := testNow.Add(10 * time.Minute)
	input := testToken("5042002")
	output := Token{ChainID: "5042002", Standard: "ERC20", Address: "0x5555555555555555555555555555555555555555", Symbol: "EURC", Decimals: 6}
	expected := testAmount("9.5", "9500000")
	minimum := testAmount("9.405", "9405000") // exactly 1% below expected (100 bps)
	return Params{
		IntentID: "int_swap_p12", Version: 1, ClientRequestID: "req_swap_p12", Nonce: "nonce_swap_p12", Type: TypeSwap,
		Ownership: Ownership{
			UserID: "user_001", IdentityProvider: "circle", ProviderUserReference: "provider_user_001",
			WalletBindingID: "bind_001", WalletBindingVersion: 4, WalletID: "wallet_001",
			WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: "5042002", Network: "arc-testnet",
		},
		Financial: FinancialParameters{Swap: &SwapParameters{
			SchemaVersion:  FinancialSchemaPhase12,
			InputToken:     input,
			OutputToken:    output,
			InputAmount:    testAmount("10", "10000000"),
			ExpectedOutput: expected,
			MinimumOutput:  minimum,
			MaxSlippageBPS: 100,
			Router:         "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Recipient:      "0x2222222222222222222222222222222222222222",
			Quote: &SwapQuote{
				QuoteID: "quote_p12_001", Source: "test-quote-source", ExpectedAmountOut: expected, MinAmountOut: minimum,
				Router: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: quoteExpiry, EvidenceReference: "ev_quote_001",
			},
			Deadline: deadline,
		}},
		Route:       Route{Type: RouteAllowlistedContract, Reference: RouteReferenceSwap, Version: RouteVersionSwap},
		Constraints: Constraints{Deadline: testNow.Add(20 * time.Minute), PolicyReference: "policy_v1"},
		CreatedAt:   testNow, ExpiresAt: testNow.Add(30 * time.Minute),
	}
}

func TestPhase12PayrollVariantsValid(t *testing.T) {
	tokenIn := testToken("5042002")
	tokenOutA := Token{ChainID: "5042002", Standard: "ERC20", Address: "0x5555555555555555555555555555555555555555", Symbol: "EURC", Decimals: 6}
	tokenOutB := Token{ChainID: "5042002", Standard: "ERC20", Address: "0x6666666666666666666666666666666666666666", Symbol: "USDC2", Decimals: 6}

	t.Run("single", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll = &PayrollParameters{
			SchemaVersion: FinancialSchemaPhase12,
			Variant:       PayrollVariantSingle, TokenIn: tokenIn,
			Recipients: []Recipient{{Address: "0x3333333333333333333333333333333333333333", TokenOut: tokenOutA, AmountIn: testAmount("1", "1000000"), MinAmountOut: testAmount("1", "1000000")}},
			Total:      testAmount("1", "1000000"),
		}
		if _, err := NewDraft(params); err != nil {
			t.Fatalf("SINGLE: %v", err)
		}
	})
	t.Run("batch_single", func(t *testing.T) {
		if _, err := NewDraft(phase12PayrollParams()); err != nil {
			t.Fatalf("BATCH_SINGLE: %v", err)
		}
	})
	t.Run("batch_multi", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll = &PayrollParameters{
			SchemaVersion: FinancialSchemaPhase12,
			Variant:       PayrollVariantBatchMultiTokenOut, TokenIn: tokenIn, ReferenceID: "ref-multi",
			Recipients: []Recipient{
				{Address: "0x3333333333333333333333333333333333333333", TokenOut: tokenOutA, AmountIn: testAmount("1", "1000000"), MinAmountOut: testAmount("0.9", "900000")},
				{Address: "0x4444444444444444444444444444444444444444", TokenOut: tokenOutB, AmountIn: testAmount("2", "2000000"), MinAmountOut: testAmount("1.8", "1800000")},
			},
			Total: testAmount("3", "3000000"),
		}
		if _, err := NewDraft(params); err != nil {
			t.Fatalf("BATCH_MULTI: %v", err)
		}
	})
}

func TestPhase12PayrollValidationRejects(t *testing.T) {
	otherOut := Token{ChainID: "5042002", Standard: "ERC20", Address: "0x6666666666666666666666666666666666666666", Symbol: "X", Decimals: 6}

	cases := []struct {
		name   string
		mutate func(*PayrollParameters)
	}{
		{"invalid_variant", func(p *PayrollParameters) { p.Variant = "NOPE" }},
		{"zero_recipients", func(p *PayrollParameters) { p.Recipients = nil; p.Total = testAmount("0", "0") }},
		{"zero_recipient_address", func(p *PayrollParameters) {
			p.Recipients[0].Address = "0x0000000000000000000000000000000000000000"
		}},
		{"duplicate_recipient", func(p *PayrollParameters) {
			p.Recipients[1].Address = p.Recipients[0].Address
		}},
		{"zero_token_in", func(p *PayrollParameters) {
			p.TokenIn.Address = "0x0000000000000000000000000000000000000000"
		}},
		{"zero_amount_in", func(p *PayrollParameters) {
			p.Recipients[0].AmountIn = testAmount("0", "0")
			p.Total = testAmount("2", "2000000")
		}},
		{"missing_min_out", func(p *PayrollParameters) {
			p.Recipients[0].MinAmountOut = Amount{}
		}},
		{"zero_min_out", func(p *PayrollParameters) {
			p.Recipients[0].MinAmountOut = Amount{Decimal: "0", BaseUnits: "0", Decimals: 6}
		}},
		{"invalid_min_out", func(p *PayrollParameters) {
			p.Recipients[0].MinAmountOut = Amount{Decimal: "1.0", BaseUnits: "1000000", Decimals: 6}
		}},
		{"single_with_two", func(p *PayrollParameters) {
			p.Variant = PayrollVariantSingle
			p.ReferenceID = ""
		}},
		{"batch_single_mixed_token_out", func(p *PayrollParameters) {
			p.Recipients[1].TokenOut = otherOut
		}},
		{"batch_missing_reference", func(p *PayrollParameters) { p.ReferenceID = "" }},
		{"total_mismatch", func(p *PayrollParameters) { p.Total = testAmount("9", "9000000") }},
		{"schema_version_zero", func(p *PayrollParameters) { p.SchemaVersion = 0 }},
		{"schema_version_unsupported", func(p *PayrollParameters) { p.SchemaVersion = 2 }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			params := phase12PayrollParams()
			payroll := *params.Financial.Payroll
			tt.mutate(&payroll)
			params.Financial.Payroll = &payroll
			if _, err := NewDraft(params); err == nil {
				t.Fatalf("expected rejection for %s", tt.name)
			}
		})
	}
}

func TestPhase12PayrollRejects257DistinctRecipients(t *testing.T) {
	params := phase12PayrollParams()
	tokenOut := params.Financial.Payroll.Recipients[0].TokenOut
	lines := make([]Recipient, 257)
	total := 0
	for i := range lines {
		// generate unique valid addresses
		hex := make([]byte, 40)
		for j := range hex {
			hex[j] = "0123456789abcdef"[(i+j*3)%16]
		}
		addr := "0x" + string(hex)
		lines[i] = Recipient{Address: addr, TokenOut: tokenOut, AmountIn: testAmount("1", "1000000"), MinAmountOut: testAmount("1", "1000000")}
		total++
	}
	params.Financial.Payroll.Recipients = lines
	params.Financial.Payroll.Total = Amount{Decimal: "257", BaseUnits: "257000000", Decimals: 6}
	if _, err := NewDraft(params); err == nil {
		t.Fatal("257 recipients must be rejected")
	}
}

func TestPhase12PayrollDigestMutations(t *testing.T) {
	base := mustTransition(t, mustDraft(t, phase12PayrollParams()), StatusCreated, testNow.Add(time.Second))
	baseDigest := base.Digest()

	mutations := []struct {
		name   string
		mutate func(*Params)
	}{
		{"variant", func(p *Params) {
			p.Financial.Payroll.Variant = PayrollVariantBatchMultiTokenOut
			// keep same token outs already identical - multi allows it
		}},
		{"token_in", func(p *Params) {
			p.Financial.Payroll.TokenIn.Address = "0x9999999999999999999999999999999999999999"
		}},
		{"recipient", func(p *Params) {
			p.Financial.Payroll.Recipients[0].Address = "0x7777777777777777777777777777777777777777"
		}},
		{"token_out", func(p *Params) {
			// switch to multi so different token outs allowed
			p.Financial.Payroll.Variant = PayrollVariantBatchMultiTokenOut
			p.Financial.Payroll.Recipients[0].TokenOut.Address = "0x8888888888888888888888888888888888888888"
		}},
		{"amount_in", func(p *Params) {
			p.Financial.Payroll.Recipients[0].AmountIn = testAmount("1.5", "1500000")
			p.Financial.Payroll.Total = testAmount("3.5", "3500000")
		}},
		{"min_amount_out", func(p *Params) {
			p.Financial.Payroll.Recipients[0].MinAmountOut = testAmount("1.1", "1100000")
		}},
		{"reference_id", func(p *Params) { p.Financial.Payroll.ReferenceID = "other-ref" }},
		{"recipient_order", func(p *Params) {
			p.Financial.Payroll.Recipients[0], p.Financial.Payroll.Recipients[1] = p.Financial.Payroll.Recipients[1], p.Financial.Payroll.Recipients[0]
		}},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			params := phase12PayrollParams()
			tt.mutate(&params)
			intent := mustTransition(t, mustDraft(t, params), StatusCreated, testNow.Add(time.Second))
			if intent.Digest() == baseDigest {
				t.Fatalf("digest unchanged after mutating %s", tt.name)
			}
		})
	}
	// Same value => same digest
	again := mustTransition(t, mustDraft(t, phase12PayrollParams()), StatusCreated, testNow.Add(time.Second))
	if again.Digest() != baseDigest {
		t.Fatalf("identical params produced different digests")
	}
}

func TestPhase12PayrollAddressCaseNormalization(t *testing.T) {
	lower := phase12PayrollParams()
	upper := phase12PayrollParams()
	upper.Financial.Payroll.Recipients[0].Address = "0x3333333333333333333333333333333333333333"
	// mixed case equivalent of same address
	upper.Financial.Payroll.Recipients[0].Address = "0x3333333333333333333333333333333333333333"
	// Use EIP-55 mixed for token_in
	upper.Financial.Payroll.TokenIn.Address = "0x1111111111111111111111111111111111111111"
	mixed := phase12PayrollParams()
	mixed.Financial.Payroll.TokenIn.Address = "0x1111111111111111111111111111111111111111"
	// force upper hex letters where valid
	mixed.Financial.Payroll.Recipients[0].Address = strings.ToUpper(mixed.Financial.Payroll.Recipients[0].Address[:2]) + mixed.Financial.Payroll.Recipients[0].Address[2:]
	// 0x + upper hex
	addr := mixed.Financial.Payroll.Recipients[0].Address
	mixed.Financial.Payroll.Recipients[0].Address = "0x" + strings.ToUpper(addr[2:])

	a := mustTransition(t, mustDraft(t, lower), StatusCreated, testNow.Add(time.Second))
	b := mustTransition(t, mustDraft(t, mixed), StatusCreated, testNow.Add(time.Second))
	if a.Digest() != b.Digest() {
		t.Fatalf("address case variants must normalize to the same digest")
	}
}

func TestPhase12SwapValidAndDigestStable(t *testing.T) {
	intent := mustTransition(t, mustDraft(t, phase12SwapParams()), StatusCreated, testNow.Add(time.Second))
	if intent.Digest() == "" {
		t.Fatal("empty digest")
	}
	again := mustTransition(t, mustDraft(t, phase12SwapParams()), StatusCreated, testNow.Add(time.Second))
	if intent.Digest() != again.Digest() {
		t.Fatal("identical swap digests differ")
	}
}

func TestPhase12SwapValidationRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SwapParameters)
	}{
		{"invalid_router", func(p *SwapParameters) { p.Router = "not-an-address"; p.Quote.Router = "not-an-address" }},
		{"zero_router", func(p *SwapParameters) {
			p.Router = "0x0000000000000000000000000000000000000000"
			p.Quote.Router = p.Router
		}},
		{"invalid_recipient", func(p *SwapParameters) { p.Recipient = "0x00" }},
		{"zero_input", func(p *SwapParameters) { p.InputAmount = testAmount("0", "0") }},
		{"min_gt_expected", func(p *SwapParameters) {
			p.MinimumOutput = testAmount("10", "10000000")
			p.Quote.MinAmountOut = p.MinimumOutput
		}},
		{"slippage_too_high", func(p *SwapParameters) { p.MaxSlippageBPS = 10001 }},
		{"min_weaker_than_slippage", func(p *SwapParameters) {
			// expected 9.5e6, 100 bps ceiling = 9405000; set min lower
			p.MinimumOutput = testAmount("9", "9000000")
			p.Quote.MinAmountOut = p.MinimumOutput
		}},
		{"schema_version_zero", func(p *SwapParameters) { p.SchemaVersion = 0 }},
		{"schema_version_unsupported", func(p *SwapParameters) { p.SchemaVersion = 2 }},
		{"quote_router_mismatch", func(p *SwapParameters) {
			p.Quote.Router = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}},
		{"quote_expected_mismatch", func(p *SwapParameters) {
			p.Quote.ExpectedAmountOut = testAmount("9", "9000000")
		}},
		{"quote_min_mismatch", func(p *SwapParameters) {
			p.Quote.MinAmountOut = testAmount("9.4", "9400000")
		}},
		{"same_token", func(p *SwapParameters) {
			p.OutputToken = p.InputToken
		}},
		{"deadline_after_quote", func(p *SwapParameters) {
			p.Deadline = p.Quote.ExpiresAt.Add(time.Minute)
		}},
		{"expired_quote", func(p *SwapParameters) {
			p.Quote.ExpiresAt = testNow.Add(-time.Minute)
			p.Deadline = testNow.Add(-2 * time.Minute)
		}},
		{"missing_quote", func(p *SwapParameters) { p.Quote = nil }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			params := phase12SwapParams()
			swap := *params.Financial.Swap
			if swap.Quote != nil {
				q := *swap.Quote
				swap.Quote = &q
			}
			tt.mutate(&swap)
			params.Financial.Swap = &swap
			if _, err := NewDraft(params); err == nil {
				t.Fatalf("expected rejection for %s", tt.name)
			}
		})
	}
}

func TestPhase12SwapDigestMutations(t *testing.T) {
	base := mustTransition(t, mustDraft(t, phase12SwapParams()), StatusCreated, testNow.Add(time.Second))
	baseDigest := base.Digest()
	mutations := []struct {
		name   string
		mutate func(*Params)
	}{
		{"token_in", func(p *Params) {
			p.Financial.Swap.InputToken.Address = "0x9999999999999999999999999999999999999999"
		}},
		{"token_out", func(p *Params) {
			p.Financial.Swap.OutputToken.Address = "0x8888888888888888888888888888888888888888"
		}},
		{"amount_in", func(p *Params) { p.Financial.Swap.InputAmount = testAmount("11", "11000000") }},
		{"expected", func(p *Params) {
			p.Financial.Swap.ExpectedOutput = testAmount("9.6", "9600000")
			p.Financial.Swap.Quote.ExpectedAmountOut = p.Financial.Swap.ExpectedOutput
			// keep min valid for 100 bps: ceil(9600000*0.99)=9504000
			p.Financial.Swap.MinimumOutput = testAmount("9.504", "9504000")
			p.Financial.Swap.Quote.MinAmountOut = p.Financial.Swap.MinimumOutput
		}},
		{"min", func(p *Params) {
			p.Financial.Swap.MinimumOutput = testAmount("9.45", "9450000")
			p.Financial.Swap.Quote.MinAmountOut = p.Financial.Swap.MinimumOutput
		}},
		{"slippage", func(p *Params) {
			p.Financial.Swap.MaxSlippageBPS = 200
			// min still >= ceiling at 200 bps
		}},
		{"router", func(p *Params) {
			p.Financial.Swap.Router = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			p.Financial.Swap.Quote.Router = p.Financial.Swap.Router
		}},
		{"recipient", func(p *Params) {
			p.Financial.Swap.Recipient = "0xcccccccccccccccccccccccccccccccccccccccc"
		}},
		{"quote_id", func(p *Params) { p.Financial.Swap.Quote.QuoteID = "quote_other" }},
		{"quote_expiry", func(p *Params) {
			p.Financial.Swap.Quote.ExpiresAt = testNow.Add(16 * time.Minute)
		}},
		{"quote_router", func(p *Params) {
			// keep match with frozen router
			p.Financial.Swap.Router = "0xb1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1"
			p.Financial.Swap.Quote.Router = p.Financial.Swap.Router
		}},
		{"deadline", func(p *Params) { p.Financial.Swap.Deadline = testNow.Add(11 * time.Minute) }},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			params := phase12SwapParams()
			// deep copy quote
			q := *params.Financial.Swap.Quote
			params.Financial.Swap.Quote = &q
			tt.mutate(&params)
			intent := mustTransition(t, mustDraft(t, params), StatusCreated, testNow.Add(time.Second))
			if intent.Digest() == baseDigest {
				t.Fatalf("digest unchanged after mutating %s", tt.name)
			}
		})
	}
}

func TestLegacyPayrollAndSwapRemainReadable(t *testing.T) {
	// Historical payroll shape (no variant/token_in).
	legacyPayroll := testParams()
	intent := mustTransition(t, mustDraft(t, legacyPayroll), StatusCreated, testNow.Add(time.Second))
	if intent.Financial().Payroll.IsPhase12() {
		t.Fatal("legacy payroll must not report Phase 12")
	}
	if intent.Financial().Payroll.Phase12Executable() {
		t.Fatal("legacy payroll must not be Phase12Executable")
	}
	if intent.Financial().Payroll.SchemaVersion != FinancialSchemaLegacy {
		t.Fatalf("legacy payroll schema = %d, want 0", intent.Financial().Payroll.SchemaVersion)
	}
	// Round-trip JSON like PostgreSQL storage.
	raw, err := json.Marshal(intent.Financial())
	if err != nil {
		t.Fatal(err)
	}
	var restored FinancialParameters
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	restoreParams := Params{
		IntentID: intent.IntentID(), Version: intent.Version(), ClientRequestID: intent.ClientRequestID(),
		Nonce: intent.Nonce(), Type: intent.Type(), Ownership: intent.Ownership(), Financial: restored,
		Route: intent.Route(), Constraints: intent.Constraints(), CreatedAt: intent.CreatedAt(), ExpiresAt: intent.ExpiresAt(),
	}
	restoredIntent, err := Restore(restoreParams, intent.Status(), intent.Digest(), intent.LifecycleRevision())
	if err != nil {
		t.Fatalf("legacy payroll restore: %v", err)
	}
	if restoredIntent.Digest() != intent.Digest() {
		t.Fatal("legacy payroll digest changed after restore")
	}
	if restoredIntent.Financial().Payroll.SchemaVersion != 0 {
		t.Fatal("restore must not stamp schema_version on legacy payroll")
	}
	if restoredIntent.Financial().Payroll.Phase12Executable() {
		t.Fatal("restored legacy payroll must not be Phase12Executable")
	}
	if !restoredIntent.Financial().Payroll.TokenIn.IsZero() || restoredIntent.Financial().Payroll.Variant != "" ||
		restoredIntent.Financial().Payroll.ReferenceID != "" {
		t.Fatalf("restore invented payroll phase12 fields: %+v", restoredIntent.Financial().Payroll)
	}
	for _, r := range restoredIntent.Financial().Payroll.Recipients {
		if !r.TokenOut.IsZero() || !r.AmountIn.IsZero() || !r.MinAmountOut.IsZero() {
			t.Fatalf("restore invented recipient phase12 fields: %+v", r)
		}
	}

	// Historical swap shape.
	legacySwap := testParams()
	legacySwap.Type = TypeSwap
	legacySwap.Financial = FinancialParameters{Swap: &SwapParameters{
		InputToken:  testToken("5042002"),
		OutputToken: Token{ChainID: "5042002", Standard: "ERC20", Address: "0x5555555555555555555555555555555555555555", Symbol: "EURC", Decimals: 6},
		InputAmount: testAmount("10", "10000000"), ExpectedOutput: testAmount("9.5", "9500000"), MinimumOutput: testAmount("9", "9000000"),
		QuoteReference: "quote_001", MaxSlippageBPS: 100,
	}}
	legacySwap.Route = Route{Type: RouteApprovedProvider, Reference: "provider_swap", Version: 1}
	swapIntent := mustTransition(t, mustDraft(t, legacySwap), StatusCreated, testNow.Add(time.Second))
	if swapIntent.Financial().Swap.IsPhase12() || swapIntent.Financial().Swap.Phase12Executable() {
		t.Fatal("legacy swap must not be phase 12 executable")
	}
	if swapIntent.Financial().Swap.SchemaVersion != FinancialSchemaLegacy {
		t.Fatalf("legacy swap schema = %d, want 0", swapIntent.Financial().Swap.SchemaVersion)
	}
	raw, err = json.Marshal(swapIntent.Financial())
	if err != nil {
		t.Fatal(err)
	}
	restored = FinancialParameters{}
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	// Ensure no silent enrichment of router/recipient/quote on JSON alone.
	if restored.Swap.Router != "" || restored.Swap.Recipient != "" || restored.Swap.Quote != nil || restored.Swap.SchemaVersion != 0 {
		t.Fatalf("legacy swap was enriched: %+v", restored.Swap)
	}
	swapRestoreParams := Params{
		IntentID: swapIntent.IntentID(), Version: swapIntent.Version(), ClientRequestID: swapIntent.ClientRequestID(),
		Nonce: swapIntent.Nonce(), Type: swapIntent.Type(), Ownership: swapIntent.Ownership(), Financial: restored,
		Route: swapIntent.Route(), Constraints: swapIntent.Constraints(), CreatedAt: swapIntent.CreatedAt(), ExpiresAt: swapIntent.ExpiresAt(),
	}
	restoredSwap, err := Restore(swapRestoreParams, swapIntent.Status(), swapIntent.Digest(), swapIntent.LifecycleRevision())
	if err != nil {
		t.Fatalf("legacy swap restore: %v", err)
	}
	if restoredSwap.Digest() != swapIntent.Digest() {
		t.Fatal("legacy swap digest changed after restore")
	}
	if restoredSwap.Financial().Swap.SchemaVersion != 0 {
		t.Fatal("restore must not stamp schema_version on legacy swap")
	}
	if restoredSwap.Financial().Swap.Phase12Executable() {
		t.Fatal("restored legacy swap must not be Phase12Executable")
	}
	if restoredSwap.Financial().Swap.Router != "" || restoredSwap.Financial().Swap.Recipient != "" ||
		restoredSwap.Financial().Swap.Quote != nil || !restoredSwap.Financial().Swap.Deadline.IsZero() {
		t.Fatalf("restore invented swap phase12 fields: %+v", restoredSwap.Financial().Swap)
	}
}

func TestPhase12PayrollApprovalDigestBinding(t *testing.T) {
	intent := mustDraft(t, phase12PayrollParams())
	intent = mustTransition(t, intent, StatusCreated, testNow.Add(time.Second))
	intent = mustTransition(t, intent, StatusApprovalRequired, testNow.Add(2*time.Second))
	intent = mustApprove(t, intent, testNow.Add(3*time.Second))
	if intent.Status() != StatusApproved {
		t.Fatalf("status = %s", intent.Status())
	}
	changed := phase12PayrollParams()
	changed.Financial.Payroll.ReferenceID = "mutated"
	if _, err := intent.ReviseDraft(changed); !hasCode(err, apperrors.CodeIntentMutated) {
		t.Fatalf("mutation error = %v, want intent_mutated", err)
	}
}

func TestPhase12RouteBindingRequired(t *testing.T) {
	params := phase12PayrollParams()
	params.Route.Reference = "route_payroll_v1"
	if _, err := NewDraft(params); err == nil {
		t.Fatal("wrong payroll route must fail")
	}
	swap := phase12SwapParams()
	swap.Route.Reference = "something_else"
	if _, err := NewDraft(swap); err == nil {
		t.Fatal("wrong swap route must fail")
	}
}

func TestPhase12PayrollMinAmountOutMustBePositive(t *testing.T) {
	tokenOut := Token{ChainID: "5042002", Standard: "ERC20", Address: "0x5555555555555555555555555555555555555555", Symbol: "EURC", Decimals: 6}

	t.Run("zero_rejected", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll.Variant = PayrollVariantSingle
		params.Financial.Payroll.ReferenceID = ""
		params.Financial.Payroll.Recipients = []Recipient{{
			Address:      "0x3333333333333333333333333333333333333333",
			TokenOut:     tokenOut,
			AmountIn:     testAmount("1", "1000000"),
			MinAmountOut: Amount{Decimal: "0", BaseUnits: "0", Decimals: 6},
		}}
		params.Financial.Payroll.Total = testAmount("1", "1000000")
		if _, err := NewDraft(params); err == nil {
			t.Fatal("explicit zero min_amount_out must be rejected")
		}
	})
	t.Run("omitted_rejected", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll.Variant = PayrollVariantSingle
		params.Financial.Payroll.ReferenceID = ""
		params.Financial.Payroll.Recipients = []Recipient{{
			Address:  "0x3333333333333333333333333333333333333333",
			TokenOut: tokenOut,
			AmountIn: testAmount("1", "1000000"),
		}}
		params.Financial.Payroll.Total = testAmount("1", "1000000")
		if _, err := NewDraft(params); err == nil {
			t.Fatal("omitted min_amount_out must be rejected")
		}
	})
	t.Run("invalid_rejected", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll.Variant = PayrollVariantSingle
		params.Financial.Payroll.ReferenceID = ""
		params.Financial.Payroll.Recipients = []Recipient{{
			Address:      "0x3333333333333333333333333333333333333333",
			TokenOut:     tokenOut,
			AmountIn:     testAmount("1", "1000000"),
			MinAmountOut: Amount{Decimal: "1.0", BaseUnits: "1000000", Decimals: 6}, // non-canonical fractional
		}}
		params.Financial.Payroll.Total = testAmount("1", "1000000")
		if _, err := NewDraft(params); err == nil {
			t.Fatal("invalid min_amount_out must be rejected")
		}
	})
	t.Run("positive_accepted", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll.Variant = PayrollVariantSingle
		params.Financial.Payroll.ReferenceID = ""
		params.Financial.Payroll.Recipients = []Recipient{{
			Address:      "0x3333333333333333333333333333333333333333",
			TokenOut:     tokenOut,
			AmountIn:     testAmount("1", "1000000"),
			MinAmountOut: testAmount("0.99", "990000"),
		}}
		params.Financial.Payroll.Total = testAmount("1", "1000000")
		if _, err := NewDraft(params); err != nil {
			t.Fatalf("positive min_amount_out must be accepted: %v", err)
		}
	})
	t.Run("token_out_decimal_mismatch_rejected", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll.Variant = PayrollVariantSingle
		params.Financial.Payroll.ReferenceID = ""
		params.Financial.Payroll.Recipients = []Recipient{{
			Address:      "0x3333333333333333333333333333333333333333",
			TokenOut:     tokenOut,
			AmountIn:     testAmount("1", "1000000"),
			MinAmountOut: Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 8},
		}}
		params.Financial.Payroll.Total = testAmount("1", "1000000")
		if _, err := NewDraft(params); err == nil {
			t.Fatal("min_amount_out decimals must match token_out")
		}
	})
}

func TestPhase12SchemaVersionExplicitRequired(t *testing.T) {
	t.Run("payroll_schema_0_rejected", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll.SchemaVersion = 0
		if _, err := NewDraft(params); err == nil {
			t.Fatal("phase 12 payroll with schema_version 0 must be rejected")
		}
	})
	t.Run("swap_schema_0_rejected", func(t *testing.T) {
		params := phase12SwapParams()
		params.Financial.Swap.SchemaVersion = 0
		if _, err := NewDraft(params); err == nil {
			t.Fatal("phase 12 swap with schema_version 0 must be rejected")
		}
	})
	t.Run("payroll_schema_1_accepted", func(t *testing.T) {
		params := phase12PayrollParams()
		if params.Financial.Payroll.SchemaVersion != FinancialSchemaPhase12 {
			t.Fatal("fixture must use schema 1")
		}
		if _, err := NewDraft(params); err != nil {
			t.Fatalf("explicit schema 1 payroll: %v", err)
		}
	})
	t.Run("swap_schema_1_accepted", func(t *testing.T) {
		params := phase12SwapParams()
		if params.Financial.Swap.SchemaVersion != FinancialSchemaPhase12 {
			t.Fatal("fixture must use schema 1")
		}
		if _, err := NewDraft(params); err != nil {
			t.Fatalf("explicit schema 1 swap: %v", err)
		}
	})
	t.Run("payroll_schema_2_rejected", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll.SchemaVersion = 2
		if _, err := NewDraft(params); err == nil {
			t.Fatal("unsupported payroll schema must be rejected")
		}
	})
	t.Run("swap_schema_2_rejected", func(t *testing.T) {
		params := phase12SwapParams()
		params.Financial.Swap.SchemaVersion = 2
		if _, err := NewDraft(params); err == nil {
			t.Fatal("unsupported swap schema must be rejected")
		}
	})
	t.Run("normalize_does_not_stamp_schema", func(t *testing.T) {
		params := phase12PayrollParams()
		params.Financial.Payroll.SchemaVersion = 0
		normalized := normalizeParams(params)
		if normalized.Financial.Payroll.SchemaVersion != 0 {
			t.Fatalf("normalizeParams stamped schema_version = %d", normalized.Financial.Payroll.SchemaVersion)
		}
		swap := phase12SwapParams()
		swap.Financial.Swap.SchemaVersion = 0
		normalized = normalizeParams(swap)
		if normalized.Financial.Swap.SchemaVersion != 0 {
			t.Fatalf("normalizeParams stamped swap schema_version = %d", normalized.Financial.Swap.SchemaVersion)
		}
	})
}

func TestMinimumOutputCeiling(t *testing.T) {
	mustEq := func(t *testing.T, got *big.Int, want string) {
		t.Helper()
		if got.String() != want {
			t.Fatalf("got %s, want %s", got.String(), want)
		}
	}
	t.Run("exact_division", func(t *testing.T) {
		// 10000 * 9900 / 10000 = 9900 exactly
		mustEq(t, minimumOutputCeiling(big.NewInt(10000), 100), "9900")
	})
	t.Run("fractional_division_ceil", func(t *testing.T) {
		// 10001 * 9999 / 10000 = 9999.9999 → ceil 10000
		mustEq(t, minimumOutputCeiling(big.NewInt(10001), 1), "10000")
	})
	t.Run("bps_0", func(t *testing.T) {
		mustEq(t, minimumOutputCeiling(big.NewInt(12345), 0), "12345")
	})
	t.Run("bps_1", func(t *testing.T) {
		mustEq(t, minimumOutputCeiling(big.NewInt(10001), 1), "10000")
	})
	t.Run("bps_10000", func(t *testing.T) {
		mustEq(t, minimumOutputCeiling(big.NewInt(999999), 10000), "0")
	})
	t.Run("large_uint256", func(t *testing.T) {
		// 2^256-1 style magnitude: ensure no panic / float path.
		huge, ok := new(big.Int).SetString("115792089237316195423570985008687907853269984665640564039457584007913129639935", 10)
		if !ok {
			t.Fatal("parse huge")
		}
		got := minimumOutputCeiling(huge, 1)
		// ceil(huge * 9999 / 10000) must be > 0 and <= huge
		if got.Sign() <= 0 || got.Cmp(huge) > 0 {
			t.Fatalf("large ceiling out of range: %s", got.String())
		}
	})
}

func TestPhase12SwapSlippageCeilingRegression10001(t *testing.T) {
	// Expected = 10001, MaxSlippageBPS = 1 → required min = ceil(10001*9999/10000) = 10000.
	// Floor-based validation would incorrectly accept 9999.
	params := phase12SwapParams()
	params.Financial.Swap.ExpectedOutput = Amount{Decimal: "0.010001", BaseUnits: "10001", Decimals: 6}
	params.Financial.Swap.MaxSlippageBPS = 1
	params.Financial.Swap.Quote.ExpectedAmountOut = params.Financial.Swap.ExpectedOutput

	t.Run("9999_rejected", func(t *testing.T) {
		p := params
		swap := *p.Financial.Swap
		q := *swap.Quote
		swap.Quote = &q
		swap.MinimumOutput = Amount{Decimal: "0.009999", BaseUnits: "9999", Decimals: 6}
		swap.Quote.MinAmountOut = swap.MinimumOutput
		p.Financial.Swap = &swap
		if _, err := NewDraft(p); err == nil {
			t.Fatal("minimum 9999 must be rejected for expected 10001 at 1 bps")
		}
	})
	t.Run("10000_accepted", func(t *testing.T) {
		p := params
		swap := *p.Financial.Swap
		q := *swap.Quote
		swap.Quote = &q
		swap.MinimumOutput = Amount{Decimal: "0.01", BaseUnits: "10000", Decimals: 6}
		swap.Quote.MinAmountOut = swap.MinimumOutput
		p.Financial.Swap = &swap
		if _, err := NewDraft(p); err != nil {
			t.Fatalf("minimum 10000 must be accepted: %v", err)
		}
	})
}

func TestHistoricalDigestIdentityThroughRestore(t *testing.T) {
	// Digest before persistence and after restore must be identical for legacy shapes.
	payroll := mustTransition(t, mustDraft(t, testParams()), StatusCreated, testNow.Add(time.Second))
	before := payroll.Digest()
	finJSON, err := json.Marshal(payroll.Financial())
	if err != nil {
		t.Fatal(err)
	}
	var fin FinancialParameters
	if err := json.Unmarshal(finJSON, &fin); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(Params{
		IntentID: payroll.IntentID(), Version: payroll.Version(), ClientRequestID: payroll.ClientRequestID(),
		Nonce: payroll.Nonce(), Type: payroll.Type(), Ownership: payroll.Ownership(), Financial: fin,
		Route: payroll.Route(), Constraints: payroll.Constraints(), CreatedAt: payroll.CreatedAt(), ExpiresAt: payroll.ExpiresAt(),
	}, payroll.Status(), payroll.Digest(), payroll.LifecycleRevision())
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Digest() != before {
		t.Fatalf("digest before %s after %s", before, restored.Digest())
	}
	if restored.Financial().Payroll.SchemaVersion != 0 {
		t.Fatal("schema upgraded on restore")
	}
	if restored.Financial().Payroll.Phase12Executable() {
		t.Fatal("legacy became executable")
	}

	// Phase 12 create → restore also keeps digest (schema 1 already explicit).
	p12 := mustTransition(t, mustDraft(t, phase12PayrollParams()), StatusCreated, testNow.Add(time.Second))
	p12JSON, err := json.Marshal(p12.Financial())
	if err != nil {
		t.Fatal(err)
	}
	fin = FinancialParameters{}
	if err := json.Unmarshal(p12JSON, &fin); err != nil {
		t.Fatal(err)
	}
	restoredP12, err := Restore(Params{
		IntentID: p12.IntentID(), Version: p12.Version(), ClientRequestID: p12.ClientRequestID(),
		Nonce: p12.Nonce(), Type: p12.Type(), Ownership: p12.Ownership(), Financial: fin,
		Route: p12.Route(), Constraints: p12.Constraints(), CreatedAt: p12.CreatedAt(), ExpiresAt: p12.ExpiresAt(),
	}, p12.Status(), p12.Digest(), p12.LifecycleRevision())
	if err != nil {
		t.Fatalf("phase12 restore: %v", err)
	}
	if restoredP12.Digest() != p12.Digest() {
		t.Fatal("phase12 digest changed after restore")
	}
	if restoredP12.Financial().Payroll.SchemaVersion != FinancialSchemaPhase12 {
		t.Fatal("phase12 schema lost on restore")
	}
}
