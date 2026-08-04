package circle

import (
	"github.com/deseti/wizpay-mcp/internal/providers"
)

// TransactionState is a Circle transaction state. Only the ten values
// documented by Circle's transaction schema are recognized here:
// INITIATED, CLEARED, QUEUED, SENT, STUCK, CONFIRMED, COMPLETE, FAILED,
// DENIED, CANCELLED. Any other value is unknown and fails closed.
type TransactionState string

const (
	StateInitiated TransactionState = "INITIATED"
	StateCleared   TransactionState = "CLEARED"
	StateQueued    TransactionState = "QUEUED"
	StateSent      TransactionState = "SENT"
	StateStuck     TransactionState = "STUCK"
	StateConfirmed TransactionState = "CONFIRMED"
	StateComplete  TransactionState = "COMPLETE"
	StateFailed    TransactionState = "FAILED"
	StateDenied    TransactionState = "DENIED"
	StateCancelled TransactionState = "CANCELLED"
)

// classifyTransactionState maps a documented Circle transaction state onto the
// provider-neutral taxonomy.
//
// Two rules govern this mapping and must not be relaxed:
//
// First, no Circle state can ever yield ClassVerifiedSuccess. CONFIRMED and
// COMPLETE mean Circle observed the transaction progress, not that WizPay
// verified it: only an Arc receipt may verify an execution. Both therefore map
// to ClassSubmittedPending so the runtime advances to on-chain verification.
//
// Second, every state here is post-initiation. A submission has already been
// attempted, so no state may return a pre-submission or resubmittable
// classification; failure states are terminal and unknown states are ambiguous.
func classifyTransactionState(state TransactionState) (providers.Classification, string) {
	switch state {
	case StateInitiated, StateCleared, StateQueued, StateSent, StateConfirmed, StateComplete:
		return providers.ClassSubmittedPending, ""
	case StateStuck:
		// The transaction reached the network and has not progressed. It may
		// still confirm, so it remains a pending observation rather than a
		// failure, and it must never be resubmitted.
		return providers.ClassSubmittedPending, ""
	case StateFailed:
		return providers.ClassPermanentProviderRejection, "PROVIDER_TRANSACTION_FAILED"
	case StateDenied:
		return providers.ClassPermanentProviderRejection, "PROVIDER_TRANSACTION_DENIED"
	case StateCancelled:
		return providers.ClassPermanentProviderRejection, "PROVIDER_TRANSACTION_CANCELLED"
	default:
		// An undocumented state is never guessed. Because submission may
		// already have occurred, it degrades to reconciliation-only ambiguity.
		return providers.ClassAmbiguousSubmission, "PROVIDER_STATE_UNRECOGNIZED"
	}
}

// ChallengeStatus is a Circle challenge status. The documented values are
// PENDING, IN_PROGRESS, COMPLETE, FAILED, and EXPIRED.
type ChallengeStatus string

const (
	ChallengePending    ChallengeStatus = "PENDING"
	ChallengeInProgress ChallengeStatus = "IN_PROGRESS"
	ChallengeComplete   ChallengeStatus = "COMPLETE"
	ChallengeFailed     ChallengeStatus = "FAILED"
	ChallengeExpired    ChallengeStatus = "EXPIRED"
)

// classifyChallengeStatus maps a challenge status onto the provider-neutral
// taxonomy.
//
// A challenge is a request for the user's own authorization. While it is
// outstanding, the execution is waiting on the user, not on a provider, and
// WizPay must never complete it on the user's behalf. A challenge that failed
// or expired means the user did not authorize: that is terminal for this
// attempt and must not trigger an automatic resubmission.
func classifyChallengeStatus(status ChallengeStatus) (providers.Classification, string) {
	switch status {
	case ChallengePending, ChallengeInProgress:
		return providers.ClassUserAuthorizationRequired, "USER_AUTHORIZATION_REQUIRED"
	case ChallengeComplete:
		// The user authorized. The transaction outcome is now the transaction's
		// own state, which the caller resolves separately.
		return providers.ClassSubmittedPending, ""
	case ChallengeFailed:
		return providers.ClassPermanentProviderRejection, "USER_AUTHORIZATION_FAILED"
	case ChallengeExpired:
		return providers.ClassPermanentProviderRejection, "USER_AUTHORIZATION_EXPIRED"
	default:
		return providers.ClassAmbiguousSubmission, "PROVIDER_CHALLENGE_STATE_UNRECOGNIZED"
	}
}

// classifyHTTPStatus maps a transport-level response onto the taxonomy.
//
// The submitted flag is the safety pivot: once a financial request has left the
// process, an inconclusive response can never be treated as "nothing happened".
// It becomes ambiguous, which is reconciliation-only.
func classifyHTTPStatus(statusCode int, submitted bool) (providers.Classification, string) {
	switch {
	case statusCode == 401 || statusCode == 403:
		if submitted {
			return providers.ClassAmbiguousSubmission, "PROVIDER_AUTHORIZATION_REJECTED"
		}
		return providers.ClassPermanentProviderRejection, "PROVIDER_AUTHORIZATION_REJECTED"
	case statusCode == 400 || statusCode == 404 || statusCode == 422:
		if submitted {
			return providers.ClassAmbiguousSubmission, "PROVIDER_REQUEST_REJECTED"
		}
		return providers.ClassPermanentProviderRejection, "PROVIDER_REQUEST_REJECTED"
	case statusCode == 409:
		// A conflict on an idempotent submission means the provider already
		// knows this operation. Reconcile it; never submit it again.
		return providers.ClassAmbiguousSubmission, "PROVIDER_REQUEST_CONFLICT"
	case statusCode == 429:
		if submitted {
			return providers.ClassAmbiguousSubmission, "PROVIDER_RATE_LIMITED"
		}
		return providers.ClassTransientProviderError, "PROVIDER_RATE_LIMITED"
	case statusCode >= 500:
		if submitted {
			return providers.ClassAmbiguousSubmission, "PROVIDER_UNAVAILABLE"
		}
		return providers.ClassTransientProviderError, "PROVIDER_UNAVAILABLE"
	default:
		if submitted {
			return providers.ClassAmbiguousSubmission, "PROVIDER_RESPONSE_UNEXPECTED"
		}
		return providers.ClassTransientProviderError, "PROVIDER_RESPONSE_UNEXPECTED"
	}
}
