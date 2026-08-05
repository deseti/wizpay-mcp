// Package intents defines provider-neutral, pre-execution financial intent values.
// It does not sign, submit, or execute financial operations.
package intents

import (
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxTextLength = 256

// Type discriminates the only financial parameter shapes accepted in Phase 3+.
type Type string

const (
	TypePayroll         Type = "PAYROLL"
	TypeSwap            Type = "SWAP"
	TypeBridge          Type = "BRIDGE"
	TypeANSRegistration Type = "ANS_REGISTRATION"
)

func (t Type) Valid() bool {
	switch t {
	case TypePayroll, TypeSwap, TypeBridge, TypeANSRegistration:
		return true
	default:
		return false
	}
}

// Amount carries an exact human decimal and its exact integer base-unit value.
// Floating-point financial values are intentionally unsupported.
type Amount struct {
	Decimal   string `json:"decimal"`
	BaseUnits string `json:"base_units"`
	Decimals  uint8  `json:"decimals"`
}

func (a Amount) Validate() error {
	if a.Decimals > 36 {
		return fmt.Errorf("amount decimals cannot exceed 36")
	}
	want, err := decimalToBaseUnits(a.Decimal, a.Decimals)
	if err != nil {
		return err
	}
	if a.BaseUnits == "" || (len(a.BaseUnits) > 1 && a.BaseUnits[0] == '0') {
		return fmt.Errorf("amount base units must be a canonical non-negative integer")
	}
	got, ok := new(big.Int).SetString(a.BaseUnits, 10)
	if !ok || got.Sign() < 0 {
		return fmt.Errorf("amount base units must be a canonical non-negative integer")
	}
	if want.Cmp(got) != 0 {
		return fmt.Errorf("amount decimal and base units do not match")
	}
	return nil
}

// IsZero reports whether the amount is the zero value (unset for omitempty).
func (a Amount) IsZero() bool {
	return a.Decimal == "" && a.BaseUnits == "" && a.Decimals == 0
}

// IsPositive reports whether the amount is a valid positive value.
func (a Amount) IsPositive() bool {
	if err := a.Validate(); err != nil {
		return false
	}
	value, ok := new(big.Int).SetString(a.BaseUnits, 10)
	return ok && value.Sign() > 0
}

// BaseInt returns the base-unit integer when the amount is valid.
func (a Amount) BaseInt() (*big.Int, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	value, ok := new(big.Int).SetString(a.BaseUnits, 10)
	if !ok {
		return nil, fmt.Errorf("amount base units are invalid")
	}
	return value, nil
}

func decimalToBaseUnits(value string, decimals uint8) (*big.Int, error) {
	if value == "" || strings.TrimSpace(value) != value || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return nil, fmt.Errorf("amount decimal must be a canonical non-negative decimal string")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts[0]) > 1 && parts[0][0] == '0') {
		return nil, fmt.Errorf("amount decimal must be a canonical non-negative decimal string")
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return nil, fmt.Errorf("amount decimal must be a canonical non-negative decimal string")
		}
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" || len(fraction) > int(decimals) || fraction[len(fraction)-1] == '0' {
			return nil, fmt.Errorf("amount decimal has a non-canonical fractional component")
		}
		for _, r := range fraction {
			if r < '0' || r > '9' {
				return nil, fmt.Errorf("amount decimal must contain digits only")
			}
		}
	}
	digits := parts[0] + fraction + strings.Repeat("0", int(decimals)-len(fraction))
	result, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, fmt.Errorf("amount decimal is invalid")
	}
	return result, nil
}

// Token identifies an asset on one explicit chain.
type Token struct {
	ChainID  string `json:"chain_id"`
	Standard string `json:"standard"`
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Decimals uint8  `json:"decimals"`
}

func (t Token) IsZero() bool {
	return t.ChainID == "" && t.Standard == "" && t.Address == "" && t.Symbol == "" && t.Decimals == 0
}

func (t Token) Validate() error {
	if err := validateChainID(t.ChainID); err != nil {
		return err
	}
	if err := validateText("token standard", t.Standard); err != nil {
		return err
	}
	if err := validateText("token address", t.Address); err != nil {
		return err
	}
	if err := validateText("token symbol", t.Symbol); err != nil {
		return err
	}
	if t.Decimals > 36 {
		return fmt.Errorf("token decimals cannot exceed 36")
	}
	return nil
}

// ValidatePhase12Token requires a non-zero EVM token address in addition to Token.Validate.
func (t Token) ValidatePhase12Token(name string) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := validateNonZeroEVMAddress(name+" address", t.Address); err != nil {
		return err
	}
	return nil
}

// PayrollVariant selects the allowlisted Payroll contract execution shape.
type PayrollVariant string

const (
	PayrollVariantSingle              PayrollVariant = "SINGLE"
	PayrollVariantBatchSingleTokenOut PayrollVariant = "BATCH_SINGLE_TOKEN_OUT"
	PayrollVariantBatchMultiTokenOut  PayrollVariant = "BATCH_MULTI_TOKEN_OUT"
)

func (v PayrollVariant) Valid() bool {
	switch v {
	case PayrollVariantSingle, PayrollVariantBatchSingleTokenOut, PayrollVariantBatchMultiTokenOut:
		return true
	default:
		return false
	}
}

// Schema version constants for financial parameter shapes.
const (
	// FinancialSchemaLegacy is the pre-Phase-12 shape (readable, not Phase-12-executable).
	FinancialSchemaLegacy uint8 = 0
	// FinancialSchemaPhase12 is the Phase 12 execution-critical shape.
	FinancialSchemaPhase12 uint8 = 1
)

// Recipient is one payroll payment line.
//
// Legacy (schema 0): Address + Amount only.
// Phase 12 (schema 1): Address + TokenOut + AmountIn + MinAmountOut.
type Recipient struct {
	Address string `json:"address"`
	// Amount is the legacy Phase 3 recipient amount. It is omitempty so Phase 12
	// lines do not re-emit it.
	Amount Amount `json:"amount,omitempty"`
	// TokenOut is the Phase 12 destination token for this line.
	TokenOut Token `json:"token_out,omitempty"`
	// AmountIn is the Phase 12 input amount for this line.
	AmountIn Amount `json:"amount_in,omitempty"`
	// MinAmountOut is the Phase 12 minimum output for this line.
	// It must be explicitly set and strictly positive; zero is rejected and
	// never silently defaulted.
	MinAmountOut Amount `json:"min_amount_out,omitempty"`
}

// PayrollParameters is the PAYROLL financial payload.
//
// SchemaVersion 0 with no Phase 12 material: legacy shape (Token, Recipients with
// Amount, Total). SchemaVersion must be FinancialSchemaPhase12 (1) for any Phase
// 12-shaped payload; mixed Phase 12 fields with schema 0 are rejected on create.
// IsPhase12 may still detect Phase 12 field presence when schema is 0 so mixed
// payloads fail closed rather than falling back to legacy validation.
//
// Old shapes remain readable and digest-stable but are not Phase12Executable.
type PayrollParameters struct {
	SchemaVersion uint8          `json:"schema_version,omitempty"`
	Variant       PayrollVariant `json:"variant,omitempty"`
	// TokenIn is the Phase 12 source token.
	TokenIn Token `json:"token_in,omitempty"`
	// Token is the legacy Phase 3 source token.
	Token Token `json:"token,omitempty"`
	// Recipients are ordered and digest-significant. Order is preserved.
	Recipients []Recipient `json:"recipients"`
	// Total must equal the sum of recipient input amounts.
	Total Amount `json:"total"`
	// ReferenceID is required for batch variants and is immutable intent metadata.
	// For SINGLE it is optional (routeAndPay has no on-chain referenceId).
	ReferenceID string `json:"reference_id,omitempty"`
}

// SwapQuote freezes quote evidence that must match the approved economic material.
type SwapQuote struct {
	QuoteID           string    `json:"quote_id"`
	Source            string    `json:"source"`
	ExpectedAmountOut Amount    `json:"expected_amount_out"`
	MinAmountOut      Amount    `json:"min_amount_out"`
	Router            string    `json:"router"`
	ExpiresAt         time.Time `json:"expires_at"`
	EvidenceReference string    `json:"evidence_reference"`
}

// SwapParameters is the SWAP financial payload.
//
// SchemaVersion 0 with no Phase 12 material: legacy shape (tokens, amounts,
// quote_reference, slippage). SchemaVersion must be FinancialSchemaPhase12 (1)
// for Phase 12-shaped payloads; schema 0 with router/recipient/quote/deadline
// is rejected on create (never auto-upgraded).
type SwapParameters struct {
	SchemaVersion  uint8  `json:"schema_version,omitempty"`
	InputToken     Token  `json:"input_token"`
	OutputToken    Token  `json:"output_token"`
	InputAmount    Amount `json:"input_amount"`
	ExpectedOutput Amount `json:"expected_output"`
	MinimumOutput  Amount `json:"minimum_output"`
	// QuoteReference is the legacy opaque quote id. Phase 12 prefers Quote.QuoteID.
	QuoteReference string `json:"quote_reference,omitempty"`
	MaxSlippageBPS uint16 `json:"max_slippage_bps"`
	// Phase 12 execution-critical fields.
	Router    string     `json:"router,omitempty"`
	Recipient string     `json:"recipient,omitempty"`
	Quote     *SwapQuote `json:"quote,omitempty"`
	// Deadline is the executeSwap unix deadline boundary as RFC3339 time in
	// canonical material. It is independent of Constraints.Deadline but must
	// not permit execution after the quote expires.
	Deadline time.Time `json:"deadline,omitempty"`
}

// FinancialParameters is a closed discriminated union. Exactly one member must
// be present and it must match the enclosing intent Type.
type FinancialParameters struct {
	Payroll *PayrollParameters `json:"payroll,omitempty"`
	Swap    *SwapParameters    `json:"swap,omitempty"`
	Bridge  *BridgeParameters  `json:"bridge,omitempty"`
	ANS     *ANSParameters     `json:"ans_registration,omitempty"`
}

func (p FinancialParameters) validate(kind Type) error {
	count := 0
	if p.Payroll != nil {
		count++
	}
	if p.Swap != nil {
		count++
	}
	if p.Bridge != nil {
		count++
	}
	if p.ANS != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("financial parameters must contain exactly one typed payload")
	}
	switch kind {
	case TypePayroll:
		if p.Payroll == nil {
			return fmt.Errorf("PAYROLL requires payroll parameters")
		}
		return p.Payroll.validate()
	case TypeSwap:
		if p.Swap == nil {
			return fmt.Errorf("SWAP requires swap parameters")
		}
		return p.Swap.validate()
	case TypeBridge:
		if p.Bridge == nil {
			return fmt.Errorf("BRIDGE requires bridge parameters")
		}
		return p.Bridge.validate()
	case TypeANSRegistration:
		if p.ANS == nil {
			return fmt.Errorf("ANS_REGISTRATION requires ANS parameters")
		}
		return p.ANS.validate()
	default:
		return fmt.Errorf("invalid intent type %q", kind)
	}
}

func cloneFinancial(p FinancialParameters) FinancialParameters {
	result := p
	if p.Payroll != nil {
		value := *p.Payroll
		value.Recipients = append([]Recipient(nil), p.Payroll.Recipients...)
		result.Payroll = &value
	}
	if p.Swap != nil {
		value := *p.Swap
		if p.Swap.Quote != nil {
			quote := *p.Swap.Quote
			value.Quote = &quote
		}
		result.Swap = &value
	}
	if p.Bridge != nil {
		value := *p.Bridge
		result.Bridge = &value
	}
	if p.ANS != nil {
		value := *p.ANS
		result.ANS = &value
	}
	return result
}

func validateText(name, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", name)
	}
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is required and must not contain surrounding whitespace", name)
	}
	if len(value) > maxTextLength {
		return fmt.Errorf("%s exceeds %d characters", name, maxTextLength)
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
	for _, r := range value {
		if r < '0' || r > '9' {
			return fmt.Errorf("chain ID must be a canonical positive decimal string")
		}
	}
	return nil
}
