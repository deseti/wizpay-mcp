package circle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

const (
	adapterSource      = "0x2222222222222222222222222222222222222222"
	adapterDestination = "0x3333333333333333333333333333333333333333"
)

type stubPlanner struct {
	plan providers.Plan
	err  error
}

func (s stubPlanner) Plan(context.Context, execution.Request) (providers.Plan, error) {
	return s.plan, s.err
}

type stubAuthorization struct {
	auth  providers.UserAuthorization
	found bool
	err   error
}

func (s stubAuthorization) UserAuthorization(context.Context, string) (providers.UserAuthorization, bool, error) {
	return s.auth, s.found, s.err
}

type stubReferences struct {
	reference providers.Reference
	found     bool
	err       error
}

func (s stubReferences) LatestReference(context.Context, string) (providers.Reference, bool, error) {
	return s.reference, s.found, s.err
}

func adapterConfig() Config {
	return Config{
		Enabled: true, BaseURL: defaultBaseURL, APIKey: APIKey{value: "secret"},
		Blockchain: BlockchainArcTestnet, ChainID: "5042002", Network: "TESTNET",
		Timeout: 20 * time.Second,
	}
}

func validPlan() providers.Plan {
	return providers.Plan{
		WalletBindingID: "binding-test", WalletID: "wallet-test", WalletAddress: adapterSource,
		ChainID: "5042002", Network: "TESTNET", DestinationAddress: adapterDestination,
		TokenID: "token-test", Amount: "1",
	}
}

func newAdapter(t *testing.T, planner providers.Planner, authorization providers.AuthorizationSource, references ReferenceStore) *Adapter {
	t.Helper()
	adapter, err := NewAdapter(adapterConfig(), nil, planner, authorization, references, func() time.Time {
		return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	return adapter
}

func TestNewAdapterRequiresDependencies(t *testing.T) {
	_, err := NewAdapter(adapterConfig(), nil, nil, stubAuthorization{}, stubReferences{}, time.Now)
	if err == nil {
		t.Fatalf("a nil planner must be rejected")
	}
}

func assertPermanent(t *testing.T, _ execution.Result, err error, reason string) {
	t.Helper()
	var adapterErr *runtime.AdapterError
	if !errors.As(err, &adapterErr) {
		t.Fatalf("expected an adapter error, got %v", err)
	}
	if adapterErr.Kind != runtime.AdapterFailurePermanent {
		t.Fatalf("expected a permanent failure, got %s", adapterErr.Kind)
	}
	if adapterErr.ReasonCode != reason {
		t.Fatalf("expected reason %s, got %s", reason, adapterErr.ReasonCode)
	}
}

func TestExecutePlannerFailureIsPermanentAndNeverSubmits(t *testing.T) {
	adapter := newAdapter(t, stubPlanner{err: errors.New("no plan")}, stubAuthorization{}, stubReferences{})
	result, err := adapter.Execute(context.Background(), execution.Request{})
	assertPermanent(t, result, err, "SUBMISSION_PLAN_UNAVAILABLE")
}

func TestExecuteInvalidPlanIsPermanent(t *testing.T) {
	// A plan that fails validation must halt before any submission.
	adapter := newAdapter(t, stubPlanner{plan: providers.Plan{WalletID: "only"}}, stubAuthorization{}, stubReferences{})
	result, err := adapter.Execute(context.Background(), execution.Request{})
	assertPermanent(t, result, err, "SUBMISSION_PLAN_INVALID")
}

func TestExecuteHaltsWhenIdentityCannotDeriveIdempotencyKey(t *testing.T) {
	// With a valid plan but an unvalidatable request, the idempotency key cannot
	// be derived, so submission is refused rather than attempted without one.
	adapter := newAdapter(t, stubPlanner{plan: validPlan()}, stubAuthorization{found: true}, stubReferences{})
	result, err := adapter.Execute(context.Background(), execution.Request{})
	assertPermanent(t, result, err, "IDEMPOTENCY_KEY_UNAVAILABLE")
}

func TestGetStatusReferenceStoreErrorIsTransient(t *testing.T) {
	adapter := newAdapter(t, stubPlanner{}, stubAuthorization{}, stubReferences{err: errors.New("db down")})
	_, err := adapter.GetStatus(context.Background(), "execution-1")
	var adapterErr *runtime.AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Kind != runtime.AdapterFailureTransient {
		t.Fatalf("a reference store error must be a transient adapter error, got %v", err)
	}
}

func TestGetStatusMissingReferenceReconcilesRatherThanResubmits(t *testing.T) {
	// No reference means submission may have occurred without being captured.
	// Resubmitting could double-spend, so the execution halts for reconciliation.
	adapter := newAdapter(t, stubPlanner{}, stubAuthorization{}, stubReferences{found: false})
	result, err := adapter.GetStatus(context.Background(), "execution-1")
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if result.Status() != execution.StatusRecoveryRequired {
		t.Fatalf("expected recovery-required, got %s", result.Status())
	}
	if result.ErrorCode() != "PROVIDER_REFERENCE_MISSING" {
		t.Fatalf("expected PROVIDER_REFERENCE_MISSING, got %s", result.ErrorCode())
	}
}
