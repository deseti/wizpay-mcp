// Package swap provides the narrow typed Swap contract primitive for the
// verified WizPaySwapExecutor Arc Testnet deployment. It validates and encodes
// already-approved immutable swap execution requirements and decodes the
// verification event.
//
// It does not choose routers, fetch quotes, plan routes, submit transactions,
// or mark financial success.
package swap

import (
	"fmt"
	"strings"
	"sync"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

// Canonical execution signature allowlisted for the Swap executor.
const SigExecuteSwap = "executeSwap(address,address,address,uint256,uint256,address,uint256)"

// Canonical verification event signature.
const SigWizPaySwapExecuted = "WizPaySwapExecuted(address,address,address,address,uint256,uint256,uint256,uint256,address)"

// Admin function names present in the full ABI that MUST never be exposed as
// runtime execution surface by this package.
var AdminFunctionNames = []string{
	"pause",
	"unpause",
	"rescueTokens",
	"setFeeBps",
	"setFeeRecipient",
	"setRouterAllowed",
	"setTokenAllowed",
	"transferOwnership",
	"renounceOwnership",
}

// minimalABI is the allowlisted ABI fragment derived from
// contracts/abi/WizPaySwapExecutor.json. Full admin surface is intentionally
// omitted.
//
// Event indexed/non-indexed layout is taken exactly from the verified ABI:
//
//	WizPaySwapExecuted: user, router, tokenIn indexed; remaining fields non-indexed
const minimalABI = `[
  {
    "type": "function",
    "name": "executeSwap",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "router", "type": "address"},
      {"name": "tokenIn", "type": "address"},
      {"name": "tokenOut", "type": "address"},
      {"name": "amountIn", "type": "uint256"},
      {"name": "minAmountOut", "type": "uint256"},
      {"name": "recipient", "type": "address"},
      {"name": "deadline", "type": "uint256"}
    ],
    "outputs": [{"name": "amountOut", "type": "uint256"}]
  },
  {
    "type": "function",
    "name": "allowedRouters",
    "stateMutability": "view",
    "inputs": [{"name": "", "type": "address"}],
    "outputs": [{"name": "", "type": "bool"}]
  },
  {
    "type": "function",
    "name": "allowedTokens",
    "stateMutability": "view",
    "inputs": [{"name": "", "type": "address"}],
    "outputs": [{"name": "", "type": "bool"}]
  },
  {
    "type": "function",
    "name": "feeBps",
    "stateMutability": "view",
    "inputs": [],
    "outputs": [{"name": "", "type": "uint256"}]
  },
  {
    "type": "function",
    "name": "feeRecipient",
    "stateMutability": "view",
    "inputs": [],
    "outputs": [{"name": "", "type": "address"}]
  },
  {
    "type": "function",
    "name": "paused",
    "stateMutability": "view",
    "inputs": [],
    "outputs": [{"name": "", "type": "bool"}]
  },
  {
    "type": "event",
    "name": "WizPaySwapExecuted",
    "anonymous": false,
    "inputs": [
      {"name": "user", "type": "address", "indexed": true},
      {"name": "router", "type": "address", "indexed": true},
      {"name": "tokenIn", "type": "address", "indexed": true},
      {"name": "tokenOut", "type": "address", "indexed": false},
      {"name": "amountIn", "type": "uint256", "indexed": false},
      {"name": "feeAmount", "type": "uint256", "indexed": false},
      {"name": "netAmountIn", "type": "uint256", "indexed": false},
      {"name": "amountOut", "type": "uint256", "indexed": false},
      {"name": "recipient", "type": "address", "indexed": false}
    ]
  }
]`

var (
	parsedABI     ethabi.ABI
	parsedABIOnce sync.Once
	parsedABIErr  error
)

func abiDefinition() (ethabi.ABI, error) {
	parsedABIOnce.Do(func() {
		parsedABI, parsedABIErr = ethabi.JSON(strings.NewReader(minimalABI))
	})
	return parsedABI, parsedABIErr
}

// MethodBySignature returns the ABI method matching the exact canonical signature.
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

// EventBySignature returns the ABI event matching the exact canonical signature.
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

// Selector returns the 4-byte selector for a canonical allowlisted signature.
func Selector(signature string) ([4]byte, error) {
	method, err := MethodBySignature(signature)
	if err != nil {
		return [4]byte{}, err
	}
	var out [4]byte
	copy(out[:], method.ID)
	return out, nil
}

// ExpectedDeployment returns the verified Arc Testnet Swap deployment from the
// provided registry (or the default registry when nil).
func ExpectedDeployment(registry *contracts.Registry) (contracts.Deployment, error) {
	if registry == nil {
		registry = contracts.DefaultRegistry()
	}
	return registry.Require(contracts.ContractWizPaySwapExecutor, contracts.RegistryVersion, contracts.ChainIDArcTestnet, contracts.NetworkArcTestnet)
}
