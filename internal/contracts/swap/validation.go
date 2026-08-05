package swap

import (
	"fmt"
	"math/big"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

func validateDeployment(deployment contracts.Deployment) error {
	if deployment.ID != contracts.ContractWizPaySwapExecutor {
		return fmt.Errorf("deployment is not WIZPAY_SWAP_EXECUTOR")
	}
	if deployment.RegistryVersion != contracts.RegistryVersion {
		return fmt.Errorf("unexpected swap registry version %d", deployment.RegistryVersion)
	}
	if deployment.ChainID != contracts.ChainIDArcTestnet {
		return fmt.Errorf("swap deployment chain ID mismatch")
	}
	if deployment.Network != contracts.NetworkArcTestnet {
		return fmt.Errorf("swap deployment network mismatch")
	}
	if !contracts.AddressesEqual(deployment.Address, contracts.AddressWizPaySwapExecutor) {
		return fmt.Errorf("swap deployment address mismatch")
	}
	if deployment.Status != contracts.StatusEnabled {
		return fmt.Errorf("swap deployment is not enabled")
	}
	return nil
}

func validateExecuteSwap(in ExecuteSwapInput) error {
	if err := validateNonZeroAddress("router", in.Router); err != nil {
		return err
	}
	if err := validateNonZeroAddress("tokenIn", in.TokenIn); err != nil {
		return err
	}
	if err := validateNonZeroAddress("tokenOut", in.TokenOut); err != nil {
		return err
	}
	if err := validatePositiveAmount("amountIn", in.AmountIn); err != nil {
		return err
	}
	if err := validatePositiveAmount("minAmountOut", in.MinAmountOut); err != nil {
		return err
	}
	if err := validateNonZeroAddress("recipient", in.Recipient); err != nil {
		return err
	}
	if in.Deadline <= 0 {
		return fmt.Errorf("deadline must be greater than zero")
	}
	return nil
}

func validateNonZeroAddress(name, value string) error {
	if !contracts.ValidAddress(value) {
		return fmt.Errorf("%s is not a valid address", name)
	}
	if contracts.NormalizeAddress(value) == "0x0000000000000000000000000000000000000000" {
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

// RejectAdminSelector ensures the selector is exactly the allowlisted executeSwap.
func RejectAdminSelector(selector [4]byte) error {
	allowed, err := Selector(SigExecuteSwap)
	if err != nil {
		return err
	}
	if selector != allowed {
		return fmt.Errorf("selector is not the allowlisted swap execution selector")
	}
	return nil
}
