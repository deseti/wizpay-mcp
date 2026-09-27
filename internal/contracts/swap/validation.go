package swap

import (
	"fmt"
	"math/big"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

func validateDeployment(deployment contracts.Deployment) error {
	if deployment.ID != contracts.ContractWizPaySwapExecutor || deployment.RegistryVersion != contracts.RegistryVersion ||
		deployment.ChainID != contracts.ChainIDArcMainnet || deployment.Network != contracts.NetworkArcMainnet ||
		!contracts.AddressesEqual(deployment.Address, contracts.AddressWizPaySwapExecutor) || deployment.Status != contracts.StatusEnabled {
		return fmt.Errorf("swap deployment does not match the canonical Arc Mainnet descriptor")
	}
	return nil
}

func validateExecuteSwap(in ExecuteSwapInput) error {
	if !canonicalPair(in.TokenIn, in.TokenOut) {
		return fmt.Errorf("token pair must be canonical Arc Mainnet USDC/EURC")
	}
	for _, value := range []struct {
		name   string
		amount *big.Int
	}{
		{"amountIn", in.AmountIn}, {"minAmountOut", in.MinAmountOut},
		{"minHopPriceX36", in.MinHopPriceX36}, {"deadline", in.Deadline},
	} {
		if value.amount == nil || value.amount.Sign() <= 0 {
			return fmt.Errorf("%s must be greater than zero", value.name)
		}
		if value.amount.BitLen() > 128 {
			return fmt.Errorf("%s exceeds uint128", value.name)
		}
	}
	return nil
}

func canonicalPair(tokenIn, tokenOut string) bool {
	return (contracts.AddressesEqual(tokenIn, contracts.AddressUSDCMainnet) && contracts.AddressesEqual(tokenOut, contracts.AddressEURCMainnet)) ||
		(contracts.AddressesEqual(tokenIn, contracts.AddressEURCMainnet) && contracts.AddressesEqual(tokenOut, contracts.AddressUSDCMainnet))
}

func RejectAdminSelector(selector [4]byte) error {
	allowed, err := Selector(SigExecuteSwap)
	if err != nil {
		return err
	}
	if selector != allowed {
		return fmt.Errorf("selector is not the allowlisted swap Mainnet selector")
	}
	return nil
}
