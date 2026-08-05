package intents

import (
	"fmt"
	"math/big"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

// Route reference constants for allowlisted Swap contract binding.
const (
	// RouteReferenceSwap is the canonical intent route reference for Swap.
	RouteReferenceSwap = string(contracts.ContractWizPaySwapExecutor)
	// RouteVersionSwap is the MCP registry version bound into approved intents.
	RouteVersionSwap = uint64(contracts.RegistryVersion)
)

// IsPhase12 reports whether this payload uses the Phase 12 execution-critical shape.
func (p SwapParameters) IsPhase12() bool {
	if p.SchemaVersion >= FinancialSchemaPhase12 {
		return true
	}
	if p.Router != "" || p.Recipient != "" || p.Quote != nil || !p.Deadline.IsZero() {
		return true
	}
	return false
}

// Phase12Executable reports whether the payload is a fully validated Phase 12
// shape ready for later planner/encoding work. Legacy shapes return false.
func (p SwapParameters) Phase12Executable() bool {
	return p.IsPhase12() && p.validatePhase12(time.Time{}, time.Time{}, time.Time{}) == nil
}

func (p SwapParameters) validate() error {
	// Time-independent structural validation. Intent-level timing (CreatedAt,
	// Constraints.Deadline, ExpiresAt) is applied in validateParams.
	if p.IsPhase12() {
		return p.validatePhase12(time.Time{}, time.Time{}, time.Time{})
	}
	return p.validateLegacy()
}

// validateWithTimeline applies Phase 12 quote/deadline relationships using the
// intent envelope clock. createdAt is the freeze clock (not wall clock).
func (p SwapParameters) validateWithTimeline(createdAt, constraintDeadline, expiresAt time.Time) error {
	if !p.IsPhase12() {
		return p.validateLegacy()
	}
	return p.validatePhase12(createdAt, constraintDeadline, expiresAt)
}

func (p SwapParameters) validateLegacy() error {
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
	}{
		{"input", p.InputAmount, p.InputToken.Decimals},
		{"expected output", p.ExpectedOutput, p.OutputToken.Decimals},
		{"minimum output", p.MinimumOutput, p.OutputToken.Decimals},
	} {
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

func (p SwapParameters) validatePhase12(createdAt, constraintDeadline, expiresAt time.Time) error {
	// Phase 12 material (detected via IsPhase12) requires an explicit schema
	// version. Schema 0 with Phase 12 fields is rejected; it is never upgraded.
	if p.SchemaVersion != FinancialSchemaPhase12 {
		if p.SchemaVersion == FinancialSchemaLegacy {
			return fmt.Errorf("phase 12 swap requires explicit schema_version %d", FinancialSchemaPhase12)
		}
		return fmt.Errorf("unsupported swap schema version %d", p.SchemaVersion)
	}
	if err := p.InputToken.ValidatePhase12Token("input_token"); err != nil {
		return err
	}
	if err := p.OutputToken.ValidatePhase12Token("output_token"); err != nil {
		return err
	}
	if p.InputToken.ChainID != p.OutputToken.ChainID {
		return fmt.Errorf("swap input and output tokens must share the same chain")
	}
	if addressesEqual(p.InputToken.Address, p.OutputToken.Address) {
		return fmt.Errorf("swap input and output tokens must differ")
	}
	if !p.InputAmount.IsPositive() {
		return fmt.Errorf("input amount must be positive")
	}
	if p.InputAmount.Decimals != p.InputToken.Decimals {
		return fmt.Errorf("input amount decimals do not match input token")
	}
	if !p.ExpectedOutput.IsPositive() {
		return fmt.Errorf("expected output must be positive")
	}
	if p.ExpectedOutput.Decimals != p.OutputToken.Decimals {
		return fmt.Errorf("expected output decimals do not match output token")
	}
	if !p.MinimumOutput.IsPositive() {
		return fmt.Errorf("minimum output must be positive")
	}
	if p.MinimumOutput.Decimals != p.OutputToken.Decimals {
		return fmt.Errorf("minimum output decimals do not match output token")
	}
	if p.MaxSlippageBPS > 10_000 {
		return fmt.Errorf("maximum slippage cannot exceed 10000 basis points")
	}
	expected, err := p.ExpectedOutput.BaseInt()
	if err != nil {
		return fmt.Errorf("expected output: %w", err)
	}
	minimum, err := p.MinimumOutput.BaseInt()
	if err != nil {
		return fmt.Errorf("minimum output: %w", err)
	}
	if minimum.Cmp(expected) > 0 {
		return fmt.Errorf("minimum output cannot exceed expected output")
	}
	// Minimum output must not be weaker than the approved slippage ceiling:
	// min >= ceil(expected * (10000 - bps) / 10000)
	// Ceiling division prevents accepting a min slightly below the economic
	// floor that floor-division would have permitted on fractional results.
	requiredMin := minimumOutputCeiling(expected, p.MaxSlippageBPS)
	if minimum.Cmp(requiredMin) < 0 {
		return fmt.Errorf("minimum output is weaker than max_slippage_bps allows")
	}

	if err := validateNonZeroEVMAddress("router", p.Router); err != nil {
		return err
	}
	if err := validateNonZeroEVMAddress("recipient", p.Recipient); err != nil {
		return err
	}
	if p.Quote == nil {
		return fmt.Errorf("swap quote is required")
	}
	if err := p.Quote.validate(p.OutputToken.Decimals); err != nil {
		return err
	}
	if !addressesEqual(p.Quote.Router, p.Router) {
		return fmt.Errorf("quote router must equal frozen router")
	}
	if p.Quote.ExpectedAmountOut.BaseUnits != p.ExpectedOutput.BaseUnits ||
		p.Quote.ExpectedAmountOut.Decimals != p.ExpectedOutput.Decimals ||
		p.Quote.ExpectedAmountOut.Decimal != p.ExpectedOutput.Decimal {
		return fmt.Errorf("quote expected output must equal frozen expected output")
	}
	if p.Quote.MinAmountOut.BaseUnits != p.MinimumOutput.BaseUnits ||
		p.Quote.MinAmountOut.Decimals != p.MinimumOutput.Decimals ||
		p.Quote.MinAmountOut.Decimal != p.MinimumOutput.Decimal {
		return fmt.Errorf("quote minimum output must equal frozen minimum output")
	}
	if p.QuoteReference != "" && p.QuoteReference != p.Quote.QuoteID {
		return fmt.Errorf("quote_reference must match quote_id when both are set")
	}
	if p.Deadline.IsZero() {
		return fmt.Errorf("swap deadline is required")
	}
	pDeadline := p.Deadline.UTC()

	// Timeline checks use the intent freeze clock when provided. Digest
	// generation never injects wall-clock time; callers pass CreatedAt.
	if !createdAt.IsZero() {
		createdAt = createdAt.UTC()
		if !p.Quote.ExpiresAt.After(createdAt) {
			return fmt.Errorf("quote has already expired at freeze time")
		}
		if !pDeadline.After(createdAt) {
			return fmt.Errorf("swap deadline must follow creation")
		}
	}
	if !p.Quote.ExpiresAt.IsZero() && pDeadline.After(p.Quote.ExpiresAt.UTC()) {
		// Executing after quote expiry would use stale economic material.
		return fmt.Errorf("swap deadline cannot be after quote expiry")
	}
	if !constraintDeadline.IsZero() && pDeadline.After(constraintDeadline.UTC()) {
		return fmt.Errorf("swap deadline cannot exceed constraint deadline")
	}
	if !expiresAt.IsZero() && pDeadline.After(expiresAt.UTC()) {
		return fmt.Errorf("swap deadline cannot exceed intent expiration")
	}
	return nil
}

func (q SwapQuote) validate(outputDecimals uint8) error {
	if err := validateText("quote_id", q.QuoteID); err != nil {
		return err
	}
	if err := validateText("quote source", q.Source); err != nil {
		return err
	}
	if err := validateText("quote evidence_reference", q.EvidenceReference); err != nil {
		return err
	}
	if err := validateNonZeroEVMAddress("quote router", q.Router); err != nil {
		return err
	}
	if q.ExpiresAt.IsZero() {
		return fmt.Errorf("quote expires_at is required")
	}
	if !q.ExpectedAmountOut.IsPositive() {
		return fmt.Errorf("quote expected amount out must be positive")
	}
	if q.ExpectedAmountOut.Decimals != outputDecimals {
		return fmt.Errorf("quote expected amount out decimals do not match output token")
	}
	if !q.MinAmountOut.IsPositive() {
		return fmt.Errorf("quote minimum amount out must be positive")
	}
	if q.MinAmountOut.Decimals != outputDecimals {
		return fmt.Errorf("quote minimum amount out decimals do not match output token")
	}
	min, err := q.MinAmountOut.BaseInt()
	if err != nil {
		return err
	}
	expected, err := q.ExpectedAmountOut.BaseInt()
	if err != nil {
		return err
	}
	if min.Cmp(expected) > 0 {
		return fmt.Errorf("quote minimum amount out cannot exceed expected amount out")
	}
	return nil
}

// minimumOutputCeiling returns ceil(expected * (10000 - bps) / 10000) using
// overflow-safe big.Int arithmetic only (no floating point).
//
// For non-negative numerator and positive divisor:
//
//	ceil(n/d) = (n + d - 1) / d
//
// When bps == 10000 the product is zero and the ceiling is zero; callers still
// require MinimumOutput itself to be strictly positive.
func minimumOutputCeiling(expected *big.Int, maxSlippageBPS uint16) *big.Int {
	if expected == nil {
		return big.NewInt(0)
	}
	// bps is at most 10000 by prior validation.
	remaining := int64(10_000 - int64(maxSlippageBPS))
	num := new(big.Int).Mul(new(big.Int).Set(expected), big.NewInt(remaining))
	div := big.NewInt(10_000)
	// ceil(num/div) for num >= 0, div > 0.
	num.Add(num, new(big.Int).Sub(div, big.NewInt(1)))
	return num.Div(num, div)
}
