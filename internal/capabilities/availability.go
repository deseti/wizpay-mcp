package capabilities

type AvailabilityReason string

const (
	ReasonAvailable              AvailabilityReason = "AVAILABLE"
	ReasonDisabled               AvailabilityReason = "DISABLED"
	ReasonDeprecated             AvailabilityReason = "DEPRECATED"
	ReasonUnsupportedChain       AvailabilityReason = "UNSUPPORTED_CHAIN"
	ReasonUnsupportedNetwork     AvailabilityReason = "UNSUPPORTED_NETWORK"
	ReasonUnsupportedToken       AvailabilityReason = "UNSUPPORTED_TOKEN"
	ReasonUnsupportedRoute       AvailabilityReason = "UNSUPPORTED_ROUTE"
	ReasonMissingProviderFeature AvailabilityReason = "MISSING_PROVIDER_FEATURE"
)

type AvailabilityRequest struct {
	Version          uint
	ChainID          string
	Network          string
	Token            TokenClass
	Route            RouteType
	ProviderFeatures []ProviderFeature
}

type AvailabilityDecision struct {
	Descriptor Descriptor
	Available  bool
	Reason     AvailabilityReason
}

func (registry *Registry) Resolve(id CapabilityID, request AvailabilityRequest) (AvailabilityDecision, error) {
	var descriptor Descriptor
	var err error
	if request.Version == 0 {
		descriptor, err = registry.GetLatest(id)
	} else {
		descriptor, err = registry.GetVersion(id, request.Version)
	}
	if err != nil {
		return AvailabilityDecision{}, err
	}
	if descriptor.Status == StatusDisabled {
		return unavailable(descriptor, ReasonDisabled), nil
	}
	if descriptor.Status == StatusDeprecated {
		return unavailable(descriptor, ReasonDeprecated), nil
	}
	if request.ChainID != "" && !contains(descriptor.SupportedChains, request.ChainID) {
		return unavailable(descriptor, ReasonUnsupportedChain), nil
	}
	if request.Network != "" && !contains(descriptor.SupportedNetworks, request.Network) {
		return unavailable(descriptor, ReasonUnsupportedNetwork), nil
	}
	if request.Token != "" && !contains(descriptor.SupportedTokens, request.Token) {
		return unavailable(descriptor, ReasonUnsupportedToken), nil
	}
	if request.Route != "" && !contains(descriptor.SupportedRoutes, request.Route) {
		return unavailable(descriptor, ReasonUnsupportedRoute), nil
	}
	availableFeatures := make(map[ProviderFeature]struct{}, len(request.ProviderFeatures))
	for _, feature := range request.ProviderFeatures {
		availableFeatures[feature] = struct{}{}
	}
	for _, required := range descriptor.ProviderFeatures {
		if _, found := availableFeatures[required]; !found {
			return unavailable(descriptor, ReasonMissingProviderFeature), nil
		}
	}
	return AvailabilityDecision{Descriptor: descriptor, Available: true, Reason: ReasonAvailable}, nil
}

func unavailable(descriptor Descriptor, reason AvailabilityReason) AvailabilityDecision {
	return AvailabilityDecision{Descriptor: descriptor, Reason: reason}
}

func contains[T comparable](values []T, value T) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
