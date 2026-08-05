package intents

import (
	"fmt"
	"math/big"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

const (
	// maxPayrollRecipientsPhase12 matches the contract encoder batch limit.
	maxPayrollRecipientsPhase12 = 256
	// maxPayrollRecipientsLegacy is the pre-Phase-12 limit.
	maxPayrollRecipientsLegacy = 500
	// maxPayrollReferenceIDLength matches contracts/payroll validation.
	maxPayrollReferenceIDLength = 128
)

// Route reference constants for allowlisted Payroll contract binding.
const (
	// RouteReferencePayroll is the canonical intent route reference for Payroll.
	RouteReferencePayroll = string(contracts.ContractWizPayPayroll)
	// RouteVersionPayroll is the MCP registry version bound into approved intents.
	RouteVersionPayroll = uint64(contracts.RegistryVersion)
)

// IsPhase12 reports whether this payload uses the Phase 12 execution-critical shape.
func (p PayrollParameters) IsPhase12() bool {
	if p.SchemaVersion >= FinancialSchemaPhase12 {
		return true
	}
	if p.Variant != "" {
		return true
	}
	if !p.TokenIn.IsZero() {
		return true
	}
	for _, recipient := range p.Recipients {
		if !recipient.TokenOut.IsZero() || !recipient.AmountIn.IsZero() || !recipient.MinAmountOut.IsZero() {
			return true
		}
	}
	return false
}

// Phase12Executable reports whether the payload is a fully validated Phase 12
// shape ready for later planner/encoding work. Legacy shapes return false.
func (p PayrollParameters) Phase12Executable() bool {
	return p.IsPhase12() && p.validatePhase12() == nil
}

// SourceToken returns the source token for policy/spend views.
func (p PayrollParameters) SourceToken() Token {
	if p.IsPhase12() && !p.TokenIn.IsZero() {
		return p.TokenIn
	}
	return p.Token
}

func (p PayrollParameters) validate() error {
	if p.IsPhase12() {
		return p.validatePhase12()
	}
	return p.validateLegacy()
}

// validateLegacy preserves pre-Phase-12 readability. It does not invent new
// execution-critical fields and is intentionally not Phase-12-executable.
func (p PayrollParameters) validateLegacy() error {
	if err := p.Token.Validate(); err != nil {
		return err
	}
	if len(p.Recipients) == 0 || len(p.Recipients) > maxPayrollRecipientsLegacy {
		return fmt.Errorf("payroll requires 1 to %d recipients", maxPayrollRecipientsLegacy)
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
		value, ok := new(big.Int).SetString(recipient.Amount.BaseUnits, 10)
		if !ok {
			return fmt.Errorf("recipient %d amount is invalid", i)
		}
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

func (p PayrollParameters) validatePhase12() error {
	// Phase 12 material (detected via IsPhase12) requires an explicit schema
	// version. Schema 0 with Phase 12 fields is rejected; it is never upgraded.
	if p.SchemaVersion != FinancialSchemaPhase12 {
		if p.SchemaVersion == FinancialSchemaLegacy {
			return fmt.Errorf("phase 12 payroll requires explicit schema_version %d", FinancialSchemaPhase12)
		}
		return fmt.Errorf("unsupported payroll schema version %d", p.SchemaVersion)
	}
	if !p.Variant.Valid() {
		return fmt.Errorf("invalid payroll variant %q", p.Variant)
	}
	if err := p.TokenIn.ValidatePhase12Token("token_in"); err != nil {
		return err
	}
	if len(p.Recipients) == 0 {
		return fmt.Errorf("payroll requires at least one recipient")
	}
	if len(p.Recipients) > maxPayrollRecipientsPhase12 {
		return fmt.Errorf("payroll recipients exceed maximum batch size %d", maxPayrollRecipientsPhase12)
	}
	if p.Variant == PayrollVariantSingle && len(p.Recipients) != 1 {
		return fmt.Errorf("SINGLE payroll requires exactly one recipient")
	}

	switch p.Variant {
	case PayrollVariantBatchSingleTokenOut, PayrollVariantBatchMultiTokenOut:
		if err := validateBoundedReferenceID("reference_id", p.ReferenceID, maxPayrollReferenceIDLength); err != nil {
			return err
		}
	case PayrollVariantSingle:
		if p.ReferenceID != "" {
			if err := validateBoundedReferenceID("reference_id", p.ReferenceID, maxPayrollReferenceIDLength); err != nil {
				return err
			}
		}
	}

	total := new(big.Int)
	seen := make(map[string]struct{}, len(p.Recipients))
	var singleTokenOut string

	for i, recipient := range p.Recipients {
		if err := validateNonZeroEVMAddress(fmt.Sprintf("recipients[%d].address", i), recipient.Address); err != nil {
			return err
		}
		normalized := normalizeEVMAddress(recipient.Address)
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("recipients[%d]: duplicate recipient address", i)
		}
		seen[normalized] = struct{}{}

		if err := recipient.TokenOut.ValidatePhase12Token(fmt.Sprintf("recipients[%d].token_out", i)); err != nil {
			return err
		}
		if recipient.TokenOut.ChainID != p.TokenIn.ChainID {
			return fmt.Errorf("recipients[%d].token_out chain does not match token_in chain", i)
		}
		if !recipient.AmountIn.IsPositive() {
			return fmt.Errorf("recipients[%d].amount_in must be a positive amount", i)
		}
		if recipient.AmountIn.Decimals != p.TokenIn.Decimals {
			return fmt.Errorf("recipients[%d].amount_in decimals do not match token_in", i)
		}
		// MinAmountOut must be explicitly present, structurally valid, and
		// strictly positive. Zero and omitted values are rejected; nothing is
		// defaulted.
		if !recipient.MinAmountOut.IsPositive() {
			return fmt.Errorf("recipients[%d].min_amount_out must be a positive amount", i)
		}
		if recipient.MinAmountOut.Decimals != recipient.TokenOut.Decimals {
			return fmt.Errorf("recipients[%d].min_amount_out decimals do not match token_out", i)
		}
		// Legacy Amount field must not reintroduce conflicting material.
		if !recipient.Amount.IsZero() {
			return fmt.Errorf("recipients[%d]: legacy amount field is not allowed on Phase 12 payroll lines", i)
		}

		value, err := recipient.AmountIn.BaseInt()
		if err != nil {
			return fmt.Errorf("recipients[%d].amount_in: %w", i, err)
		}
		total.Add(total, value)

		if p.Variant == PayrollVariantBatchSingleTokenOut {
			out := normalizeEVMAddress(recipient.TokenOut.Address)
			if i == 0 {
				singleTokenOut = out
			} else if out != singleTokenOut {
				return fmt.Errorf("BATCH_SINGLE_TOKEN_OUT requires identical token_out on every recipient")
			}
		}
	}

	if err := p.Total.Validate(); err != nil {
		return err
	}
	if p.Total.Decimals != p.TokenIn.Decimals {
		return fmt.Errorf("payroll total decimals do not match token_in")
	}
	if !p.Total.IsPositive() {
		return fmt.Errorf("payroll total must be positive")
	}
	if total.String() != p.Total.BaseUnits {
		return fmt.Errorf("payroll total does not match recipient amounts")
	}
	// Legacy Token field must not conflict when present.
	if !p.Token.IsZero() {
		if err := p.Token.Validate(); err != nil {
			return fmt.Errorf("legacy token: %w", err)
		}
		if !addressesEqual(p.Token.Address, p.TokenIn.Address) || p.Token.ChainID != p.TokenIn.ChainID ||
			p.Token.Decimals != p.TokenIn.Decimals || p.Token.Standard != p.TokenIn.Standard {
			return fmt.Errorf("legacy token does not match token_in")
		}
	}
	return nil
}
