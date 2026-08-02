package execution

import (
	stderrors "errors"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestExecutionLifecycleValidTransitions(t *testing.T) {
	execution := mustExecution(t)
	statuses := []Status{StatusAuthorized, StatusQueued, StatusExecuting, StatusSubmitted, StatusConfirming, StatusConfirmed, StatusVerified, StatusCompleted}
	for index, status := range statuses {
		execution = advanceExecution(t, execution, status, executionTestNow.Add(time.Duration(8+index+1)*time.Second))
	}
	if !execution.Terminal() || execution.Revision() != uint64(len(statuses)+1) {
		t.Fatalf("execution = (%s, revision %d)", execution.Status(), execution.Revision())
	}
	if _, err := execution.Transition(StatusCancelled, executionTestNow.Add(time.Minute)); !executionErrorCode(err, apperrors.CodeExecutionInvalid) {
		t.Fatalf("terminal transition error = %v", err)
	}
}

func TestExecutionLifecycleRejectsInvalidTransition(t *testing.T) {
	execution := mustExecution(t)
	if _, err := execution.Transition(StatusQueued, executionTestNow.Add(9*time.Second)); !executionErrorCode(err, apperrors.CodeExecutionInvalid) {
		t.Fatalf("invalid transition error = %v", err)
	}
	cancelled := advanceExecution(t, execution, StatusCancelled, executionTestNow.Add(9*time.Second))
	if !cancelled.Terminal() {
		t.Fatal("cancelled execution is not terminal")
	}
	if _, err := cancelled.Transition(StatusAuthorized, executionTestNow.Add(10*time.Second)); !executionErrorCode(err, apperrors.CodeExecutionInvalid) {
		t.Fatalf("cancelled transition error = %v", err)
	}
}

func TestSameAuthorizedRequestProducesSameExecutionIdentity(t *testing.T) {
	intent, approval, result := requestParts(t)
	first, err := NewRequest(intent, approval, result, executionTestNow.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRequest(intent, approval, result, executionTestNow.Add(20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if first.ExecutionID() != second.ExecutionID() || first.RequestID() != second.RequestID() || first.RequestKey() != second.RequestKey() || first.CreatedAt() != second.CreatedAt() {
		t.Fatalf("request retry changed identity:\n%+v\n%+v", first, second)
	}
}

func TestExecutionRequestRejectsNonAllowingPolicyResult(t *testing.T) {
	intent, approval, result := requestParts(t)
	result.Decision = "DENY"
	if _, err := NewRequest(intent, approval, result, executionTestNow.Add(8*time.Second)); !executionErrorCode(err, apperrors.CodeExecutionNotAuthorized) {
		t.Fatalf("authorization error = %v", err)
	}
}

func TestFailedExecutionRecoveryIsExplicitAndDeterministic(t *testing.T) {
	execution := mustExecution(t)
	execution = advanceExecution(t, execution, StatusAuthorized, executionTestNow.Add(9*time.Second))
	execution = advanceExecution(t, execution, StatusQueued, executionTestNow.Add(10*time.Second))
	execution = advanceExecution(t, execution, StatusExecuting, executionTestNow.Add(11*time.Second))
	failed, err := execution.Fail(Failure{Code: "SAFE_RETRY_ALLOWED", Eligibility: RecoveryRecoverable, RecoveryTarget: StatusExecuting}, executionTestNow.Add(12*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Terminal() {
		t.Fatal("explicitly recoverable failure is terminal")
	}
	recoverable, err := failed.Recover(executionTestNow.Add(13 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if recoverable.Status() != StatusRecoveryRequired {
		t.Fatalf("status = %s", recoverable.Status())
	}
	recovery, ok := recoverable.Recovery()
	if !ok || recovery.Target != StatusExecuting || recovery.FromStatus != StatusExecuting {
		t.Fatalf("recovery = %+v", recovery)
	}
	resumed, err := recoverable.Resume(executionTestNow.Add(14 * time.Second))
	if err != nil || resumed.Status() != StatusExecuting {
		t.Fatalf("resume = (%s, %v)", resumed.Status(), err)
	}
}

func TestTerminalFailureCannotRecover(t *testing.T) {
	execution := mustExecution(t)
	execution = advanceExecution(t, execution, StatusAuthorized, executionTestNow.Add(9*time.Second))
	execution = advanceExecution(t, execution, StatusQueued, executionTestNow.Add(10*time.Second))
	execution = advanceExecution(t, execution, StatusExecuting, executionTestNow.Add(11*time.Second))
	failed, err := execution.Fail(Failure{Code: "PROVEN_TERMINAL_FAILURE", Eligibility: RecoveryTerminal}, executionTestNow.Add(12*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !failed.Terminal() {
		t.Fatal("terminal failure is not terminal")
	}
	if _, err := failed.Recover(executionTestNow.Add(13 * time.Second)); !executionErrorCode(err, apperrors.CodeExecutionFailed) {
		t.Fatalf("recovery error = %v", err)
	}
}

func TestAmbiguousStateRecoveryKeepsSameExecution(t *testing.T) {
	execution := mustExecution(t)
	originalID := execution.ExecutionID()
	execution = advanceExecution(t, execution, StatusAuthorized, executionTestNow.Add(9*time.Second))
	execution = advanceExecution(t, execution, StatusQueued, executionTestNow.Add(10*time.Second))
	execution = advanceExecution(t, execution, StatusExecuting, executionTestNow.Add(11*time.Second))
	execution = advanceExecution(t, execution, StatusSubmitted, executionTestNow.Add(12*time.Second))
	recovery, err := execution.RequireRecovery("SUBMISSION_OUTCOME_AMBIGUOUS", executionTestNow.Add(13*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := recovery.Resume(executionTestNow.Add(14 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ExecutionID() != originalID || resumed.Status() != StatusConfirming {
		t.Fatalf("resumed = (%s, %s)", resumed.ExecutionID(), resumed.Status())
	}
}

func executionErrorCode(err error, code apperrors.Code) bool {
	var appError *apperrors.Error
	return stderrors.As(err, &appError) && appError.Code == code
}
