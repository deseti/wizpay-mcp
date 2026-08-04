// Package capabilities defines provider-neutral capability metadata and
// deterministic availability decisions. It never executes financial actions.
package capabilities

import "github.com/deseti/wizpay-mcp/internal/intents"

type CapabilityID string

const (
	CapabilityPayroll CapabilityID = "PAYROLL"
	CapabilitySwap    CapabilityID = "SWAP"
	CapabilityBridge  CapabilityID = "BRIDGE"
	CapabilityANS     CapabilityID = "ANS"
)

func (id CapabilityID) Valid() bool {
	switch id {
	case CapabilityPayroll, CapabilitySwap, CapabilityBridge, CapabilityANS:
		return true
	default:
		return false
	}
}

func expectedIntentType(id CapabilityID) intents.Type {
	switch id {
	case CapabilityPayroll:
		return intents.TypePayroll
	case CapabilitySwap:
		return intents.TypeSwap
	case CapabilityBridge:
		return intents.TypeBridge
	case CapabilityANS:
		return intents.TypeANSRegistration
	default:
		return ""
	}
}

type Status string

const (
	StatusEnabled    Status = "ENABLED"
	StatusDisabled   Status = "DISABLED"
	StatusDeprecated Status = "DEPRECATED"
)

func (status Status) Valid() bool {
	switch status {
	case StatusEnabled, StatusDisabled, StatusDeprecated:
		return true
	default:
		return false
	}
}

type TokenClass string

type RouteType string

const (
	RouteDirect       RouteType = "DIRECT"
	RouteCrossChain   RouteType = "CROSS_CHAIN"
	RouteRegistration RouteType = "REGISTRATION"
)

func (route RouteType) Valid() bool {
	switch route {
	case RouteDirect, RouteCrossChain, RouteRegistration:
		return true
	default:
		return false
	}
}

type ProviderFeature string

const (
	FeatureUserControlledWallet ProviderFeature = "USER_CONTROLLED_WALLET"
	FeatureTokenTransfer        ProviderFeature = "TOKEN_TRANSFER"
	FeatureContractExecution    ProviderFeature = "CONTRACT_EXECUTION"
	FeatureSwapExecution        ProviderFeature = "SWAP_EXECUTION"
	FeatureBridgeExecution      ProviderFeature = "BRIDGE_EXECUTION"
	FeatureANSRegistration      ProviderFeature = "ANS_REGISTRATION"
)

func (feature ProviderFeature) Valid() bool {
	switch feature {
	case FeatureUserControlledWallet, FeatureTokenTransfer, FeatureContractExecution,
		FeatureSwapExecution, FeatureBridgeExecution, FeatureANSRegistration:
		return true
	default:
		return false
	}
}
