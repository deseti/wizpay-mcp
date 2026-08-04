package capabilities

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

const maxDescriptionLength = 256

type Requirements struct {
	Approval  bool `json:"approval"`
	Policy    bool `json:"policy"`
	Execution bool `json:"execution"`
}

type Descriptor struct {
	ID                CapabilityID      `json:"id"`
	Version           uint              `json:"version"`
	Status            Status            `json:"status"`
	IntentType        intents.Type      `json:"intent_type"`
	Permissions       []auth.Permission `json:"permissions"`
	SupportedChains   []string          `json:"supported_chains"`
	SupportedNetworks []string          `json:"supported_networks"`
	SupportedTokens   []TokenClass      `json:"supported_tokens"`
	SupportedRoutes   []RouteType       `json:"supported_routes"`
	Requirements      Requirements      `json:"requirements"`
	ProviderFeatures  []ProviderFeature `json:"provider_features"`
	Description       string            `json:"description"`
}

func (descriptor Descriptor) Validate() error {
	if !descriptor.ID.Valid() {
		return fmt.Errorf("invalid capability ID %q", descriptor.ID)
	}
	if descriptor.Version == 0 {
		return fmt.Errorf("capability version must be positive")
	}
	if !descriptor.Status.Valid() {
		return fmt.Errorf("invalid capability status %q", descriptor.Status)
	}
	if !descriptor.IntentType.Valid() || descriptor.IntentType != expectedIntentType(descriptor.ID) {
		return fmt.Errorf("intent type %q does not match capability %q", descriptor.IntentType, descriptor.ID)
	}
	if err := validateDescription(descriptor.Description); err != nil {
		return err
	}
	if err := validateUnique(descriptor.Permissions, func(value auth.Permission) bool { return value.Valid() }, "permission"); err != nil {
		return err
	}
	if err := validateUnique(descriptor.SupportedChains, validText, "chain"); err != nil {
		return err
	}
	if err := validateUnique(descriptor.SupportedNetworks, validText, "network"); err != nil {
		return err
	}
	if err := validateUnique(descriptor.SupportedTokens, func(value TokenClass) bool { return validText(string(value)) }, "token"); err != nil {
		return err
	}
	if err := validateUnique(descriptor.SupportedRoutes, func(value RouteType) bool { return value.Valid() }, "route"); err != nil {
		return err
	}
	return validateUnique(descriptor.ProviderFeatures, func(value ProviderFeature) bool { return value.Valid() }, "provider feature")
}

func (descriptor Descriptor) normalized() Descriptor {
	descriptor.Permissions = append([]auth.Permission(nil), descriptor.Permissions...)
	descriptor.SupportedChains = append([]string(nil), descriptor.SupportedChains...)
	descriptor.SupportedNetworks = append([]string(nil), descriptor.SupportedNetworks...)
	descriptor.SupportedTokens = append([]TokenClass(nil), descriptor.SupportedTokens...)
	descriptor.SupportedRoutes = append([]RouteType(nil), descriptor.SupportedRoutes...)
	descriptor.ProviderFeatures = append([]ProviderFeature(nil), descriptor.ProviderFeatures...)
	sort.Slice(descriptor.Permissions, func(i, j int) bool { return descriptor.Permissions[i] < descriptor.Permissions[j] })
	sort.Strings(descriptor.SupportedChains)
	sort.Strings(descriptor.SupportedNetworks)
	sort.Slice(descriptor.SupportedTokens, func(i, j int) bool { return descriptor.SupportedTokens[i] < descriptor.SupportedTokens[j] })
	sort.Slice(descriptor.SupportedRoutes, func(i, j int) bool { return descriptor.SupportedRoutes[i] < descriptor.SupportedRoutes[j] })
	sort.Slice(descriptor.ProviderFeatures, func(i, j int) bool { return descriptor.ProviderFeatures[i] < descriptor.ProviderFeatures[j] })
	return descriptor
}

func (descriptor Descriptor) Digest() string {
	canonical, _ := json.Marshal(descriptor.normalized())
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validateDescription(value string) error {
	if !validText(value) || utf8.RuneCountInString(value) > maxDescriptionLength {
		return fmt.Errorf("capability description must be safe text of 1 to %d characters", maxDescriptionLength)
	}
	return nil
}

func validText(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
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
