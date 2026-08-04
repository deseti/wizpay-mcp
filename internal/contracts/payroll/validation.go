package payroll

import (
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

const (
	maxReferenceIDLength = 128
	maxBatchRecipients   = 256
)

func validateDeployment(deployment contracts.Deployment) error {
	if deployment.ID != contracts.ContractWizPayPayroll {
		return fmt.Errorf("deployment is not WIZPAY_PAYROLL")
	}
	if deployment.RegistryVersion != contracts.RegistryVersion {
		return fmt.Errorf("unexpected payroll registry version %d", deployment.RegistryVersion)
	}
	if deployment.ChainID != contracts.ChainIDArcTestnet {
		return fmt.Errorf("payroll deployment chain ID mismatch")
	}
	if deployment.Network != contracts.NetworkArcTestnet {
		return fmt.Errorf("payroll deployment network mismatch")
	}
	if !contracts.AddressesEqual(deployment.Address, contracts.AddressWizPayPayroll) {
		return fmt.Errorf("payroll deployment address mismatch")
	}
	if deployment.Status != contracts.StatusEnabled {
		return fmt.Errorf("payroll deployment is not enabled")
	}
	return nil
}

func validateTokenAddress(name, value string) error {
	if !contracts.ValidAddress(value) {
		return fmt.Errorf("%s is not a valid address", name)
	}
	if isZeroAddress(value) {
		return fmt.Errorf("%s must not be the zero address", name)
	}
	return nil
}

func validateRecipientAddress(name, value string) error {
	if !contracts.ValidAddress(value) {
		return fmt.Errorf("%s is not a valid address", name)
	}
	if isZeroAddress(value) {
		return fmt.Errorf("%s must not be the zero address", name)
	}
	return nil
}

func validatePositiveAmount(name string, value *big.Int) error {
	if value == nil {
		return fmt.Errorf("%s is required", name)
	}
	if value.Sign() <= 0 {
		return fmt.Errorf("%s must be greater than zero", name)
	}
	return nil
}

func validateNonNegativeAmount(name string, value *big.Int) error {
	if value == nil {
		return fmt.Errorf("%s is required", name)
	}
	if value.Sign() < 0 {
		return fmt.Errorf("%s must not be negative", name)
	}
	return nil
}

func validateReferenceID(value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("referenceId must be non-empty trimmed text")
	}
	if utf8.RuneCountInString(value) > maxReferenceIDLength {
		return fmt.Errorf("referenceId exceeds %d characters", maxReferenceIDLength)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("referenceId contains control characters")
		}
	}
	return nil
}

func validateBatchArrays(recipients, tokenOuts []string, amountsIn, minAmountsOut []*big.Int, multiTokenOut bool) error {
	if len(recipients) == 0 {
		return fmt.Errorf("recipients must be non-empty")
	}
	if len(recipients) > maxBatchRecipients {
		return fmt.Errorf("recipients exceed maximum batch size %d", maxBatchRecipients)
	}
	if len(amountsIn) != len(recipients) {
		return fmt.Errorf("amountsIn length must match recipients length")
	}
	if len(minAmountsOut) != len(recipients) {
		return fmt.Errorf("minAmountsOut length must match recipients length")
	}
	if multiTokenOut {
		if len(tokenOuts) != len(recipients) {
			return fmt.Errorf("tokenOuts length must match recipients length")
		}
	}
	for i := range recipients {
		if err := validateRecipientAddress(fmt.Sprintf("recipients[%d]", i), recipients[i]); err != nil {
			return err
		}
		if err := validatePositiveAmount(fmt.Sprintf("amountsIn[%d]", i), amountsIn[i]); err != nil {
			return err
		}
		if err := validateNonNegativeAmount(fmt.Sprintf("minAmountsOut[%d]", i), minAmountsOut[i]); err != nil {
			return err
		}
		if multiTokenOut {
			if err := validateTokenAddress(fmt.Sprintf("tokenOuts[%d]", i), tokenOuts[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateBatchMulti(in BatchMultiTokenOut) error {
	if err := validateTokenAddress("tokenIn", in.TokenIn); err != nil {
		return err
	}
	if err := validateReferenceID(in.ReferenceID); err != nil {
		return err
	}
	return validateBatchArrays(in.Recipients, in.TokenOuts, in.AmountsIn, in.MinAmountsOut, true)
}

func validateBatchSingle(in BatchSingleTokenOut) error {
	if err := validateTokenAddress("tokenIn", in.TokenIn); err != nil {
		return err
	}
	if err := validateTokenAddress("tokenOut", in.TokenOut); err != nil {
		return err
	}
	if err := validateReferenceID(in.ReferenceID); err != nil {
		return err
	}
	return validateBatchArrays(in.Recipients, nil, in.AmountsIn, in.MinAmountsOut, false)
}

func validateSingle(in SinglePayment) error {
	if err := validateTokenAddress("tokenIn", in.TokenIn); err != nil {
		return err
	}
	if err := validateTokenAddress("tokenOut", in.TokenOut); err != nil {
		return err
	}
	if err := validatePositiveAmount("amountIn", in.AmountIn); err != nil {
		return err
	}
	if err := validateNonNegativeAmount("minAmountOut", in.MinAmountOut); err != nil {
		return err
	}
	return validateRecipientAddress("recipient", in.Recipient)
}

func isZeroAddress(value string) bool {
	return contracts.NormalizeAddress(value) == "0x0000000000000000000000000000000000000000"
}

// RejectAdminSelector is a defensive check: admin selectors must never appear
// as the product of this package's encoders.
func RejectAdminSelector(selector [4]byte) error {
	for _, name := range AdminFunctionNames {
		// Admin signatures vary; reject by matching known full ABI selectors
		// computed from name alone is incomplete. Instead, reject if the
		// selector is not one of the three allowlisted execution selectors.
		_ = name
	}
	allowed := map[[4]byte]struct{}{}
	for _, signature := range []string{SigBatchMultiTokenOut, SigBatchSingleTokenOut, SigRouteAndPay} {
		sel, err := Selector(signature)
		if err != nil {
			return err
		}
		allowed[sel] = struct{}{}
	}
	if _, ok := allowed[selector]; !ok {
		return fmt.Errorf("selector is not an allowlisted payroll execution selector")
	}
	return nil
}
