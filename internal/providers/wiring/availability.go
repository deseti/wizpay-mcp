package wiring

import (
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/capabilities"
)

// Availability composes the Phase 10 capability registry with the assembled
// provider plane. It is the seam that lets a capability's availability reflect
// whether a provider can actually reach the requested chain and network, rather
// than trusting a caller's claim about provider features.
//
// It is strictly read-only and control-plane only: it performs no I/O,
// initiates no execution, holds no authorization, and never mutates provider or
// capability state. An available decision here still implies nothing about
// authentication, approval, policy allow, execution preparation, or execution
// success.
type Availability struct {
	capabilities *capabilities.Registry
	plane        Plane
}

// NewAvailability composes a capability registry with a provider plane. Both are
// required: without the plane there is no source of truth for which provider
// features are actually reachable.
func NewAvailability(registry *capabilities.Registry, plane Plane) (*Availability, error) {
	if registry == nil {
		return nil, fmt.Errorf("capability registry is required")
	}
	if plane.Registry == nil {
		return nil, fmt.Errorf("assembled provider plane is required")
	}
	return &Availability{capabilities: registry, plane: plane}, nil
}

// Resolve decides a capability's availability with the available provider
// features derived from the provider plane.
//
// Any ProviderFeatures the caller placed on the request are discarded and
// replaced with the features a configured provider actually supplies on the
// requested chain and network. This is the fail-closed direction: a caller can
// never assert a provider feature into existence, so a capability whose required
// feature no configured provider supplies resolves to MISSING_PROVIDER_FEATURE
// rather than available. An unconfigured provider plane therefore keeps every
// execution-requiring capability unavailable, which is exactly the Phase 11
// state until a domain planner exists.
func (a *Availability) Resolve(id capabilities.CapabilityID, request capabilities.AvailabilityRequest) (capabilities.AvailabilityDecision, error) {
	request.ProviderFeatures = a.plane.ProviderFeatures(request.ChainID, request.Network)
	return a.capabilities.Resolve(id, request)
}
