package payroll

import (
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractpayroll "github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

// Planner remains as the typed boundary for a later execution track. Track A
// deliberately refuses to produce a Mainnet execution plan.
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
	return Plan{}, fmt.Errorf("Arc Mainnet payroll execution is disabled in Track A")
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
	if ownership.ChainID != contracts.ChainIDArcMainnet {
		return fmt.Errorf("payroll ownership chain must be %s", contracts.ChainIDArcMainnet)
	}
	deployment, err := contractpayroll.ExpectedDeployment(p.registry)
	if err != nil {
		return fmt.Errorf("resolve payroll deployment: %w", err)
	}
	if ownership.ChainID != deployment.ChainID ||
		!contracts.AddressesEqual(deployment.Address, contracts.AddressWizPayPayroll) {
		return fmt.Errorf("payroll deployment does not match canonical Arc Mainnet binding")
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
