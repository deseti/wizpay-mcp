package tools

import (
	"context"
	"sort"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/services"
)

type swapServiceStub struct{}

func (*swapServiceStub) PreviewSwap(context.Context, services.SwapDraft) (services.SwapPreview, error) {
	return services.SwapPreview{}, nil
}
func (*swapServiceStub) CreateSwapIntent(context.Context, services.SwapDraft) (intents.Intent, error) {
	return intents.Intent{}, nil
}
func (*swapServiceStub) ExecuteSwap(context.Context, string, string, string, uint64) (execution.Request, error) {
	return execution.Request{}, nil
}
func (*swapServiceStub) SwapStatus(context.Context, string) (execution.Execution, error) {
	return execution.Execution{}, nil
}

func TestSwapRegistryExposesOnlyTypedLifecycle(t *testing.T) {
	registry, err := NewSwapRegistry(&swapServiceStub{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range registry.Tools() {
		names = append(names, tool.Name())
	}
	sort.Strings(names)
	want := []string{SwapCreateIntentName, SwapExecuteName, SwapPreviewName, SwapStatusName}
	sort.Strings(want)
	if len(names) != len(want) {
		t.Fatalf("names=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names=%v", names)
		}
	}
	execute, _ := registry.Lookup(SwapExecuteName)
	for _, forbidden := range []string{"token_in", "token_out", "amount_in", "min_amount_out", "min_hop_price_x36", "deadline", "router", "target", "calldata", "recipient", "sender"} {
		if _, exists := execute.InputSchema().Properties[forbidden]; exists {
			t.Fatalf("execute exposes %q", forbidden)
		}
	}
}
