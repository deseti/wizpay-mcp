// Package providers defines the provider execution plane: a typed provider
// registry, provider-neutral submission primitives, ephemeral user
// authorization, safe provider references, and provider error classification.
//
// This package never signs, never holds key material, and never implements
// capability business logic. Domain planning (payroll, swap, bridge, ANS)
// belongs to a later phase and reaches this package only through the injected
// Planner port.
package providers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/deseti/wizpay-mcp/internal/capabilities"
	"github.com/deseti/wizpay-mcp/internal/execution"
)

// ProviderID is a stable typed provider identifier. Opaque provider strings are
// intentionally not accepted anywhere in this package.
type ProviderID string

// ProviderCircleUserControlled is the Circle User-Controlled Wallet provider.
// The user retains signing authority; WizPay MCP holds no signing share.
const ProviderCircleUserControlled ProviderID = "CIRCLE_USER_CONTROLLED_WALLET"

func (id ProviderID) Valid() bool {
	switch id {
	case ProviderCircleUserControlled:
		return true
	default:
		return false
	}
}

// Descriptor declares one immutable provider version: which abstract provider
// features it supplies, which chains and networks it serves, whether its
// configuration is present, and the adapter that implements it.
//
// Configured reports configuration availability only. It is never a live
// network health check, and it never implies capability availability.
type Descriptor struct {
	ID         ProviderID
	Version    uint
	Features   []capabilities.ProviderFeature
	ChainIDs   []string
	Networks   []string
	Configured bool
	Adapter    execution.Adapter
}

func (d Descriptor) Validate() error {
	if !d.ID.Valid() {
		return fmt.Errorf("invalid provider ID %q", d.ID)
	}
	if d.Version == 0 {
		return fmt.Errorf("provider version must be positive")
	}
	if len(d.Features) == 0 {
		return fmt.Errorf("provider must declare at least one supported feature")
	}
	if err := validateUnique(d.Features, func(value capabilities.ProviderFeature) bool { return value.Valid() }, "provider feature"); err != nil {
		return err
	}
	if len(d.ChainIDs) == 0 {
		return fmt.Errorf("provider must declare at least one supported chain ID")
	}
	if err := validateUnique(d.ChainIDs, validChainID, "chain ID"); err != nil {
		return err
	}
	if len(d.Networks) == 0 {
		return fmt.Errorf("provider must declare at least one supported network")
	}
	if err := validateUnique(d.Networks, validSafeText, "network"); err != nil {
		return err
	}
	if d.Configured && d.Adapter == nil {
		return fmt.Errorf("configured provider requires an adapter implementation")
	}
	return nil
}

func (d Descriptor) normalized() Descriptor {
	d.Features = append([]capabilities.ProviderFeature(nil), d.Features...)
	d.ChainIDs = append([]string(nil), d.ChainIDs...)
	d.Networks = append([]string(nil), d.Networks...)
	sort.Slice(d.Features, func(i, j int) bool { return d.Features[i] < d.Features[j] })
	sort.Strings(d.ChainIDs)
	sort.Strings(d.Networks)
	return d
}

// Digest covers declared metadata only. The adapter implementation is a runtime
// value and is deliberately excluded so that an exact metadata replay stays
// idempotent.
func (d Descriptor) Digest() string {
	normalized := d.normalized()
	canonical, _ := json.Marshal(struct {
		ID         ProviderID                     `json:"id"`
		Version    uint                           `json:"version"`
		Features   []capabilities.ProviderFeature `json:"features"`
		ChainIDs   []string                       `json:"chain_ids"`
		Networks   []string                       `json:"networks"`
		Configured bool                           `json:"configured"`
	}{normalized.ID, normalized.Version, normalized.Features, normalized.ChainIDs, normalized.Networks, normalized.Configured})
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:])
}

// Supports reports whether this provider declares the feature. It does not
// consider configuration availability.
func (d Descriptor) Supports(feature capabilities.ProviderFeature) bool {
	for _, candidate := range d.Features {
		if candidate == feature {
			return true
		}
	}
	return false
}

func (d Descriptor) servesChain(chainID string) bool {
	if chainID == "" {
		return true
	}
	for _, candidate := range d.ChainIDs {
		if candidate == chainID {
			return true
		}
	}
	return false
}

func (d Descriptor) servesNetwork(network string) bool {
	if network == "" {
		return true
	}
	for _, candidate := range d.Networks {
		if candidate == network {
			return true
		}
	}
	return false
}

func validateUnique[T comparable](values []T, valid func(T) bool, name string) error {
	seen := make(map[T]struct{}, len(values))
	for _, value := range values {
		if !valid(value) {
			return fmt.Errorf("invalid %s %v", name, value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate %s %v", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
