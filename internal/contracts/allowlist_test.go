package contracts_test

import (
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestCanonicalDescriptorsStillRegister(t *testing.T) {
	registry := contracts.NewRegistry()
	for _, deployment := range contracts.DefaultDeployments() {
		if err := registry.Register(deployment); err != nil {
			t.Fatalf("canonical deployment %s failed: %v", deployment.ID, err)
		}
	}
	list := registry.List()
	if len(list) != 2 {
		t.Fatalf("list length = %d", len(list))
	}
}

func TestPayrollDescriptorRejectsPauseExecution(t *testing.T) {
	deployment := contracts.DefaultDeployments()[0]
	deployment.ExecutionFunctions = append(append([]string(nil), deployment.ExecutionFunctions...), "pause()")
	if err := contracts.NewRegistry().Register(deployment); !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("payroll with pause() should be rejected, got %v", err)
	}
}

func TestSwapDescriptorRejectsSetFeeBps(t *testing.T) {
	deployment := contracts.DefaultDeployments()[1]
	deployment.ExecutionFunctions = append(append([]string(nil), deployment.ExecutionFunctions...), "setFeeBps(uint256)")
	if err := contracts.NewRegistry().Register(deployment); !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("swap with setFeeBps should be rejected, got %v", err)
	}
}

func TestMissingRequiredExecutionFunctionRejected(t *testing.T) {
	deployment := contracts.DefaultDeployments()[0]
	// Drop routeAndPay.
	deployment.ExecutionFunctions = []string{
		"batchRouteAndPay(address,address[],address[],uint256[],uint256[],string)",
		"batchRouteAndPay(address,address,address[],uint256[],uint256[],string)",
	}
	if err := contracts.NewRegistry().Register(deployment); !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("missing execution function should be rejected, got %v", err)
	}
}

func TestExtraReadFunctionRejected(t *testing.T) {
	deployment := contracts.DefaultDeployments()[0]
	deployment.ReadFunctions = append(append([]string(nil), deployment.ReadFunctions...), "owner()")
	if err := contracts.NewRegistry().Register(deployment); !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("extra read function should be rejected, got %v", err)
	}
}

func TestExtraEventRejected(t *testing.T) {
	deployment := contracts.DefaultDeployments()[1]
	deployment.VerificationEvents = append(append([]string(nil), deployment.VerificationEvents...), "Paused(address)")
	if err := contracts.NewRegistry().Register(deployment); !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("extra event should be rejected, got %v", err)
	}
}

func TestAllowlistOrderDoesNotMatter(t *testing.T) {
	deployment := contracts.DefaultDeployments()[0]
	// Reverse execution functions; set equality must still accept.
	fns := append([]string(nil), deployment.ExecutionFunctions...)
	for i, j := 0, len(fns)-1; i < j; i, j = i+1, j-1 {
		fns[i], fns[j] = fns[j], fns[i]
	}
	deployment.ExecutionFunctions = fns
	if err := contracts.NewRegistry().Register(deployment); err != nil {
		t.Fatalf("reordered canonical allowlist should register: %v", err)
	}
}

func TestSubstitutedExecutionFunctionRejected(t *testing.T) {
	deployment := contracts.DefaultDeployments()[1]
	deployment.ExecutionFunctions = []string{"pause()"}
	if err := contracts.NewRegistry().Register(deployment); !hasCode(err, apperrors.CodeValidationError) {
		t.Fatalf("substituted execution function should be rejected, got %v", err)
	}
}
