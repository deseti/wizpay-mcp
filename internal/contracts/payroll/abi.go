// Package payroll provides the narrow typed ABI boundary for the reviewed
// WizPayPayrollMainnet deployment. Track A does not wire financial execution.
package payroll

import (
	"fmt"
	"strings"
	"sync"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

const (
	SigExecuteSameTokenPayroll  = "executeSameTokenPayroll(address,address[],uint256[],string)"
	SigExecuteCrossTokenPayroll = "executeCrossTokenPayroll(address,address,address[],uint256[],uint256,uint256,uint256,uint256,string)"
)

const (
	SigPayrollBatchExecuted     = "PayrollBatchExecuted(address,address,address,uint256,uint256,uint256,uint256,string)"
	SigPayrollPayment           = "PayrollPayment(bytes32,address,address,address,uint256,uint256)"
	SigPayrollReferenceConsumed = "PayrollReferenceConsumed(bytes32,address,address,address,bytes32,uint256,uint256,uint256,uint256,string)"
	SigPayrollSurplusRefunded   = "PayrollSurplusRefunded(bytes32,address,address,uint256)"
	SigPayrollSwapExecuted      = "PayrollSwapExecuted(bytes32,address,address,address,uint256,uint256,uint256,uint256,uint256)"
)

var AdminFunctionNames = []string{"pause", "unpause", "rescueTokens", "transferOwnership", "renounceOwnership"}

// minimalABI is derived from contracts/abi/WizPayPayrollMainnet.json. Only
// reviewed execution descriptors, attestation reads, and verification events
// are present; administration is excluded.
const minimalABI = `[
  {"type":"function","name":"executeSameTokenPayroll","stateMutability":"payable","inputs":[{"name":"token","type":"address"},{"name":"recipients","type":"address[]"},{"name":"amounts","type":"uint256[]"},{"name":"referenceId","type":"string"}],"outputs":[{"name":"totalOut","type":"uint256"}]},
  {"type":"function","name":"executeCrossTokenPayroll","stateMutability":"payable","inputs":[{"name":"tokenIn","type":"address"},{"name":"tokenOut","type":"address"},{"name":"recipients","type":"address[]"},{"name":"outputAmounts","type":"uint256[]"},{"name":"grossInput","type":"uint256"},{"name":"minTotalOut","type":"uint256"},{"name":"minHopPriceX36","type":"uint256"},{"name":"deadline","type":"uint256"},{"name":"referenceId","type":"string"}],"outputs":[{"name":"amountOut","type":"uint256"}]},
  {"type":"function","name":"ARC_MAINNET_CHAIN_ID","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"ARC_MAINNET_POOL_FEE","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint24"}]},
  {"type":"function","name":"ARC_MAINNET_POOL_TICK_SPACING","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"int24"}]},
  {"type":"function","name":"ARC_NATIVE_USDC_SCALE","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"EURC","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"MAX_BATCH_SIZE","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"MAX_DEADLINE_WINDOW","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"MAX_FEE_BPS","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"MAX_REFERENCE_ID_LENGTH","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"USDC","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"feeBps","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"feeRecipient","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"owner","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"paused","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"bool"}]},
  {"type":"function","name":"permit2","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"poolFee","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint24"}]},
  {"type":"function","name":"poolManager","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"poolTickSpacing","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"int24"}]},
  {"type":"function","name":"universalRouter","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
  {"type":"function","name":"usedReferenceHashes","stateMutability":"view","inputs":[{"name":"","type":"bytes32"}],"outputs":[{"name":"","type":"bool"}]},
  {"type":"event","name":"PayrollBatchExecuted","anonymous":false,"inputs":[{"name":"employer","type":"address","indexed":true},{"name":"tokenIn","type":"address","indexed":true},{"name":"tokenOut","type":"address","indexed":true},{"name":"totalInput","type":"uint256","indexed":false},{"name":"totalOutput","type":"uint256","indexed":false},{"name":"totalFees","type":"uint256","indexed":false},{"name":"recipientCount","type":"uint256","indexed":false},{"name":"referenceId","type":"string","indexed":false}]},
  {"type":"event","name":"PayrollPayment","anonymous":false,"inputs":[{"name":"referenceHash","type":"bytes32","indexed":true},{"name":"employer","type":"address","indexed":true},{"name":"tokenOut","type":"address","indexed":true},{"name":"recipient","type":"address","indexed":false},{"name":"paymentIndex","type":"uint256","indexed":false},{"name":"amountOut","type":"uint256","indexed":false}]},
  {"type":"event","name":"PayrollReferenceConsumed","anonymous":false,"inputs":[{"name":"referenceHash","type":"bytes32","indexed":true},{"name":"employer","type":"address","indexed":true},{"name":"tokenIn","type":"address","indexed":true},{"name":"tokenOut","type":"address","indexed":false},{"name":"batchDigest","type":"bytes32","indexed":false},{"name":"totalInput","type":"uint256","indexed":false},{"name":"totalOutput","type":"uint256","indexed":false},{"name":"totalFees","type":"uint256","indexed":false},{"name":"recipientCount","type":"uint256","indexed":false},{"name":"referenceId","type":"string","indexed":false}]},
  {"type":"event","name":"PayrollSurplusRefunded","anonymous":false,"inputs":[{"name":"referenceHash","type":"bytes32","indexed":true},{"name":"employer","type":"address","indexed":true},{"name":"tokenOut","type":"address","indexed":true},{"name":"amount","type":"uint256","indexed":false}]},
  {"type":"event","name":"PayrollSwapExecuted","anonymous":false,"inputs":[{"name":"referenceHash","type":"bytes32","indexed":true},{"name":"employer","type":"address","indexed":true},{"name":"tokenIn","type":"address","indexed":true},{"name":"tokenOut","type":"address","indexed":false},{"name":"grossInput","type":"uint256","indexed":false},{"name":"feeAmount","type":"uint256","indexed":false},{"name":"netAmountIn","type":"uint256","indexed":false},{"name":"amountOut","type":"uint256","indexed":false},{"name":"minTotalOut","type":"uint256","indexed":false}]}
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
		return ethabi.Method{}, fmt.Errorf("payroll ABI parse failed: %w", err)
	}
	for _, method := range definition.Methods {
		if method.Sig == signature {
			return method, nil
		}
	}
	return ethabi.Method{}, fmt.Errorf("payroll method %q is not on the allowlisted ABI fragment", signature)
}

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
	return registry.Require(contracts.ContractWizPayPayroll, contracts.RegistryVersion, contracts.ChainIDArcMainnet, contracts.NetworkArcMainnet)
}
