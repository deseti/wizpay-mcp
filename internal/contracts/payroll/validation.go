package payroll

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

const (
	maxReferenceIDLength = 64
	maxBatchRecipients   = 50
)

func validateDeployment(deployment contracts.Deployment) error {
	if deployment.ID != contracts.ContractWizPayPayroll || deployment.RegistryVersion != contracts.RegistryVersion ||
		deployment.ChainID != contracts.ChainIDArcMainnet || deployment.Network != contracts.NetworkArcMainnet ||
		!contracts.AddressesEqual(deployment.Address, contracts.AddressWizPayPayroll) || deployment.Status != contracts.StatusEnabled {
		return fmt.Errorf("payroll deployment does not match the canonical Arc Mainnet descriptor")
	}
	return nil
}

func validateSameToken(in SameTokenPayrollInput) error {
	if !canonicalToken(in.Token) {
		return fmt.Errorf("token must be canonical Arc Mainnet USDC or EURC")
	}
	if err := validateBatch(in.Recipients, in.Amounts, in.ReferenceID); err != nil {
		return err
	}
	return nil
}

func validateCrossToken(in CrossTokenPayrollInput) error {
	if !canonicalPair(in.TokenIn, in.TokenOut) {
		return fmt.Errorf("token pair must be canonical Arc Mainnet USDC/EURC")
	}
	if err := validateBatch(in.Recipients, in.OutputAmounts, in.ReferenceID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(in.Recipients))
	for i, recipient := range in.Recipients {
		if contracts.AddressesEqual(recipient, contracts.AddressWizPayPayroll) {
			return fmt.Errorf("recipients[%d] cannot be the payroll contract", i)
		}
		normalized := contracts.NormalizeAddress(recipient)
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("recipients[%d] is duplicated", i)
		}
		seen[normalized] = struct{}{}
	}
	for _, field := range []struct {
		name  string
		value *big.Int
	}{
		{name: "grossInput", value: in.GrossInput},
		{name: "minTotalOut", value: in.MinTotalOut},
		{name: "minHopPriceX36", value: in.MinHopPriceX36},
		{name: "deadline", value: in.Deadline},
	} {
		if err := positiveUint128(field.name, field.value); err != nil {
			return err
		}
	}
	total := new(big.Int)
	for _, amount := range in.OutputAmounts {
		total.Add(total, amount)
	}
	if in.MinTotalOut.Cmp(total) < 0 {
		return fmt.Errorf("minTotalOut must cover exact output obligations")
	}
	return nil
}

func validateBatch(recipients []string, amounts []*big.Int, referenceID string) error {
	if len(recipients) == 0 || len(recipients) > maxBatchRecipients {
		return fmt.Errorf("recipient count must be between 1 and %d", maxBatchRecipients)
	}
	if len(amounts) != len(recipients) {
		return fmt.Errorf("amounts length must match recipients length")
	}
	// Solidity validates bytes(referenceId).length <= maxReferenceIDLength.
	// len() returns the UTF-8 byte length, which matches Solidity's bytes cast.
	if referenceID == "" || strings.TrimSpace(referenceID) != referenceID || len(referenceID) > maxReferenceIDLength {
		return fmt.Errorf("referenceId must be non-empty trimmed text of at most %d bytes", maxReferenceIDLength)
	}
	for i, recipient := range recipients {
		if !contracts.ValidAddress(recipient) || contracts.AddressesEqual(recipient, contracts.AddressZero) {
			return fmt.Errorf("recipients[%d] is invalid", i)
		}
		if err := positiveUint128(fmt.Sprintf("amounts[%d]", i), amounts[i]); err != nil {
			return err
		}
	}
	return nil
}

func canonicalToken(value string) bool {
	return contracts.AddressesEqual(value, contracts.AddressUSDCMainnet) || contracts.AddressesEqual(value, contracts.AddressEURCMainnet)
}

func canonicalPair(tokenIn, tokenOut string) bool {
	return (contracts.AddressesEqual(tokenIn, contracts.AddressUSDCMainnet) && contracts.AddressesEqual(tokenOut, contracts.AddressEURCMainnet)) ||
		(contracts.AddressesEqual(tokenIn, contracts.AddressEURCMainnet) && contracts.AddressesEqual(tokenOut, contracts.AddressUSDCMainnet))
}

func positiveUint128(name string, value *big.Int) error {
	if value == nil || value.Sign() <= 0 {
		return fmt.Errorf("%s must be greater than zero", name)
	}
	if value.BitLen() > 128 {
		return fmt.Errorf("%s exceeds uint128", name)
	}
	return nil
}

func RejectAdminSelector(selector [4]byte) error {
	for _, signature := range []string{SigExecuteSameTokenPayroll, SigExecuteCrossTokenPayroll} {
		allowed, err := Selector(signature)
		if err != nil {
			return err
		}
		if selector == allowed {
			return nil
		}
	}
	return fmt.Errorf("selector is not an allowlisted payroll Mainnet selector")
}
