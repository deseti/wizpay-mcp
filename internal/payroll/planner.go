package payroll

import (
	"fmt"
	"math/big"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

// Planner deterministically maps frozen Phase 12 Payroll intent material to an
// allowlisted typed contract call. It performs no I/O and consumes no approval.
type Planner struct {
	registry *contracts.Registry
}

// NewPlanner binds planning to a trusted static registry. A nil registry uses
// the repository's verified default deployment registry.
func NewPlanner(registry *contracts.Registry) Planner {
	return Planner{registry: registry}
}

// Plan derives every economic field from intent. It accepts no target,
// selector, function, calldata, token, recipient, or amount overrides.
func (p Planner) Plan(intent intents.Intent) (Plan, error) {
	if err := intent.Validate(); err != nil {
		return Plan{}, fmt.Errorf("payroll intent is invalid: %w", err)
	}
	if intent.Type() != intents.TypePayroll {
		return Plan{}, fmt.Errorf("payroll planner requires PAYROLL intent")
	}
	if intent.Digest() == "" || intent.Status() == intents.StatusDraft {
		return Plan{}, fmt.Errorf("payroll planner requires a frozen intent")
	}
	financial := intent.Financial().Payroll
	if financial == nil {
		return Plan{}, fmt.Errorf("payroll financial parameters are required")
	}
	if !financial.Phase12Executable() {
		return Plan{}, fmt.Errorf("payroll intent is not Phase 12 executable")
	}
	ownership := intent.Ownership()
	if err := p.validateBinding(intent.Route(), ownership); err != nil {
		return Plan{}, err
	}
	call, err := p.encode(*financial)
	if err != nil {
		return Plan{}, fmt.Errorf("encode payroll call: %w", err)
	}
	return newPlan(intent, call), nil
}

func (p Planner) encode(financial intents.PayrollParameters) (contracts.EncodedCall, error) {
	recipients := make([]string, len(financial.Recipients))
	tokenOuts := make([]string, len(financial.Recipients))
	amountsIn := make([]*big.Int, len(financial.Recipients))
	minAmountsOut := make([]*big.Int, len(financial.Recipients))
	for i, line := range financial.Recipients {
		recipients[i] = line.Address
		tokenOuts[i] = line.TokenOut.Address
		var err error
		amountsIn[i], err = line.AmountIn.BaseInt()
		if err != nil {
			return contracts.EncodedCall{}, fmt.Errorf("recipient %d amount_in is invalid: %w", i, err)
		}
		minAmountsOut[i], err = line.MinAmountOut.BaseInt()
		if err != nil {
			return contracts.EncodedCall{}, fmt.Errorf("recipient %d min_amount_out is invalid: %w", i, err)
		}
	}

	switch financial.Variant {
	case intents.PayrollVariantSingle:
		return contractpayroll.EncodeRouteAndPay(p.registry, contractpayroll.SinglePayment{
			TokenIn: financial.TokenIn.Address, TokenOut: tokenOuts[0], AmountIn: amountsIn[0],
			MinAmountOut: minAmountsOut[0], Recipient: recipients[0],
		})
	case intents.PayrollVariantBatchSingleTokenOut:
		return contractpayroll.EncodeBatchSingleTokenOut(p.registry, contractpayroll.BatchSingleTokenOut{
			TokenIn: financial.TokenIn.Address, TokenOut: tokenOuts[0], Recipients: recipients,
			AmountsIn: amountsIn, MinAmountsOut: minAmountsOut, ReferenceID: financial.ReferenceID,
		})
	case intents.PayrollVariantBatchMultiTokenOut:
		return contractpayroll.EncodeBatchMultiTokenOut(p.registry, contractpayroll.BatchMultiTokenOut{
			TokenIn: financial.TokenIn.Address, TokenOuts: tokenOuts, Recipients: recipients,
			AmountsIn: amountsIn, MinAmountsOut: minAmountsOut, ReferenceID: financial.ReferenceID,
		})
	default:
		return contracts.EncodedCall{}, fmt.Errorf("unsupported payroll variant %q", financial.Variant)
	}
}

func (p Planner) validateBinding(route intents.Route, ownership intents.Ownership) error {
	if route.Type != intents.RouteAllowlistedContract {
		return fmt.Errorf("payroll route must be ALLOWLISTED_CONTRACT")
	}
	if route.Reference != intents.RouteReferencePayroll {
		return fmt.Errorf("payroll route reference must be %s", intents.RouteReferencePayroll)
	}
	if route.Version != intents.RouteVersionPayroll {
		return fmt.Errorf("payroll route version must be %d", intents.RouteVersionPayroll)
	}
	if route.Version != uint64(contracts.RegistryVersion) {
		return fmt.Errorf("payroll route version must be %d", contracts.RegistryVersion)
	}
	if ownership.ChainID != contracts.ChainIDArcTestnet {
		return fmt.Errorf("payroll ownership chain must be %s", contracts.ChainIDArcTestnet)
	}
	deployment, err := contractpayroll.ExpectedDeployment(p.registry)
	if err != nil {
		return fmt.Errorf("resolve payroll deployment: %w", err)
	}
	if ownership.ChainID != deployment.ChainID ||
		!contracts.AddressesEqual(deployment.Address, contracts.AddressWizPayPayroll) {
		return fmt.Errorf("payroll deployment does not match canonical Arc Testnet binding")
	}
	return nil
}

func newPlan(intent intents.Intent, call contracts.EncodedCall) Plan {
	ownership := intent.Ownership()
	return Plan{
		intentID: intent.IntentID(), intentDigest: intent.Digest(), capability: intents.TypePayroll,
		contractID: call.ContractID(), registryVersion: call.RegistryVersion(), chainID: call.ChainID(),
		walletAddress: ownership.WalletAddress, encodedCall: call.Clone(),
	}
}
