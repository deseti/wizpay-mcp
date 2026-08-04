package execution

import (
	"context"
	"testing"
	"time"
)

type mockAdapter struct{ result Result }

func (m mockAdapter) Execute(_ context.Context, request Request) (Result, error) {
	if err := request.Validate(); err != nil {
		return Result{}, err
	}
	return m.result, nil
}

func (m mockAdapter) GetStatus(_ context.Context, _ string) (Result, error) { return m.result, nil }

var _ Adapter = mockAdapter{}

func TestAdapterInterfaceWithLocalMock(t *testing.T) {
	request := mustRequest(t)
	result, err := NewResult(ResultParams{
		ExecutionID: request.ExecutionID(), ExecutionVersion: RequestVersion, Status: StatusSubmitted,
		AdapterReference: "opaque_adapter_operation_001", ObservedAt: executionTestNow.Add(9 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter := mockAdapter{result: result}
	got, err := adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExecutionID() != request.ExecutionID() || got.Status() != StatusSubmitted {
		t.Fatalf("result = (%s, %s)", got.ExecutionID(), got.Status())
	}
	if err := got.EnsureMatches(request); err != nil {
		t.Fatalf("EnsureMatches() error = %v", err)
	}
	observed, err := adapter.GetStatus(context.Background(), request.ExecutionID())
	if err != nil || observed.ObservedAt() != executionTestNow.Add(9*time.Second) {
		t.Fatalf("status result = (%+v, %v)", observed, err)
	}
}

func TestExecutionResultRejectsDifferentExecution(t *testing.T) {
	request := mustRequest(t)
	result, err := NewResult(ResultParams{ExecutionID: "exec_different", ExecutionVersion: 1, Status: StatusSubmitted, AdapterReference: "opaque_reference", ObservedAt: executionTestNow})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.EnsureMatches(request); !executionErrorCode(err, "execution_conflict") {
		t.Fatalf("mismatch error = %v", err)
	}
}

func TestExecutionResultRejectsMissingReferenceAndUnsafeErrorCode(t *testing.T) {
	request := mustRequest(t)
	if _, err := NewResult(ResultParams{ExecutionID: request.ExecutionID(), ExecutionVersion: 1, Status: StatusSubmitted, ObservedAt: executionTestNow}); !executionErrorCode(err, "execution_invalid") {
		t.Fatalf("missing reference error = %v", err)
	}
	if _, err := NewResult(ResultParams{ExecutionID: request.ExecutionID(), ExecutionVersion: 1, Status: StatusFailed, ErrorCode: "provider said secret detail", ObservedAt: executionTestNow}); !executionErrorCode(err, "execution_invalid") {
		t.Fatalf("unsafe error code error = %v", err)
	}
}

func TestExecutionTextLimitsAreScoped(t *testing.T) {
	request := mustRequest(t)
	// Generic execution text (execution ID) remains bounded at 256.
	longID := "e" + string(make([]byte, 256))
	// make zeros - need printable chars
	longID = "e" + stringsRepeat("x", 256)
	if _, err := NewResult(ResultParams{
		ExecutionID: longID, ExecutionVersion: 1, Status: StatusSubmitted,
		AdapterReference: "opaque-ref", ObservedAt: executionTestNow,
	}); !executionErrorCode(err, "execution_invalid") {
		t.Fatalf("execution ID >256 must be rejected, err=%v", err)
	}
	// Adapter reference may be 257..512.
	mid := stringsRepeat("a", 300)
	if _, err := NewResult(ResultParams{
		ExecutionID: request.ExecutionID(), ExecutionVersion: 1, Status: StatusSubmitted,
		AdapterReference: mid, ObservedAt: executionTestNow,
	}); err != nil {
		t.Fatalf("adapter reference length 300 must be accepted: %v", err)
	}
	// Adapter reference >512 is rejected.
	tooLong := stringsRepeat("b", 513)
	if _, err := NewResult(ResultParams{
		ExecutionID: request.ExecutionID(), ExecutionVersion: 1, Status: StatusSubmitted,
		AdapterReference: tooLong, ObservedAt: executionTestNow,
	}); !executionErrorCode(err, "execution_invalid") {
		t.Fatalf("adapter reference >512 must be rejected, err=%v", err)
	}
}

func stringsRepeat(value string, n int) string {
	out := make([]byte, 0, n*len(value))
	for i := 0; i < n; i++ {
		out = append(out, value...)
	}
	return string(out)
}
