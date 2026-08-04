package circle

import (
	"testing"

	"github.com/deseti/wizpay-mcp/internal/providers"
)

func TestClassifyTransactionStateNeverVerifiesSuccess(t *testing.T) {
	// The cardinal rule: no Circle transaction state may assert verified success.
	// Only an Arc receipt can. CONFIRMED and COMPLETE included.
	for _, state := range []TransactionState{
		StateInitiated, StateCleared, StateQueued, StateSent, StateStuck, StateConfirmed, StateComplete,
	} {
		class, _ := classifyTransactionState(state)
		if class == providers.ClassVerifiedSuccess {
			t.Fatalf("state %s must never classify as verified success", state)
		}
		if class != providers.ClassSubmittedPending {
			t.Fatalf("state %s must be submitted-pending, got %s", state, class)
		}
	}
}

func TestClassifyTransactionStateTerminalFailures(t *testing.T) {
	cases := map[TransactionState]string{
		StateFailed:    "PROVIDER_TRANSACTION_FAILED",
		StateDenied:    "PROVIDER_TRANSACTION_DENIED",
		StateCancelled: "PROVIDER_TRANSACTION_CANCELLED",
	}
	for state, reason := range cases {
		class, got := classifyTransactionState(state)
		if class != providers.ClassPermanentProviderRejection {
			t.Fatalf("state %s must be a permanent rejection, got %s", state, class)
		}
		if got != reason {
			t.Fatalf("state %s expected reason %s, got %s", state, reason, got)
		}
	}
}

func TestClassifyTransactionStateUnknownFailsClosed(t *testing.T) {
	// An undocumented state may have submitted, so it must degrade to ambiguous
	// (reconciliation-only), never to a resubmittable or pre-submission class.
	class, reason := classifyTransactionState("SOME_FUTURE_STATE")
	if class != providers.ClassAmbiguousSubmission {
		t.Fatalf("an unknown state must be ambiguous, got %s", class)
	}
	if reason != "PROVIDER_STATE_UNRECOGNIZED" {
		t.Fatalf("unexpected reason %s", reason)
	}
}

func TestClassifyChallengeStatus(t *testing.T) {
	pending, _ := classifyChallengeStatus(ChallengePending)
	inProgress, _ := classifyChallengeStatus(ChallengeInProgress)
	if pending != providers.ClassUserAuthorizationRequired || inProgress != providers.ClassUserAuthorizationRequired {
		t.Fatalf("outstanding challenges must wait on the user")
	}
	if complete, _ := classifyChallengeStatus(ChallengeComplete); complete != providers.ClassSubmittedPending {
		t.Fatalf("a completed challenge must hand off to transaction state, got %s", complete)
	}
	for _, status := range []ChallengeStatus{ChallengeFailed, ChallengeExpired} {
		if class, _ := classifyChallengeStatus(status); class != providers.ClassPermanentProviderRejection {
			t.Fatalf("challenge %s must be a permanent rejection, got %s", status, class)
		}
	}
	if class, _ := classifyChallengeStatus("MYSTERY"); class != providers.ClassAmbiguousSubmission {
		t.Fatalf("an unknown challenge status must fail closed to ambiguous, got %s", class)
	}
}

func TestClassifyHTTPStatusSubmittedNeverDropsWork(t *testing.T) {
	// Once submitted, no inconclusive HTTP response may be treated as
	// "nothing happened": every one becomes ambiguous (reconciliation-only).
	for _, code := range []int{401, 403, 400, 404, 422, 409, 429, 500, 503, 418} {
		class, _ := classifyHTTPStatus(code, true)
		if class != providers.ClassAmbiguousSubmission {
			t.Fatalf("submitted status %d must be ambiguous, got %s", code, class)
		}
	}
}

func TestClassifyHTTPStatusNotSubmitted(t *testing.T) {
	cases := map[int]providers.Classification{
		401: providers.ClassPermanentProviderRejection,
		400: providers.ClassPermanentProviderRejection,
		429: providers.ClassTransientProviderError,
		500: providers.ClassTransientProviderError,
		418: providers.ClassTransientProviderError,
	}
	for code, expected := range cases {
		if class, _ := classifyHTTPStatus(code, false); class != expected {
			t.Fatalf("unsubmitted status %d expected %s, got %s", code, expected, class)
		}
	}
	// A conflict always means the provider already knows the operation.
	if class, _ := classifyHTTPStatus(409, false); class != providers.ClassAmbiguousSubmission {
		t.Fatalf("409 must always be ambiguous, got %s", class)
	}
}
