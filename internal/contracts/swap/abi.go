// Package swap provides the narrow typed ABI boundary for the reviewed
// WizPaySwapExecutorMainnet deployment.
package swap

import (
	"fmt"
	"strings"
	"sync"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

const SigExecuteSwap = "executeSwap(address,address,uint256,uint256,uint256,uint256)"
const SigWizPayMainnetSwapExecuted = "WizPayMainnetSwapExecuted(address,address,address,uint256,uint256,uint256,uint256,uint256)"

var AdminFunctionNames = []string{"pause", "unpause", "rescueTokens", "transferOwnership", "renounceOwnership"}

// minimalABI is derived from contracts/abi/WizPaySwapExecutorMainnet.json.
const minimalABI = `[
  {"type":"function","name":"executeSwap","stateMutability":"payable","inputs":[{"name":"tokenIn","type":"address"},{"name":"tokenOut","type":"address"},{"name":"amountIn","type":"uint256"},{"name":"minAmountOut","type":"uint256"},{"name":"minHopPriceX36","type":"uint256"},{"name":"deadline","type":"uint256"}],"outputs":[{"name":"amountOut","type":"uint256"}]},
  {"type":"function","name":"ARC_MAINNET_CHAIN_ID","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"ARC_MAINNET_POOL_FEE","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint24"}]},
  {"type":"function","name":"ARC_MAINNET_POOL_TICK_SPACING","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"int24"}]},
  {"type":"function","name":"ARC_NATIVE_USDC_SCALE","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"EURC","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"MAX_DEADLINE_WINDOW","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"MAX_FEE_BPS","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"USDC","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"feeBps","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"feeRecipient","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"owner","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"paused","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"bool"}]},
  {"type":"function","name":"permit2","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"poolFee","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint24"}]},
  {"type":"function","name":"poolTickSpacing","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"int24"}]},
  {"type":"function","name":"universalRouter","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"event","name":"WizPayMainnetSwapExecuted","anonymous":false,"inputs":[{"name":"caller","type":"address","indexed":true},{"name":"tokenIn","type":"address","indexed":true},{"name":"tokenOut","type":"address","indexed":true},{"name":"amountIn","type":"uint256","indexed":false},{"name":"feeAmount","type":"uint256","indexed":false},{"name":"netAmountIn","type":"uint256","indexed":false},{"name":"amountOut","type":"uint256","indexed":false},{"name":"minAmountOut","type":"uint256","indexed":false}]}
]`

var (
	parsedABI     ethabi.ABI
	parsedABIOnce sync.Once
	parsedABIErr  error
)

func abiDefinition() (ethabi.ABI, error) {
	parsedABIOnce.Do(func() { parsedABI, parsedABIErr = ethabi.JSON(strings.NewReader(minimalABI)) })
	return parsedABI, parsedABIErr
}

func MethodBySignature(signature string) (ethabi.Method, error) {
	definition, err := abiDefinition()
	if err != nil {
		return ethabi.Method{}, fmt.Errorf("swap ABI parse failed: %w", err)
	}
	for _, method := range definition.Methods {
		if method.Sig == signature {
			return method, nil
		}
	}
	return ethabi.Method{}, fmt.Errorf("swap method %q is not on the allowlisted ABI fragment", signature)
}

func EventBySignature(signature string) (ethabi.Event, error) {
	definition, err := abiDefinition()
	if err != nil {
		return ethabi.Event{}, fmt.Errorf("swap ABI parse failed: %w", err)
	}
	for _, event := range definition.Events {
		if event.Sig == signature {
			return event, nil
		}
	}
	return ethabi.Event{}, fmt.Errorf("swap event %q is not on the allowlisted ABI fragment", signature)
}

func Selector(signature string) ([4]byte, error) {
	method, err := MethodBySignature(signature)
	if err != nil {
		return [4]byte{}, err
	}
	var out [4]byte
	copy(out[:], method.ID)
	return out, nil
}

func ExpectedDeployment(registry *contracts.Registry) (contracts.Deployment, error) {
	if registry == nil {
		registry = contracts.DefaultRegistry()
	}
	return registry.Require(contracts.ContractWizPaySwapExecutor, contracts.RegistryVersion, contracts.ChainIDArcMainnet, contracts.NetworkArcMainnet)
}
