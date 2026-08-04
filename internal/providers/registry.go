package providers

import (
	"sort"
	"sync"

	"github.com/deseti/wizpay-mcp/internal/capabilities"
	"github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
)

// Registry is the typed provider registry. It is deliberately separate from the
// Phase 10 capability registry: this one answers "which provider can execute
// this, and is it configured", never "is this capability enabled".
//
// Resolution is deterministic and purely local. It performs no network health
// check, no weighted routing, and no failover selection.
type Registry struct {
	mu      sync.RWMutex
	entries map[ProviderID]map[uint]Descriptor
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[ProviderID]map[uint]Descriptor)}
}

// Register adds one provider version. Registering an identical descriptor again
// is idempotent; registering a different descriptor for the same provider and
// version is rejected rather than silently overwriting a live adapter.
func (registry *Registry) Register(descriptor Descriptor) error {
	if err := descriptor.Validate(); err != nil {
		return errors.Wrap(errors.CodeValidationError, "Provider definition is invalid.", false, true, true, err)
	}
	descriptor = descriptor.normalized()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	versions := registry.entries[descriptor.ID]
	if versions == nil {
		versions = make(map[uint]Descriptor)
		registry.entries[descriptor.ID] = versions
	}
	if existing, found := versions[descriptor.Version]; found {
		if existing.Digest() == descriptor.Digest() {
			return nil
		}
		return errors.New(errors.CodeProviderConflict, "Provider version conflicts with an existing definition.", false, true, true)
	}
	versions[descriptor.Version] = descriptor
	return nil
}

// GetVersion returns one exact provider version, whether or not it is
// configured. Callers that intend to execute must use Resolve instead.
func (registry *Registry) GetVersion(id ProviderID, version uint) (Descriptor, error) {
	if !id.Valid() || version == 0 {
		return Descriptor{}, errors.New(errors.CodeValidationError, "Provider lookup is invalid.", false, true, true)
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	descriptor, found := registry.entries[id][version]
	if !found {
		return Descriptor{}, providerNotFound()
	}
	return descriptor.normalized(), nil
}

// GetLatest returns the highest configured version of a provider.
func (registry *Registry) GetLatest(id ProviderID) (Descriptor, error) {
	if !id.Valid() {
		return Descriptor{}, errors.New(errors.CodeValidationError, "Provider lookup is invalid.", false, true, true)
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	versions, found := registry.entries[id]
	if !found {
		return Descriptor{}, providerNotFound()
	}
	var latest Descriptor
	for _, descriptor := range versions {
		if descriptor.Configured && descriptor.Version > latest.Version {
			latest = descriptor
		}
	}
	if latest.Version == 0 {
		return Descriptor{}, providerUnavailable()
	}
	return latest.normalized(), nil
}

// List returns every registered provider version in deterministic order.
func (registry *Registry) List() []Descriptor {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	var descriptors []Descriptor
	for _, versions := range registry.entries {
		for _, descriptor := range versions {
			descriptors = append(descriptors, descriptor.normalized())
		}
	}
	sort.Slice(descriptors, func(i, j int) bool {
		if descriptors[i].ID != descriptors[j].ID {
			return descriptors[i].ID < descriptors[j].ID
		}
		return descriptors[i].Version < descriptors[j].Version
	})
	return descriptors
}

// Query selects a provider by the abstract feature it must supply and the
// chain and network it must serve. An empty chain or network matches any.
type Query struct {
	Feature capabilities.ProviderFeature
	ChainID string
	Network string
}

// Resolve returns the highest configured provider version satisfying the query,
// along with its adapter. It fails closed: an unconfigured provider is
// unavailable, never a silently degraded one.
func (registry *Registry) Resolve(query Query) (Descriptor, execution.Adapter, error) {
	if !query.Feature.Valid() {
		return Descriptor{}, nil, errors.New(errors.CodeValidationError, "Provider lookup is invalid.", false, true, true)
	}
	if query.ChainID != "" && !validChainID(query.ChainID) {
		return Descriptor{}, nil, errors.New(errors.CodeValidationError, "Provider lookup is invalid.", false, true, true)
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	var best Descriptor
	matchedUnconfigured := false
	for _, versions := range registry.entries {
		for _, descriptor := range versions {
			if !descriptor.Supports(query.Feature) || !descriptor.servesChain(query.ChainID) || !descriptor.servesNetwork(query.Network) {
				continue
			}
			if !descriptor.Configured {
				matchedUnconfigured = true
				continue
			}
			if best.Version == 0 || descriptor.ID < best.ID || (descriptor.ID == best.ID && descriptor.Version > best.Version) {
				best = descriptor
			}
		}
	}
	if best.Version == 0 {
		if matchedUnconfigured {
			return Descriptor{}, nil, providerUnavailable()
		}
		return Descriptor{}, nil, providerNotFound()
	}
	return best.normalized(), best.Adapter, nil
}

// Features returns the distinct provider features actually available, meaning
// declared by a configured provider. This is the value Phase 10 consumes as
// capabilities.AvailabilityRequest.ProviderFeatures; it reports provider
// reachability only and never enables a capability by itself.
func (registry *Registry) Features(chainID, network string) []capabilities.ProviderFeature {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	unique := make(map[capabilities.ProviderFeature]struct{})
	for _, versions := range registry.entries {
		for _, descriptor := range versions {
			if !descriptor.Configured || !descriptor.servesChain(chainID) || !descriptor.servesNetwork(network) {
				continue
			}
			for _, feature := range descriptor.Features {
				unique[feature] = struct{}{}
			}
		}
	}
	features := make([]capabilities.ProviderFeature, 0, len(unique))
	for feature := range unique {
		features = append(features, feature)
	}
	sort.Slice(features, func(i, j int) bool { return features[i] < features[j] })
	return features
}

func providerNotFound() error {
	return errors.New(errors.CodeProviderNotFound, "Provider was not found.", false, true, true)
}

func providerUnavailable() error {
	return errors.New(errors.CodeProviderUnavailable, "Provider is unavailable.", false, true, false)
}
