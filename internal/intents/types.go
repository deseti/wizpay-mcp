// Package intents defines provider-neutral, pre-execution financial intent values.
// It does not sign, submit, or execute financial operations.
package intents

import (
	"fmt"
	"math/big"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxTextLength = 256

// Type discriminates the only financial parameter shapes accepted in Phase 3.
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

type Recipient struct {
	Address string `json:"address"`
	Amount  Amount `json:"amount"`
}

type PayrollParameters struct {
	Token      Token       `json:"token"`
	Recipients []Recipient `json:"recipients"`
	Total      Amount      `json:"total"`
}

type SwapParameters struct {
	InputToken     Token  `json:"input_token"`
	OutputToken    Token  `json:"output_token"`
	InputAmount    Amount `json:"input_amount"`
	ExpectedOutput Amount `json:"expected_output"`
	MinimumOutput  Amount `json:"minimum_output"`
	QuoteReference string `json:"quote_reference"`
	MaxSlippageBPS uint16 `json:"max_slippage_bps"`
}

type BridgeParameters struct {
	SourceChainID      string `json:"source_chain_id"`
	DestinationChainID string `json:"destination_chain_id"`
	SourceToken        Token  `json:"source_token"`
	SourceAmount       Amount `json:"source_amount"`
	DestinationAmount  Amount `json:"destination_amount"`
	DestinationAddress string `json:"destination_address"`
	PlanReference      string `json:"plan_reference"`
}

type ANSParameters struct {
	NormalizedName string `json:"normalized_name"`
	NameVersion    string `json:"name_version"`
	TermSeconds    uint64 `json:"term_seconds"`
	Controller     string `json:"controller"`
	CostToken      Token  `json:"cost_token"`
	Cost           Amount `json:"cost"`
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

func (p PayrollParameters) validate() error {
	if err := p.Token.Validate(); err != nil {
		return err
	}
	if len(p.Recipients) == 0 || len(p.Recipients) > 500 {
		return fmt.Errorf("payroll requires 1 to 500 recipients")
	}
	total := new(big.Int)
	for i, recipient := range p.Recipients {
		if err := validateText("recipient address", recipient.Address); err != nil {
			return fmt.Errorf("recipient %d: %w", i, err)
		}
		if err := recipient.Amount.Validate(); err != nil {
			return fmt.Errorf("recipient %d: %w", i, err)
		}
		if recipient.Amount.Decimals != p.Token.Decimals {
			return fmt.Errorf("recipient %d amount decimals do not match token", i)
		}
		value, _ := new(big.Int).SetString(recipient.Amount.BaseUnits, 10)
		total.Add(total, value)
	}
	if err := p.Total.Validate(); err != nil {
		return err
	}
	if p.Total.Decimals != p.Token.Decimals || total.String() != p.Total.BaseUnits {
		return fmt.Errorf("payroll total does not match recipient amounts")
	}
	return nil
}

func (p SwapParameters) validate() error {
	if err := p.InputToken.Validate(); err != nil {
		return err
	}
	if err := p.OutputToken.Validate(); err != nil {
		return err
	}
	for _, item := range []struct {
		name     string
		amount   Amount
		decimals uint8
	}{{"input", p.InputAmount, p.InputToken.Decimals}, {"expected output", p.ExpectedOutput, p.OutputToken.Decimals}, {"minimum output", p.MinimumOutput, p.OutputToken.Decimals}} {
		if err := item.amount.Validate(); err != nil {
			return fmt.Errorf("%s amount: %w", item.name, err)
		}
		if item.amount.Decimals != item.decimals {
			return fmt.Errorf("%s amount decimals do not match token", item.name)
		}
	}
	if err := validateText("quote reference", p.QuoteReference); err != nil {
		return err
	}
	if p.MaxSlippageBPS > 10_000 {
		return fmt.Errorf("maximum slippage cannot exceed 10000 basis points")
	}
	minimum, _ := new(big.Int).SetString(p.MinimumOutput.BaseUnits, 10)
	expected, _ := new(big.Int).SetString(p.ExpectedOutput.BaseUnits, 10)
	if minimum.Cmp(expected) > 0 {
		return fmt.Errorf("minimum output cannot exceed expected output")
	}
	return nil
}

func (p BridgeParameters) validate() error {
	if err := validateChainID(p.SourceChainID); err != nil {
		return err
	}
	if err := validateChainID(p.DestinationChainID); err != nil {
		return err
	}
	if p.SourceChainID == p.DestinationChainID {
		return fmt.Errorf("bridge source and destination chains must differ")
	}
	if err := p.SourceToken.Validate(); err != nil {
		return err
	}
	if p.SourceToken.ChainID != p.SourceChainID {
		return fmt.Errorf("bridge source token chain does not match source chain")
	}
	if err := p.SourceAmount.Validate(); err != nil {
		return err
	}
	if err := p.DestinationAmount.Validate(); err != nil {
		return err
	}
	if p.SourceAmount.Decimals != p.SourceToken.Decimals {
		return fmt.Errorf("bridge source amount decimals do not match token")
	}
	if err := validateText("destination address", p.DestinationAddress); err != nil {
		return err
	}
	return validateText("bridge plan reference", p.PlanReference)
}

func (p ANSParameters) validate() error {
	if err := validateText("normalized name", p.NormalizedName); err != nil {
		return err
	}
	if p.NormalizedName != strings.ToLower(p.NormalizedName) || strings.TrimSuffix(p.NormalizedName, ".") != p.NormalizedName {
		return fmt.Errorf("ANS name must be normalized lowercase without a trailing dot")
	}
	if err := validateText("name version", p.NameVersion); err != nil {
		return err
	}
	if p.TermSeconds == 0 {
		return fmt.Errorf("ANS term must be positive")
	}
	if err := validateText("ANS controller", p.Controller); err != nil {
		return err
	}
	if err := p.CostToken.Validate(); err != nil {
		return err
	}
	if err := p.Cost.Validate(); err != nil {
		return err
	}
	if p.Cost.Decimals != p.CostToken.Decimals {
		return fmt.Errorf("ANS cost decimals do not match token")
	}
	return nil
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
