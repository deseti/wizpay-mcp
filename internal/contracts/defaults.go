package contracts

// DefaultRegistry returns a registry preloaded with the verified Arc Testnet
// Payroll and Swap deployments at RegistryVersion 1.
//
// No Bridge, CCTP, ANS, or FX Engine deployment is registered.
func DefaultRegistry() *Registry {
	registry := NewRegistry()
	for _, deployment := range DefaultDeployments() {
		if err := registry.Register(deployment); err != nil {
			panic(err)
		}
	}
	return registry
}

// DefaultDeployments returns the verified Arc Testnet deployment descriptors.
//
// RegistryVersion is MCP artifact metadata only; it is not a Solidity semantic
// version. No separate FX Engine deployment is assumed or invented.
func DefaultDeployments() []Deployment {
	return []Deployment{
		payrollDeploymentV1(),
		swapDeploymentV1(),
	}
}

func payrollDeploymentV1() Deployment {
	return Deployment{
		ID:                 ContractWizPayPayroll,
		RegistryVersion:    RegistryVersion,
		Name:               "WizPay",
		ChainID:            ChainIDArcTestnet,
		Network:            NetworkArcTestnet,
		Address:            AddressWizPayPayroll,
		ExecutionFunctions: append([]string(nil), canonicalPayrollExecution...),
		ReadFunctions:      append([]string(nil), canonicalPayrollReads...),
		VerificationEvents: append([]string(nil), canonicalPayrollEvents...),
		Status:             StatusEnabled,
		ABISource:          "contracts/abi/WizPay.json",
		SourceContract:     "contracts/WizPay.sol",
		Notes:              "RegistryVersion is MCP artifact metadata, not a Solidity semantic version. Admin functions are intentionally excluded. No separate FX Engine deployment is assumed.",
	}
}

func swapDeploymentV1() Deployment {
	return Deployment{
		ID:                 ContractWizPaySwapExecutor,
		RegistryVersion:    RegistryVersion,
		Name:               "WizPaySwapExecutor",
		ChainID:            ChainIDArcTestnet,
		Network:            NetworkArcTestnet,
		Address:            AddressWizPaySwapExecutor,
		ExecutionFunctions: append([]string(nil), canonicalSwapExecution...),
		ReadFunctions:      append([]string(nil), canonicalSwapReads...),
		VerificationEvents: append([]string(nil), canonicalSwapEvents...),
		Status:             StatusEnabled,
		ABISource:          "contracts/abi/WizPaySwapExecutor.json",
		SourceContract:     "contracts/WizPaySwapExecutor.sol",
		Notes:              "RegistryVersion is MCP artifact metadata, not a Solidity semantic version. Admin functions are intentionally excluded.",
	}
}
