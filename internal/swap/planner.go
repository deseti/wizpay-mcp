package swap

import (
	"fmt"
	"math/big"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

// Planner derives a sealed canonical Mainnet swap call from frozen intent.
type Planner struct {
	registry *contracts.Registry
}

// NewPlanner binds planning to a trusted static registry. A nil registry uses
// the repository's verified default deployment registry.
func NewPlanner(registry *contracts.Registry) Planner {
	return Planner{registry: registry}
}

// Plan derives every economic field from intent. It accepts no router, target,
// selector, function, calldata, token, recipient, amount, or deadline overrides.
func (p Planner) Plan(intent intents.Intent) (Plan, error) {
	if err := intent.Validate(); err != nil {
		return Plan{}, fmt.Errorf("swap intent is invalid: %w", err)
	}
	if intent.Type() != intents.TypeSwap {
		return Plan{}, fmt.Errorf("swap planner requires SWAP intent")
	}
	if intent.Digest() == "" || intent.Status() == intents.StatusDraft {
		return Plan{}, fmt.Errorf("swap planner requires a frozen intent")
	}
	financial := intent.Financial().Swap
	if financial == nil {
		return Plan{}, fmt.Errorf("swap financial parameters are required")
	}
	if !financial.Phase12Executable() {
		return Plan{}, fmt.Errorf("swap intent is not Phase 12 executable")
	}
	ownership := intent.Ownership()
	if err := p.validateBinding(intent.Route(), ownership); err != nil {
		return Plan{}, err
	}
	amountIn, err := financial.InputAmount.BaseInt()
	if err != nil {
		return Plan{}, fmt.Errorf("swap amountIn: %w", err)
	}
	minAmountOut, err := financial.MinimumOutput.BaseInt()
	if err != nil {
		return Plan{}, fmt.Errorf("swap minAmountOut: %w", err)
	}
	minHopPrice, ok := new(big.Int).SetString(financial.MinHopPriceX36, 10)
	if !ok || minHopPrice.Sign() <= 0 {
		return Plan{}, fmt.Errorf("swap minHopPriceX36 is invalid")
	}
	deadline := big.NewInt(financial.Deadline.UTC().Unix())
	call, err := contractswap.EncodeExecuteSwap(p.registry, contractswap.ExecuteSwapInput{
		TokenIn: financial.InputToken.Address, TokenOut: financial.OutputToken.Address,
		AmountIn: amountIn, MinAmountOut: minAmountOut, MinHopPriceX36: minHopPrice, Deadline: deadline,
	})
	if err != nil {
		return Plan{}, fmt.Errorf("encode swap: %w", err)
	}
	return newPlan(intent, call), nil
}

func (p Planner) validateBinding(route intents.Route, ownership intents.Ownership) error {
	if route.Type != intents.RouteAllowlistedContract {
		return fmt.Errorf("swap route must be ALLOWLISTED_CONTRACT")
	}
	if route.Reference != intents.RouteReferenceSwap {
		return fmt.Errorf("swap route reference must be %s", intents.RouteReferenceSwap)
	}
	if route.Version != intents.RouteVersionSwap {
		return fmt.Errorf("swap route version must be %d", intents.RouteVersionSwap)
	}
	if route.Version != uint64(contracts.RegistryVersion) {
		return fmt.Errorf("swap route version must be %d", contracts.RegistryVersion)
	}
	if ownership.ChainID != contracts.ChainIDArcMainnet {
		return fmt.Errorf("swap ownership chain must be %s", contracts.ChainIDArcMainnet)
	}
	if ownership.Network != contracts.NetworkArcMainnet {
		return fmt.Errorf("swap ownership network must be %s", contracts.NetworkArcMainnet)
	}
	deployment, err := contractswap.ExpectedDeployment(p.registry)
	if err != nil {
		return fmt.Errorf("resolve swap deployment: %w", err)
	}
	if ownership.ChainID != deployment.ChainID ||
		!contracts.AddressesEqual(deployment.Address, contracts.AddressWizPaySwapExecutor) {
		return fmt.Errorf("swap deployment does not match canonical Arc Mainnet binding")
	}
	return nil
}

func newPlan(intent intents.Intent, call contracts.EncodedCall) Plan {
	ownership := intent.Ownership()
	return Plan{
		intentID: intent.IntentID(), intentDigest: intent.Digest(), capability: intents.TypeSwap,
		contractID: call.ContractID(), registryVersion: call.RegistryVersion(), chainID: call.ChainID(),
		walletAddress: ownership.WalletAddress, encodedCall: call.Clone(),
	}
}
