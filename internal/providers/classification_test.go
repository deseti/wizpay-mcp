package providers

import (
	"errors"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
)

func testReference() Reference {
	return Reference{Provider: ProviderCircleUserControlled, ChainID: testChainID, WalletID: testWalletID, ProviderTransactionID: "tx-1"}
}

// This is the core safety property of the taxonomy: once external submission may
// have occurred, no classification may permit another submission attempt.
func TestReconciliationOnlyAfterPossibleSubmission(t *testing.T) {
	expected := map[Classification]bool{
		ClassPreSubmissionValidationFailure: false,
		ClassPermanentProviderRejection:     false,
		ClassUserAuthorizationRequired:      true,
		ClassTransientProviderError:         true,
		ClassAmbiguousSubmission:            true,
		ClassSubmittedPending:               true,
		ClassConfirmedOnchainFailed:         true,
		ClassVerifiedSuccess:                true,
	}
	for class, reconciliationOnly := range expected {
		if class.ReconciliationOnly() != reconciliationOnly {
			t.Fatalf("%s.ReconciliationOnly() != %v", class, reconciliationOnly)
		}
	}
}

func TestTerminalClassifications(t *testing.T) {
	terminal := map[Classification]bool{
		ClassPreSubmissionValidationFailure: true,
		ClassPermanentProviderRejection:     true,
		ClassConfirmedOnchainFailed:         true,
		ClassUserAuthorizationRequired:      false,
		ClassTransientProviderError:         false,
		ClassAmbiguousSubmission:            false,
		ClassSubmittedPending:               false,
		ClassVerifiedSuccess:                false,
	}
	for class, expected := range terminal {
		if class.Terminal() != expected {
			t.Fatalf("%s.Terminal() != %v", class, expected)
		}
	}
}

func TestClassificationValidRejectsUnknown(t *testing.T) {
	if Classification("SUBMITTED").Valid() || Classification("").Valid() {
		t.Fatal("unknown classifications must be invalid")
	}
}

// adapterFailureKind reports the typed adapter failure kind carried by an error.
func adapterFailureKind(t *testing.T, err error) runtime.AdapterFailureKind {
	t.Helper()
	var failure *runtime.AdapterError
	if !errors.As(err, &failure) {
		t.Fatalf("expected a typed adapter error, got %v", err)
	}
	return failure.Kind
}

// A transient provider error after submission must surface as a retryable
// adapter error, which the runtime resolves through GetStatus because it has
// already marked submission-started.
func TestAdapterResultTransientIsRetryableError(t *testing.T) {
	outcome := Outcome{Class: ClassTransientProviderError, ReasonCode: "PROVIDER_UNREACHABLE", ObservedAt: providerTestNow}
	_, err := outcome.AdapterResult("execution-1")
	if kind := adapterFailureKind(t, err); kind != runtime.AdapterFailureTransient {
		t.Fatalf("transient provider error must not be permanent: %v", kind)
	}
}

func TestAdapterResultPermanentClassesAreTerminal(t *testing.T) {
	for _, class := range []Classification{ClassPreSubmissionValidationFailure, ClassPermanentProviderRejection} {
		outcome := Outcome{Class: class, ReasonCode: "PROVIDER_REJECTED", ObservedAt: providerTestNow}
		_, err := outcome.AdapterResult("execution-1")
		if kind := adapterFailureKind(t, err); kind != runtime.AdapterFailurePermanent {
			t.Fatalf("%s must be permanent, got %v", class, kind)
		}
	}
}

// Ambiguity and pending user authorization must both hold the execution for
// recovery, never fail it and never resubmit it.
func TestAdapterResultAmbiguousAndAuthorizationRequireRecovery(t *testing.T) {
	for _, class := range []Classification{ClassAmbiguousSubmission, ClassUserAuthorizationRequired} {
		outcome := Outcome{Class: class, ReasonCode: "PROVIDER_SUBMISSION_UNCONFIRMED", Reference: testReference(), HasReference: true, ObservedAt: providerTestNow}
		result, err := outcome.AdapterResult("execution-1")
		if err != nil {
			t.Fatalf("%s: %v", class, err)
		}
		if result.Status() != execution.StatusRecoveryRequired {
			t.Fatalf("%s produced status %s", class, result.Status())
		}
		if result.AdapterReference() == "" {
			t.Fatalf("%s must persist the reference so reconciliation can find the transaction", class)
		}
	}
}

func TestAdapterResultSubmittedPending(t *testing.T) {
	outcome := Outcome{Class: ClassSubmittedPending, Reference: testReference(), HasReference: true, ObservedAt: providerTestNow}
	result, err := outcome.AdapterResult("execution-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != execution.StatusSubmitted {
		t.Fatalf("unexpected status %s", result.Status())
	}
	if result.ErrorCode() != "" {
		t.Fatal("a pending submission is not an error")
	}
}

// The adapter is not permitted to assert on-chain success. Only the verifier,
// holding a receipt, may reach VERIFIED.
func TestAdapterResultNeverProducesVerified(t *testing.T) {
	for _, class := range []Classification{ClassVerifiedSuccess, ClassSubmittedPending, ClassAmbiguousSubmission, ClassUserAuthorizationRequired, ClassConfirmedOnchainFailed} {
		outcome := Outcome{Class: class, Reference: testReference(), HasReference: true, ObservedAt: providerTestNow}
		if class != ClassSubmittedPending && class != ClassVerifiedSuccess {
			outcome.ReasonCode = "PROVIDER_STATE_UNRECOGNIZED"
		}
		result, err := outcome.AdapterResult("execution-1")
		if err != nil {
			t.Fatalf("%s: %v", class, err)
		}
		if result.Status() == execution.StatusVerified {
			t.Fatalf("%s must never produce VERIFIED from an adapter", class)
		}
	}
}

func TestAdapterResultVerifiedSuccessDegradesToConfirming(t *testing.T) {
	outcome := Outcome{Class: ClassVerifiedSuccess, Reference: testReference(), HasReference: true, ObservedAt: providerTestNow}
	result, err := outcome.AdapterResult("execution-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != execution.StatusConfirming {
		t.Fatalf("expected CONFIRMING, got %s", result.Status())
	}
}

func TestAdapterResultConfirmedOnchainFailed(t *testing.T) {
	outcome := Outcome{Class: ClassConfirmedOnchainFailed, ReasonCode: "ONCHAIN_EXECUTION_REVERTED", Reference: testReference(), HasReference: true, ObservedAt: providerTestNow}
	result, err := outcome.AdapterResult("execution-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != execution.StatusFailed || result.ErrorCode() != "ONCHAIN_EXECUTION_REVERTED" {
		t.Fatalf("unexpected result %s/%s", result.Status(), result.ErrorCode())
	}
}

func TestAdapterResultUsesRequestContractVersion(t *testing.T) {
	value := testExecution(t)
	outcome := Outcome{Class: ClassSubmittedPending, Reference: testReference(), HasReference: true, ObservedAt: providerTestNow}
	result, err := outcome.AdapterResult(value.ExecutionID())
	if err != nil {
		t.Fatal(err)
	}
	if err := result.EnsureMatches(value.Request()); err != nil {
		t.Fatalf("adapter result must satisfy the Phase 9 request contract: %v", err)
	}
}

func TestVerificationResultOnlyReceiptSuccessVerifies(t *testing.T) {
	outcome := Outcome{Class: ClassVerifiedSuccess, Reference: testReference(), HasReference: true, ObservedAt: providerTestNow}
	result, err := outcome.VerificationResult()
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != runtime.VerificationVerified {
		t.Fatalf("expected VERIFIED, got %s", result.Outcome)
	}

	for _, class := range []Classification{ClassSubmittedPending, ClassAmbiguousSubmission, ClassUserAuthorizationRequired} {
		pending := Outcome{Class: class, Reference: testReference(), HasReference: true, ObservedAt: providerTestNow}
		if class != ClassSubmittedPending {
			pending.ReasonCode = "PROVIDER_STATE_UNRECOGNIZED"
		}
		verification, err := pending.VerificationResult()
		if err != nil {
			t.Fatalf("%s: %v", class, err)
		}
		if verification.Outcome != runtime.VerificationPending {
			t.Fatalf("%s must remain pending, got %s", class, verification.Outcome)
		}
	}
}

func TestVerificationResultTransientIsNotEvidence(t *testing.T) {
	outcome := Outcome{Class: ClassTransientProviderError, ReasonCode: "CHAIN_VERIFICATION_UNAVAILABLE", ObservedAt: providerTestNow}
	_, err := outcome.VerificationResult()
	var failure *runtime.VerificationError
	if !errors.As(err, &failure) {
		t.Fatalf("expected a typed verification error, got %v", err)
	}
}

func TestVerificationResultConfirmedOnchainFailed(t *testing.T) {
	outcome := Outcome{Class: ClassConfirmedOnchainFailed, ReasonCode: "ONCHAIN_EXECUTION_REVERTED", Reference: testReference(), HasReference: true, ObservedAt: providerTestNow}
	result, err := outcome.VerificationResult()
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != runtime.VerificationFailed || result.ReasonCode != "ONCHAIN_EXECUTION_REVERTED" {
		t.Fatalf("unexpected verification result %+v", result)
	}
}

func TestOutcomeValidate(t *testing.T) {
	cases := map[string]Outcome{
		"unknown class":            {Class: Classification("NOPE"), ReasonCode: "CODE_X", ObservedAt: providerTestNow},
		"missing observation time": {Class: ClassAmbiguousSubmission, ReasonCode: "CODE_X"},
		"pending with reason":      {Class: ClassSubmittedPending, ReasonCode: "CODE_X", Reference: testReference(), HasReference: true, ObservedAt: providerTestNow},
		"pending without ref":      {Class: ClassSubmittedPending, ObservedAt: providerTestNow},
		"failure without reason":   {Class: ClassAmbiguousSubmission, ObservedAt: providerTestNow},
		"lowercase reason":         {Class: ClassAmbiguousSubmission, ReasonCode: "provider_error", ObservedAt: providerTestNow},
		"invalid reference":        {Class: ClassSubmittedPending, Reference: Reference{Provider: ProviderCircleUserControlled}, HasReference: true, ObservedAt: providerTestNow},
	}
	for name, outcome := range cases {
		t.Run(name, func(t *testing.T) {
			if err := outcome.Validate(); err == nil {
				t.Fatalf("%s must be rejected", name)
			}
		})
	}
}

func TestValidReasonCode(t *testing.T) {
	valid := map[string]bool{
		"PROVIDER_UNREACHABLE": true, "A1": true,
		"": false, "A": false, "1ABC": false, "abc": false, "A-B": false, "A B": false,
	}
	for value, expected := range valid {
		if validReasonCode(value) != expected {
			t.Fatalf("validReasonCode(%q) != %v", value, expected)
		}
	}
}

var _ = errors.Is
