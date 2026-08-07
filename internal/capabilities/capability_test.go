package capabilities

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/auth"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestRegistryRegistrationAndVersionLookup(t *testing.T) {
	registry := NewRegistry()
	descriptor := testDescriptor(CapabilityPayroll, 1, StatusEnabled)
	if err := registry.Register(descriptor); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(descriptor); err != nil {
		t.Fatalf("identical registration was not idempotent: %v", err)
	}
	changed := descriptor
	changed.Description = "changed"
	if err := registry.Register(changed); !hasCode(err, apperrors.CodeCapabilityConflict) {
		t.Fatalf("changed duplicate error = %v", err)
	}
	got, err := registry.GetVersion(CapabilityPayroll, 1)
	if err != nil || got.Digest() != descriptor.Digest() {
		t.Fatalf("exact lookup = %#v, %v", got, err)
	}
}

func TestRegistryLatestEnabledIsDeterministic(t *testing.T) {
	registry := NewRegistry()
	for _, descriptor := range []Descriptor{
		testDescriptor(CapabilitySwap, 2, StatusEnabled),
		testDescriptor(CapabilitySwap, 1, StatusDisabled),
		testDescriptor(CapabilitySwap, 3, StatusDeprecated),
	} {
		if err := registry.Register(descriptor); err != nil {
			t.Fatal(err)
		}
	}
	got, err := registry.GetLatest(CapabilitySwap)
	if err != nil || got.Version != 2 {
		t.Fatalf("latest = %#v, %v", got, err)
	}
	list := registry.List()
	if len(list) != 3 || list[0].Version != 1 || list[1].Version != 2 || list[2].Version != 3 {
		t.Fatalf("list ordering = %#v", list)
	}
	decision, err := registry.Resolve(CapabilitySwap, AvailabilityRequest{})
	if err != nil || !decision.Available || decision.Descriptor.Version != 2 {
		t.Fatalf("implicit resolution = %#v, %v", decision, err)
	}
}

func TestAvailabilityRejectsEachUnavailableReason(t *testing.T) {
	base := testDescriptor(CapabilitySwap, 1, StatusEnabled)
	base.SupportedChains = []string{"5042002"}
	base.SupportedNetworks = []string{"arc-testnet"}
	base.SupportedTokens = []TokenClass{"USDC"}
	base.SupportedRoutes = []RouteType{RouteDirect}
	base.ProviderFeatures = []ProviderFeature{FeatureTokenTransfer}
	registry := NewRegistry()
	if err := registry.Register(base); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		request AvailabilityRequest
		want    AvailabilityReason
	}{
		{"chain", AvailabilityRequest{ChainID: "1", Network: "arc-testnet", Token: "USDC", Route: RouteDirect, ProviderFeatures: []ProviderFeature{FeatureTokenTransfer}}, ReasonUnsupportedChain},
		{"network", AvailabilityRequest{ChainID: "5042002", Network: "mainnet", Token: "USDC", Route: RouteDirect, ProviderFeatures: []ProviderFeature{FeatureTokenTransfer}}, ReasonUnsupportedNetwork},
		{"token", AvailabilityRequest{ChainID: "5042002", Network: "arc-testnet", Token: "EURC", Route: RouteDirect, ProviderFeatures: []ProviderFeature{FeatureTokenTransfer}}, ReasonUnsupportedToken},
		{"route", AvailabilityRequest{ChainID: "5042002", Network: "arc-testnet", Token: "USDC", Route: RouteCrossChain, ProviderFeatures: []ProviderFeature{FeatureTokenTransfer}}, ReasonUnsupportedRoute},
		{"provider feature", AvailabilityRequest{ChainID: "5042002", Network: "arc-testnet", Token: "USDC", Route: RouteDirect}, ReasonMissingProviderFeature},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := registry.Resolve(CapabilitySwap, tt.request)
			if err != nil || decision.Available || decision.Reason != tt.want {
				t.Fatalf("decision = %#v, error = %v", decision, err)
			}
		})
	}
}

func TestAvailabilityAcceptsSupportedRequest(t *testing.T) {
	descriptor := testDescriptor(CapabilityPayroll, 1, StatusEnabled)
	descriptor.SupportedChains = []string{"5042002"}
	descriptor.SupportedNetworks = []string{"arc-testnet"}
	descriptor.SupportedTokens = []TokenClass{"USDC"}
	descriptor.SupportedRoutes = []RouteType{RouteDirect}
	descriptor.ProviderFeatures = []ProviderFeature{FeatureUserControlledWallet, FeatureTokenTransfer}
	registry := NewRegistry()
	if err := registry.Register(descriptor); err != nil {
		t.Fatal(err)
	}
	decision, err := registry.Resolve(CapabilityPayroll, AvailabilityRequest{
		ChainID: "5042002", Network: "arc-testnet", Token: "USDC", Route: RouteDirect,
		ProviderFeatures: []ProviderFeature{FeatureTokenTransfer, FeatureUserControlledWallet},
	})
	if err != nil || !decision.Available || decision.Reason != ReasonAvailable {
		t.Fatalf("decision = %#v, error = %v", decision, err)
	}
}

func TestExactDisabledAndDeprecatedVersionsAreUnavailable(t *testing.T) {
	registry := NewRegistry()
	for version, status := range map[uint]Status{1: StatusDisabled, 2: StatusDeprecated} {
		if err := registry.Register(testDescriptor(CapabilityBridge, version, status)); err != nil {
			t.Fatal(err)
		}
	}
	for version, want := range map[uint]AvailabilityReason{1: ReasonDisabled, 2: ReasonDeprecated} {
		decision, err := registry.Resolve(CapabilityBridge, AvailabilityRequest{Version: version})
		if err != nil || decision.Available || decision.Reason != want {
			t.Fatalf("version %d decision = %#v, error = %v", version, decision, err)
		}
	}
}

func TestRegisteredDescriptorIsImmutableAndDigestDeterministic(t *testing.T) {
	descriptor := testDescriptor(CapabilityPayroll, 1, StatusEnabled)
	descriptor.Permissions = []auth.Permission{auth.PermissionPrepareExecution, auth.PermissionCreateIntent}
	descriptor.SupportedChains = []string{"5042002", "1"}
	reordered := descriptor
	reordered.Permissions = []auth.Permission{auth.PermissionCreateIntent, auth.PermissionPrepareExecution}
	reordered.SupportedChains = []string{"1", "5042002"}
	if descriptor.Digest() != reordered.Digest() {
		t.Fatal("digest changed with collection ordering")
	}
	registry := NewRegistry()
	if err := registry.Register(descriptor); err != nil {
		t.Fatal(err)
	}
	descriptor.SupportedChains[0] = "mutated"
	got, err := registry.GetVersion(CapabilityPayroll, 1)
	if err != nil || got.SupportedChains[0] != "1" {
		t.Fatalf("registered descriptor mutated: %#v, %v", got, err)
	}
	got.SupportedChains[0] = "mutated-again"
	again, _ := registry.GetVersion(CapabilityPayroll, 1)
	if again.SupportedChains[0] != "1" {
		t.Fatal("lookup returned mutable registry storage")
	}
}

func TestDefaultsAreRegisteredButUnavailable(t *testing.T) {
	registry := DefaultRegistry()
	for _, id := range []CapabilityID{CapabilityPayroll, CapabilitySwap, CapabilityBridge, CapabilityANS} {
		_, err := registry.GetLatest(id)
		if err == nil {
			t.Fatalf("disabled default %s selected as latest", id)
		}
		descriptor, err := registry.GetVersion(id, 1)
		if err != nil {
			t.Fatalf("default %s not registered: %v", id, err)
		}
		if descriptor.Status != StatusDisabled {
			t.Fatalf("default %s status = %s, want %s", id, descriptor.Status, StatusDisabled)
		}
		if _, err := registry.Resolve(id, AvailabilityRequest{}); !hasCode(err, apperrors.CodeCapabilityUnavailable) {
			t.Fatalf("default %s implicit resolution error = %v", id, err)
		}
		decision, err := registry.Resolve(id, AvailabilityRequest{Version: 1})
		if err != nil || decision.Available || decision.Reason != ReasonDisabled {
			t.Fatalf("default %s exact decision = %#v, error = %v", id, decision, err)
		}
	}
}

func TestTypedDefaultRequirementsAndDescriptions(t *testing.T) {
	registry := DefaultRegistry()
	checks := []struct {
		id       CapabilityID
		features []ProviderFeature
	}{
		{CapabilityPayroll, []ProviderFeature{FeatureUserControlledWallet, FeatureContractExecution}},
		{CapabilitySwap, []ProviderFeature{FeatureUserControlledWallet, FeatureContractExecution, FeatureSwapExecution}},
	}
	for _, check := range checks {
		descriptor, err := registry.GetVersion(check.id, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !sameFeatures(descriptor.ProviderFeatures, check.features) {
			t.Fatalf("%s features = %v, want %v", check.id, descriptor.ProviderFeatures, check.features)
		}
		if !strings.Contains(descriptor.Description, "typed allowlisted contract execution") || strings.Contains(descriptor.Description, "not implemented") {
			t.Fatalf("%s has stale description: %q", check.id, descriptor.Description)
		}
	}
}

func sameFeatures(got, want []ProviderFeature) bool {
	if len(got) != len(want) {
		return false
	}
	for _, feature := range want {
		if !contains(got, feature) {
			return false
		}
	}
	return true
}

func TestDescriptorValidationAndMapping(t *testing.T) {
	for _, item := range []struct {
		id     CapabilityID
		intent intents.Type
	}{
		{CapabilityPayroll, intents.TypePayroll}, {CapabilitySwap, intents.TypeSwap},
		{CapabilityBridge, intents.TypeBridge}, {CapabilityANS, intents.TypeANSRegistration},
	} {
		descriptor := testDescriptor(item.id, 1, StatusEnabled)
		if descriptor.IntentType != item.intent || descriptor.Validate() != nil {
			t.Fatalf("descriptor %s invalid or mapped to %s", item.id, descriptor.IntentType)
		}
	}
	invalid := testDescriptor(CapabilityPayroll, 0, StatusEnabled)
	if err := invalid.Validate(); err == nil {
		t.Fatal("version zero accepted")
	}
	invalid = testDescriptor(CapabilityID("UNKNOWN"), 1, StatusEnabled)
	if err := invalid.Validate(); err == nil {
		t.Fatal("unknown capability accepted")
	}
	invalid = testDescriptor(CapabilityPayroll, 1, StatusEnabled)
	invalid.IntentType = intents.TypeSwap
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid intent mapping accepted")
	}
	invalid = testDescriptor(CapabilityPayroll, 1, StatusEnabled)
	invalid.Permissions = []auth.Permission{auth.Permission("unknown")}
	if err := invalid.Validate(); err == nil {
		t.Fatal("unknown permission accepted")
	}
}

func TestRegistryConcurrentReadsAndRegistration(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(testDescriptor(CapabilityPayroll, 1, StatusEnabled)); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func(version uint) {
			defer wait.Done()
			_, _ = registry.GetLatest(CapabilityPayroll)
			_ = registry.Register(testDescriptor(CapabilityPayroll, version, StatusEnabled))
		}(uint(i + 2))
	}
	wait.Wait()
}

func testDescriptor(id CapabilityID, version uint, status Status) Descriptor {
	return Descriptor{ID: id, Version: version, Status: status, IntentType: map[CapabilityID]intents.Type{CapabilityPayroll: intents.TypePayroll, CapabilitySwap: intents.TypeSwap, CapabilityBridge: intents.TypeBridge, CapabilityANS: intents.TypeANSRegistration}[id], Permissions: []auth.Permission{auth.PermissionCreateIntent}, Description: "test capability"}
}

func hasCode(err error, want apperrors.Code) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Code == want
}
