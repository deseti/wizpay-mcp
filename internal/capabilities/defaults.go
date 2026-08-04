package capabilities

import (
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func DefaultRegistry() *Registry {
	registry := NewRegistry()
	for _, descriptor := range DefaultDescriptors() {
		if err := registry.Register(descriptor); err != nil {
			panic(err)
		}
	}
	return registry
}

func DefaultDescriptors() []Descriptor {
	return []Descriptor{
		defaultDescriptor(CapabilityPayroll, intents.TypePayroll, "Payroll capability metadata; execution is not implemented.", []ProviderFeature{FeatureUserControlledWallet, FeatureTokenTransfer}),
		defaultDescriptor(CapabilitySwap, intents.TypeSwap, "Swap capability metadata; execution is not implemented.", []ProviderFeature{FeatureUserControlledWallet, FeatureSwapExecution}),
		defaultDescriptor(CapabilityBridge, intents.TypeBridge, "Bridge capability metadata; execution is not implemented.", []ProviderFeature{FeatureUserControlledWallet, FeatureBridgeExecution}),
		defaultDescriptor(CapabilityANS, intents.TypeANSRegistration, "ANS registration capability metadata; execution is not implemented.", []ProviderFeature{FeatureUserControlledWallet, FeatureANSRegistration}),
	}
}

func defaultDescriptor(id CapabilityID, intentType intents.Type, description string, features []ProviderFeature) Descriptor {
	return Descriptor{
		ID: id, Version: 1, Status: StatusDisabled, IntentType: intentType,
		Permissions:      []auth.Permission{auth.PermissionCreateIntent, auth.PermissionRequestApproval, auth.PermissionEvaluatePolicy, auth.PermissionPrepareExecution},
		Requirements:     Requirements{Approval: true, Policy: true, Execution: true},
		ProviderFeatures: features, Description: description,
	}
}
