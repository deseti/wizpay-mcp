package contracts

import (
	"sort"
	"sync"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// Registry is a deterministic, in-process registry of verified contract
// deployments. It is intentionally static: deployment metadata is not loaded
// from PostgreSQL and is not mutated by runtime financial flows.
//
// Identity is (ContractID, RegistryVersion). Identical re-registration is
// idempotent; a conflicting descriptor for the same key is rejected. A second
// deployment that would create (chain, address) ambiguity is also rejected.
type Registry struct {
	mu         sync.RWMutex
	entries    map[ContractID]map[uint]Deployment
	byLocation map[string]locationKey // chain|address -> id/version
}

type locationKey struct {
	ID              ContractID
	RegistryVersion uint
}

// NewRegistry returns an empty contract deployment registry.
func NewRegistry() *Registry {
	return &Registry{
		entries:    make(map[ContractID]map[uint]Deployment),
		byLocation: make(map[string]locationKey),
	}
}

// Register adds one deployment descriptor. Registering an identical descriptor
// again is idempotent. Registering a different descriptor for the same
// (ContractID, RegistryVersion) is rejected. Registering a second contract at
// the same chain/address is rejected as deployment ambiguity.
func (r *Registry) Register(deployment Deployment) error {
	if err := deployment.Validate(); err != nil {
		return apperrors.Wrap(apperrors.CodeValidationError, "Contract deployment definition is invalid.", false, true, true, err)
	}
	normalized := deployment.Normalized()
	location := locationIndex(normalized.ChainID, normalized.Address)

	r.mu.Lock()
	defer r.mu.Unlock()

	versions := r.entries[normalized.ID]
	if versions == nil {
		versions = make(map[uint]Deployment)
		r.entries[normalized.ID] = versions
	}
	if existing, found := versions[normalized.RegistryVersion]; found {
		if existing.Digest() == normalized.Digest() {
			return nil
		}
		return contractConflict()
	}
	if owner, found := r.byLocation[location]; found {
		if owner.ID != normalized.ID || owner.RegistryVersion != normalized.RegistryVersion {
			return validationError("Contract deployment address conflicts with another registered deployment.", nil)
		}
	}
	versions[normalized.RegistryVersion] = normalized
	r.byLocation[location] = locationKey{ID: normalized.ID, RegistryVersion: normalized.RegistryVersion}
	return nil
}

// GetVersion returns one exact deployment by ID and registry version.
func (r *Registry) GetVersion(id ContractID, version uint) (Deployment, error) {
	if !id.Valid() || version == 0 {
		return Deployment{}, validationError("Contract deployment lookup is invalid.", nil)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, found := r.entries[id]
	if !found {
		return Deployment{}, contractNotFound()
	}
	deployment, found := versions[version]
	if !found {
		return Deployment{}, contractNotFound()
	}
	return deployment.Normalized(), nil
}

// GetEnabled returns the highest ENABLED registry version for a contract ID.
func (r *Registry) GetEnabled(id ContractID) (Deployment, error) {
	if !id.Valid() {
		return Deployment{}, validationError("Contract deployment lookup is invalid.", nil)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, found := r.entries[id]
	if !found {
		return Deployment{}, contractNotFound()
	}
	var latest Deployment
	for _, deployment := range versions {
		if deployment.Status == StatusEnabled && deployment.RegistryVersion > latest.RegistryVersion {
			latest = deployment
		}
	}
	if latest.RegistryVersion == 0 {
		return Deployment{}, contractUnavailable()
	}
	return latest.Normalized(), nil
}

// Require matches an exact deployment and fails closed on chain/network/address
// mismatch against the registered descriptor. expectedAddress may be empty to
// skip an additional address check beyond the registry entry itself.
func (r *Registry) Require(id ContractID, version uint, chainID, network string) (Deployment, error) {
	deployment, err := r.GetVersion(id, version)
	if err != nil {
		return Deployment{}, err
	}
	if deployment.Status != StatusEnabled {
		return Deployment{}, contractUnavailable()
	}
	if chainID != "" && chainID != deployment.ChainID {
		return Deployment{}, validationError("Contract deployment chain ID does not match the registered Arc Testnet deployment.", nil)
	}
	if network != "" && network != deployment.Network {
		return Deployment{}, validationError("Contract deployment network does not match the registered Arc Testnet deployment.", nil)
	}
	return deployment, nil
}

// LookupByAddress returns the deployment registered at chainID+address.
func (r *Registry) LookupByAddress(chainID, address string) (Deployment, error) {
	if chainID == "" || !ValidAddress(address) {
		return Deployment{}, validationError("Contract address lookup is invalid.", nil)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, found := r.byLocation[locationIndex(chainID, address)]
	if !found {
		return Deployment{}, contractNotFound()
	}
	deployment := r.entries[key.ID][key.RegistryVersion]
	return deployment.Normalized(), nil
}

// List returns every registered deployment in deterministic order:
// ContractID ascending, then RegistryVersion ascending.
func (r *Registry) List() []Deployment {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Deployment, 0)
	for _, versions := range r.entries {
		for _, deployment := range versions {
			result = append(result, deployment.Normalized())
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].RegistryVersion < result[j].RegistryVersion
	})
	return result
}

func locationIndex(chainID, address string) string {
	return chainID + "|" + NormalizeAddress(address)
}
