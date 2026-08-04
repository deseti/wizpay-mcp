package capabilities

import (
	"sort"
	"sync"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

type Registry struct {
	mu      sync.RWMutex
	entries map[CapabilityID]map[uint]Descriptor
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[CapabilityID]map[uint]Descriptor)}
}

func (registry *Registry) Register(descriptor Descriptor) error {
	if err := descriptor.Validate(); err != nil {
		return apperrors.Wrap(apperrors.CodeValidationError, "Capability definition is invalid.", false, true, true, err)
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
		return apperrors.New(apperrors.CodeCapabilityConflict, "Capability version conflicts with an existing definition.", false, true, true)
	}
	versions[descriptor.Version] = descriptor
	return nil
}

func (registry *Registry) GetVersion(id CapabilityID, version uint) (Descriptor, error) {
	if !id.Valid() || version == 0 {
		return Descriptor{}, apperrors.New(apperrors.CodeValidationError, "Capability lookup is invalid.", false, true, true)
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	versions, found := registry.entries[id]
	if !found {
		return Descriptor{}, capabilityNotFound()
	}
	descriptor, found := versions[version]
	if !found {
		return Descriptor{}, apperrors.New(apperrors.CodeCapabilityNotFound, "Capability version was not found.", false, true, true)
	}
	return descriptor.normalized(), nil
}

func (registry *Registry) GetLatest(id CapabilityID) (Descriptor, error) {
	if !id.Valid() {
		return Descriptor{}, apperrors.New(apperrors.CodeValidationError, "Capability lookup is invalid.", false, true, true)
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	versions, found := registry.entries[id]
	if !found {
		return Descriptor{}, capabilityNotFound()
	}
	var latest Descriptor
	for _, descriptor := range versions {
		if descriptor.Status == StatusEnabled && descriptor.Version > latest.Version {
			latest = descriptor
		}
	}
	if latest.Version == 0 {
		return Descriptor{}, apperrors.New(apperrors.CodeCapabilityUnavailable, "Capability is unavailable.", false, true, false)
	}
	return latest.normalized(), nil
}

func (registry *Registry) List() []Descriptor {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]Descriptor, 0)
	for _, versions := range registry.entries {
		for _, descriptor := range versions {
			result = append(result, descriptor.normalized())
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].Version < result[j].Version
	})
	return result
}

func capabilityNotFound() error {
	return apperrors.New(apperrors.CodeCapabilityNotFound, "Capability was not found.", false, true, true)
}
