package circle

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/circuit"
)

// ReferenceStore recovers the safe provider reference already persisted for an
// execution.
//
// The Phase 9 adapter contract identifies a reconciliation by execution ID
// alone, so the adapter must recover the provider reference itself. This is
// what makes reconciliation survive a process restart or a lease handoff.
type ReferenceStore interface {
	LatestReference(ctx context.Context, executionID string) (providers.Reference, bool, error)
}

// Adapter is the Circle User-Controlled Wallet execution adapter.
//
// It initiates transfers that only the user can authorize and reconciles their
// outcome. It never signs, never completes a challenge on the user's behalf,
// never creates users or wallets, and never accepts a wallet identifier from
// runtime input: the wallet comes from the approved plan alone.
type Adapter struct {
	client        *client
	planner       providers.Planner
	authorization providers.AuthorizationSource
	references    ReferenceStore
	config        Config
	now           func() time.Time
	breaker       *circuit.Breaker
}

// NewAdapter builds the Circle adapter. It fails closed when the provider is
// not fully configured rather than returning a degraded adapter.
func NewAdapter(config Config, httpClient *http.Client, planner providers.Planner, authorization providers.AuthorizationSource, references ReferenceStore, now func() time.Time) (*Adapter, error) {
	return NewAdapterWithBreaker(config, httpClient, planner, authorization, references, now, nil)
}

// NewAdapterWithBreaker builds the adapter with an optional circuit breaker for
// outbound Circle infrastructure calls.
func NewAdapterWithBreaker(config Config, httpClient *http.Client, planner providers.Planner, authorization providers.AuthorizationSource, references ReferenceStore, now func() time.Time, breaker *circuit.Breaker) (*Adapter, error) {
	if planner == nil || authorization == nil || references == nil || now == nil {
		return nil, fmt.Errorf("Circle adapter dependencies are required")
	}
	transport, err := newClientWithBreaker(config, httpClient, breaker)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: transport, planner: planner, authorization: authorization, references: references, config: config, now: now, breaker: breaker}, nil
}

var (
	_ execution.Adapter           = (*Adapter)(nil)
	_ providers.ReferenceResolver = (*Adapter)(nil)
)

// Execute initiates a transfer by creating a Circle challenge.
//
// A successful return is NOT a submitted transaction and NOT success. It means
// the user has been asked to authorize, and the resulting challenge reference
// has been captured so every later attempt reconciles instead of resubmitting.
func (a *Adapter) Execute(ctx context.Context, request execution.Request) (execution.Result, error) {
	executionID := request.ExecutionID()
	plan, err := a.planner.Plan(ctx, request)
	if err != nil {
		return a.outcome(executionID, providers.Outcome{
			Class: providers.ClassPreSubmissionValidationFailure, ReasonCode: "SUBMISSION_PLAN_UNAVAILABLE",
		})
	}
	if err := a.validatePlan(plan); err != nil {
		return a.outcome(executionID, providers.Outcome{
			Class: providers.ClassPreSubmissionValidationFailure, ReasonCode: "SUBMISSION_PLAN_INVALID",
		})
	}
	idempotencyKey, err := providers.IdempotencyKey(request)
	if err != nil {
		return a.outcome(executionID, providers.Outcome{
			Class: providers.ClassPreSubmissionValidationFailure, ReasonCode: "IDEMPOTENCY_KEY_UNAVAILABLE",
		})
	}
	authorization, found, err := a.authorization.UserAuthorization(ctx, executionID)
	if err != nil || !found {
		// The user has not delegated an authorized session. This is a wait for
		// user action, never a provider failure, and never something backend
		// orchestration may substitute for.
		return a.outcome(executionID, providers.Outcome{
			Class: providers.ClassUserAuthorizationRequired, ReasonCode: "USER_AUTHORIZATION_REQUIRED",
		})
	}

	challengeID, err := a.client.createTransferChallenge(ctx, authorization, transferRequest{
		IdempotencyKey:     idempotencyKey,
		DestinationAddress: plan.DestinationAddress,
		Amounts:            []string{plan.Amount},
		WalletID:           plan.WalletID,
		TokenID:            plan.TokenID,
		RefID:              executionID,
	})
	baseReference := providers.Reference{
		Provider: providers.ProviderCircleUserControlled,
		ChainID:  plan.ChainID,
		WalletID: plan.WalletID,
	}
	if err != nil {
		return a.transportOutcome(executionID, err, baseReference, false)
	}
	reference := baseReference
	reference.ChallengeID = challengeID
	// The challenge reference is persisted through a recovery-required
	// observation, so the execution waits on the user while remaining
	// reconcilable.
	return a.outcome(executionID, providers.Outcome{
		Class:        providers.ClassUserAuthorizationRequired,
		ReasonCode:   "USER_AUTHORIZATION_REQUIRED",
		Reference:    reference,
		HasReference: true,
	})
}

// GetStatus reconciles an execution that may already have been submitted. It is
// the only path taken once submission has started, and it never submits.
func (a *Adapter) GetStatus(ctx context.Context, executionID string) (execution.Result, error) {
	reference, found, err := a.references.LatestReference(ctx, executionID)
	if err != nil {
		return a.outcome(executionID, providers.Outcome{
			Class: providers.ClassTransientProviderError, ReasonCode: "PROVIDER_REFERENCE_UNAVAILABLE",
		})
	}
	if !found {
		// Submission may have been attempted without a reference ever being
		// captured. Resubmitting could double-spend, so this halts for
		// operator reconciliation instead.
		return a.outcome(executionID, providers.Outcome{
			Class: providers.ClassAmbiguousSubmission, ReasonCode: "PROVIDER_REFERENCE_MISSING",
		})
	}
	authorization, present, err := a.authorization.UserAuthorization(ctx, executionID)
	if err != nil || !present {
		return a.outcome(executionID, providers.Outcome{
			Class:        providers.ClassUserAuthorizationRequired,
			ReasonCode:   "USER_AUTHORIZATION_REQUIRED",
			Reference:    reference,
			HasReference: true,
		})
	}
	outcome, err := a.reconcile(ctx, executionID, authorization, reference)
	if err != nil {
		return execution.Result{}, err
	}
	return a.outcome(executionID, outcome)
}

// ResolveReference reconciles a reference on behalf of the verifier, adding the
// on-chain transaction hash once Circle reports one. It is read-only.
func (a *Adapter) ResolveReference(ctx context.Context, executionID string, reference providers.Reference) (providers.Reference, error) {
	authorization, present, err := a.authorization.UserAuthorization(ctx, executionID)
	if err != nil || !present {
		return providers.Reference{}, fmt.Errorf("user authorization is unavailable for reconciliation")
	}
	observed, err := a.resolveTransaction(ctx, executionID, authorization, reference)
	if err != nil {
		return providers.Reference{}, err
	}
	return observed.reference, nil
}

// reconcile resolves the current provider state into a classified outcome.
func (a *Adapter) reconcile(ctx context.Context, executionID string, authorization providers.UserAuthorization, reference providers.Reference) (providers.Outcome, error) {
	observed, err := a.resolveTransaction(ctx, executionID, authorization, reference)
	if err != nil {
		class, reasonCode := classifyTransportError(err)
		return providers.Outcome{
			Class: class, ReasonCode: reasonCode, Reference: reference, HasReference: true, ObservedAt: a.now().UTC(),
		}, nil
	}
	if !observed.found {
		// No transaction exists yet. If the challenge is still outstanding the
		// user simply has not authorized; otherwise the outcome is unknown and
		// must be reconciled rather than assumed.
		return a.challengeOutcome(ctx, authorization, observed.reference)
	}
	class, reasonCode := classifyResolution(observed)
	return providers.Outcome{
		Class: class, ReasonCode: reasonCode,
		Reference: observed.reference, HasReference: true, ObservedAt: a.now().UTC(),
	}, nil
}

// resolution is one reconciliation observation: the reference enriched with
// whatever the provider now knows, plus the observed transaction state. It is
// passed by value so concurrent executions never share state.
type resolution struct {
	reference providers.Reference
	state     TransactionState
	found     bool
}

// classifyResolution maps an observed transaction onto the taxonomy.
//
// A provider failure verdict that already produced an on-chain transaction is
// deliberately not trusted as terminal: the chain decides, so it degrades to a
// pending observation and the receipt verifier renders the final judgment.
func classifyResolution(observed resolution) (providers.Classification, string) {
	class, reasonCode := classifyTransactionState(observed.state)
	if observed.reference.TransactionHash != "" && class == providers.ClassPermanentProviderRejection {
		return providers.ClassSubmittedPending, ""
	}
	if class == providers.ClassSubmittedPending {
		return class, ""
	}
	return class, reasonCode
}

// resolveTransaction finds the provider transaction for a reference, enriching
// it with the transaction ID and on-chain hash when they are known.
func (a *Adapter) resolveTransaction(ctx context.Context, executionID string, authorization providers.UserAuthorization, reference providers.Reference) (resolution, error) {
	if reference.ProviderTransactionID != "" {
		record, err := a.client.getTransaction(ctx, authorization, reference.ProviderTransactionID)
		if err != nil {
			return resolution{reference: reference}, err
		}
		return a.applyTransaction(reference, record)
	}
	// The transaction ID was never observed. Match on the reference this system
	// set at submission time, which is derived from immutable execution
	// identity and is therefore stable across restarts.
	record, found, err := a.client.findTransactionByRef(ctx, authorization, reference.WalletID, executionID)
	if err != nil {
		return resolution{reference: reference}, err
	}
	if !found {
		return resolution{reference: reference}, nil
	}
	return a.applyTransaction(reference, record)
}

func (a *Adapter) applyTransaction(reference providers.Reference, record transaction) (resolution, error) {
	if record.Blockchain != "" && record.Blockchain != string(a.config.Blockchain) {
		return resolution{reference: reference}, fmt.Errorf("provider transaction is on an unexpected blockchain")
	}
	if reference.WalletID != "" && record.WalletID != "" && record.WalletID != reference.WalletID {
		return resolution{reference: reference}, fmt.Errorf("provider transaction belongs to another wallet")
	}
	enriched := reference.WithTransaction(record.ID, strings.ToLower(record.TxHash))
	if enriched.TransactionHash != "" && !providers.ValidTransactionHash(enriched.TransactionHash) {
		// An unusable hash is dropped rather than persisted; reconciliation
		// continues from the provider transaction ID.
		enriched.TransactionHash = ""
	}
	return resolution{reference: enriched, state: record.State, found: true}, nil
}

// challengeOutcome interprets an outstanding challenge when no transaction has
// been observed yet.
func (a *Adapter) challengeOutcome(ctx context.Context, authorization providers.UserAuthorization, reference providers.Reference) (providers.Outcome, error) {
	observedAt := a.now().UTC()
	if reference.ChallengeID == "" {
		return providers.Outcome{
			Class: providers.ClassAmbiguousSubmission, ReasonCode: "PROVIDER_TRANSACTION_NOT_FOUND",
			Reference: reference, HasReference: true, ObservedAt: observedAt,
		}, nil
	}
	record, found, err := a.client.findChallenge(ctx, authorization, reference.ChallengeID)
	if err != nil {
		class, reasonCode := classifyTransportError(err)
		return providers.Outcome{
			Class: class, ReasonCode: reasonCode, Reference: reference, HasReference: true, ObservedAt: observedAt,
		}, nil
	}
	if !found {
		// Circle lists only outstanding challenges, so absence is not failure.
		// With no transaction found either, the outcome is genuinely unknown.
		return providers.Outcome{
			Class: providers.ClassAmbiguousSubmission, ReasonCode: "PROVIDER_CHALLENGE_NOT_OUTSTANDING",
			Reference: reference, HasReference: true, ObservedAt: observedAt,
		}, nil
	}
	class, reasonCode := classifyChallengeStatus(record.Status)
	if class == providers.ClassSubmittedPending {
		// The user authorized but the transaction is not yet visible. Remain
		// pending rather than claiming a submission that cannot be shown.
		class, reasonCode = providers.ClassAmbiguousSubmission, "PROVIDER_TRANSACTION_NOT_FOUND"
	}
	return providers.Outcome{
		Class: class, ReasonCode: reasonCode, Reference: reference, HasReference: true, ObservedAt: observedAt,
	}, nil
}

func (a *Adapter) validatePlan(plan providers.Plan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if plan.ChainID != a.config.ChainID || plan.Network != a.config.Network {
		return fmt.Errorf("submission plan targets an unsupported chain or network")
	}
	return nil
}

// outcome converts a classified outcome into the Phase 9 adapter contract.
func (a *Adapter) outcome(executionID string, value providers.Outcome) (execution.Result, error) {
	if value.ObservedAt.IsZero() {
		value.ObservedAt = a.now().UTC()
	}
	return value.AdapterResult(executionID)
}

func (a *Adapter) transportOutcome(executionID string, err error, reference providers.Reference, hasReference bool) (execution.Result, error) {
	class, reasonCode := classifyTransportError(err)
	return a.outcome(executionID, providers.Outcome{
		Class: class, ReasonCode: reasonCode, Reference: reference, HasReference: hasReference,
	})
}

func classifyTransportError(err error) (providers.Classification, string) {
	var classified *transportError
	if errors.As(err, &classified) {
		return classified.classification()
	}
	// An unclassified error is never assumed safe.
	return providers.ClassAmbiguousSubmission, "PROVIDER_OUTCOME_UNKNOWN"
}
