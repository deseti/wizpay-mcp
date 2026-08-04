// Package payroll provides the narrow typed Payroll contract primitive for the
// verified WizPay Arc Testnet deployment. It validates and encodes already-
// approved immutable execution requirements and decodes verification events.
//
// It does not plan payroll, choose recipients, fetch prices, submit
// transactions, or mark financial success.
package payroll

import (
	"fmt"
	"strings"
	"sync"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

// Canonical execution signatures allowlisted for the Payroll contract.
const (
	SigBatchMultiTokenOut  = "batchRouteAndPay(address,address[],address[],uint256[],uint256[],string)"
	SigBatchSingleTokenOut = "batchRouteAndPay(address,address,address[],uint256[],uint256[],string)"
	SigRouteAndPay         = "routeAndPay(address,address,uint256,uint256,address)"
)

// Canonical verification event signatures.
const (
	SigBatchPaymentRouted = "BatchPaymentRouted(address,address,address,uint256,uint256,uint256,uint256,string)"
	SigPaymentRouted      = "PaymentRouted(address,address,address,address,uint256,uint256,uint256)"
)

// Admin function names present in the full ABI that MUST never be exposed as
// runtime execution surface by this package.
var AdminFunctionNames = []string{
	"emergencyWithdraw",
	"pause",
	"unpause",
	"setTokenWhitelist",
	"batchSetTokenWhitelist",
	"setWhitelistEnabled",
	"updateFXEngine",
	"updateFee",
	"updateFeeCollector",
	"transferOwnership",
	"renounceOwnership",
}

// minimalABI is the allowlisted ABI fragment derived from contracts/abi/WizPay.json.
// Full admin surface is intentionally omitted.
//
// Event indexed/non-indexed layout is taken exactly from the verified ABI:
//
//	BatchPaymentRouted: sender indexed; remaining fields non-indexed
//	PaymentRouted: sender and recipient indexed; remaining fields non-indexed
const minimalABI = `[
  {
    "type": "function",
    "name": "batchRouteAndPay",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "tokenIn", "type": "address"},
      {"name": "tokenOuts", "type": "address[]"},
      {"name": "recipients", "type": "address[]"},
      {"name": "amountsIn", "type": "uint256[]"},
      {"name": "minAmountsOut", "type": "uint256[]"},
      {"name": "referenceId", "type": "string"}
    ],
    "outputs": [{"name": "totalOut", "type": "uint256"}]
  },
  {
    "type": "function",
    "name": "batchRouteAndPay",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "tokenIn", "type": "address"},
      {"name": "tokenOut", "type": "address"},
      {"name": "recipients", "type": "address[]"},
      {"name": "amountsIn", "type": "uint256[]"},
      {"name": "minAmountsOut", "type": "uint256[]"},
      {"name": "referenceId", "type": "string"}
    ],
    "outputs": [{"name": "totalOut", "type": "uint256"}]
  },
  {
    "type": "function",
    "name": "routeAndPay",
    "stateMutability": "nonpayable",
    "inputs": [
      {"name": "tokenIn", "type": "address"},
      {"name": "tokenOut", "type": "address"},
      {"name": "amountIn", "type": "uint256"},
      {"name": "minAmountOut", "type": "uint256"},
      {"name": "recipient", "type": "address"}
    ],
    "outputs": [{"name": "amountOut", "type": "uint256"}]
  },
  {
    "type": "function",
    "name": "getBatchEstimatedOutputs",
    "stateMutability": "view",
    "inputs": [
      {"name": "tokenIn", "type": "address"},
      {"name": "tokenOuts", "type": "address[]"},
      {"name": "amountsIn", "type": "uint256[]"}
    ],
    "outputs": [
      {"name": "", "type": "uint256[]"},
      {"name": "", "type": "uint256"},
      {"name": "", "type": "uint256"}
    ]
  },
  {
    "type": "function",
    "name": "getEstimatedOutput",
    "stateMutability": "view",
    "inputs": [
      {"name": "tokenIn", "type": "address"},
      {"name": "tokenOut", "type": "address"},
      {"name": "amountIn", "type": "uint256"}
    ],
    "outputs": [{"name": "", "type": "uint256"}]
  },
  {
    "type": "function",
    "name": "paused",
    "stateMutability": "view",
    "inputs": [],
    "outputs": [{"name": "", "type": "bool"}]
  },
  {
    "type": "function",
    "name": "whitelistEnabled",
    "stateMutability": "view",
    "inputs": [],
    "outputs": [{"name": "", "type": "bool"}]
  },
  {
    "type": "function",
    "name": "whitelistedTokens",
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
    "type": "event",
    "name": "BatchPaymentRouted",
    "anonymous": false,
    "inputs": [
      {"name": "sender", "type": "address", "indexed": true},
      {"name": "tokenIn", "type": "address", "indexed": false},
      {"name": "tokenOut", "type": "address", "indexed": false},
      {"name": "totalAmountIn", "type": "uint256", "indexed": false},
      {"name": "totalAmountOut", "type": "uint256", "indexed": false},
      {"name": "totalFees", "type": "uint256", "indexed": false},
      {"name": "recipientCount", "type": "uint256", "indexed": false},
      {"name": "referenceId", "type": "string", "indexed": false}
    ]
  },
  {
    "type": "event",
    "name": "PaymentRouted",
    "anonymous": false,
    "inputs": [
      {"name": "sender", "type": "address", "indexed": true},
      {"name": "recipient", "type": "address", "indexed": true},
      {"name": "tokenIn", "type": "address", "indexed": false},
      {"name": "tokenOut", "type": "address", "indexed": false},
      {"name": "amountIn", "type": "uint256", "indexed": false},
      {"name": "amountOut", "type": "uint256", "indexed": false},
      {"name": "feeAmount", "type": "uint256", "indexed": false}
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
		return ethabi.Method{}, fmt.Errorf("payroll ABI parse failed: %w", err)
	}
	for _, method := range definition.Methods {
		if method.Sig == signature {
			return method, nil
		}
	}
	return ethabi.Method{}, fmt.Errorf("payroll method %q is not on the allowlisted ABI fragment", signature)
}

// EventBySignature returns the ABI event matching the exact canonical signature.
func EventBySignature(signature string) (ethabi.Event, error) {
	definition, err := abiDefinition()
	if err != nil {
		return ethabi.Event{}, fmt.Errorf("payroll ABI parse failed: %w", err)
	}
	for _, event := range definition.Events {
		if event.Sig == signature {
			return event, nil
		}
	}
	return ethabi.Event{}, fmt.Errorf("payroll event %q is not on the allowlisted ABI fragment", signature)
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

// ExpectedDeployment returns the verified Arc Testnet Payroll deployment from
// the provided registry (or the default registry when nil).
func ExpectedDeployment(registry *contracts.Registry) (contracts.Deployment, error) {
	if registry == nil {
		registry = contracts.DefaultRegistry()
	}
	return registry.Require(contracts.ContractWizPayPayroll, contracts.RegistryVersion, contracts.ChainIDArcTestnet, contracts.NetworkArcTestnet)
}
