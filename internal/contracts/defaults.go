package contracts

// DefaultRegistry returns a registry preloaded with the reviewed Arc Mainnet
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

// DefaultDeployments returns the reviewed Arc Mainnet deployment descriptors.
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
		Name:               "WizPayPayrollMainnet",
		ChainID:            ChainIDArcMainnet,
		Network:            NetworkArcMainnet,
		Address:            AddressWizPayPayroll,
		ExecutionFunctions: append([]string(nil), canonicalPayrollExecution...),
		ReadFunctions:      append([]string(nil), canonicalPayrollReads...),
		VerificationEvents: append([]string(nil), canonicalPayrollEvents...),
		Status:             StatusEnabled,
		ABISource:          "contracts/abi/WizPayPayrollMainnet.json",
		SourceContract:     "src/WizPayPayrollMainnet.sol",
		Notes:              "Track A descriptor only. Capabilities remain disabled; native-value and ERC-20 approval execution are not implemented. Admin functions are excluded.",
	}
}

func swapDeploymentV1() Deployment {
	return Deployment{
		ID:                 ContractWizPaySwapExecutor,
		RegistryVersion:    RegistryVersion,
		Name:               "WizPaySwapExecutorMainnet",
		ChainID:            ChainIDArcMainnet,
		Network:            NetworkArcMainnet,
		Address:            AddressWizPaySwapExecutor,
		ExecutionFunctions: append([]string(nil), canonicalSwapExecution...),
		ReadFunctions:      append([]string(nil), canonicalSwapReads...),
		VerificationEvents: append([]string(nil), canonicalSwapEvents...),
		Status:             StatusEnabled,
		ABISource:          "contracts/abi/WizPaySwapExecutorMainnet.json",
		SourceContract:     "src/WizPaySwapExecutorMainnet.sol",
		Notes:              "Track A descriptor only. Swap capability remains disabled; native-value and ERC-20 approval execution are not implemented. Admin functions are excluded.",
	}
}
