package tools

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/services"
)

type payrollServiceStub struct{ preview, create, execute, status int }

func (s *payrollServiceStub) PreviewPayroll(context.Context, services.PayrollDraft) (services.PayrollPreview, error) {
	s.preview++
	return services.PayrollPreview{}, nil
}
func (s *payrollServiceStub) CreatePayrollIntent(context.Context, services.PayrollDraft) (intents.Intent, error) {
	s.create++
	return intents.Intent{}, nil
}
func (s *payrollServiceStub) ExecutePayroll(context.Context, string, string, string, uint64) (execution.Request, error) {
	s.execute++
	return execution.Request{}, nil
}
func (s *payrollServiceStub) PayrollStatus(context.Context, string) (execution.Execution, error) {
	s.status++
	return execution.Execution{}, nil
}

func TestPayrollRegistryExposesOnlyTypedLifecycle(t *testing.T) {
	registry, err := NewPayrollRegistry(&payrollServiceStub{})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(registry.Tools()))
	for _, tool := range registry.Tools() {
		names = append(names, tool.Name())
	}
	sort.Strings(names)
	want := []string{PayrollCreateIntentName, PayrollExecuteName, PayrollPreviewName, PayrollStatusName}
	sort.Strings(want)
	if len(names) != len(want) {
		t.Fatalf("names = %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v", names)
		}
	}
	execute, _ := registry.Lookup(PayrollExecuteName)
	for _, forbidden := range []string{"token", "recipients", "amounts", "reference_id", "calldata", "contract", "router", "sender", "private_key"} {
		if _, exists := execute.InputSchema().Properties[forbidden]; exists {
			t.Fatalf("execute exposes %q", forbidden)
		}
	}
	preview, _ := registry.Lookup(PayrollPreviewName)
	for _, required := range []string{"output_token", "gross_input", "min_total_out", "min_hop_price_x36", "swap_deadline"} {
		if _, ok := preview.InputSchema().Properties[required]; !ok {
			t.Fatalf("preview missing Track E field %q", required)
		}
	}
}

func TestPayrollDraftValidationBoundsReferenceByUTF8Bytes(t *testing.T) {
	in := PayrollDraftInput{RequestID: "request", ClientRequestID: "client", Nonce: "nonce", WalletBindingID: "binding", Token: "USDC", ReferenceID: "ééééééééééééééééééééééééééééééééé", PolicyReference: "policy", Deadline: time.Now().Add(time.Hour), Recipients: []PayrollRecipientInput{{Address: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}}}
	if err := in.Validate(); err == nil {
		t.Fatal("reference over 64 UTF-8 bytes must fail")
	}
}
