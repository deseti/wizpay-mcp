package tools

import (
	"context"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/services"
)

type sendServiceStub struct {
	previewCalls int
	createCalls  int
	executeCalls int
	statusCalls  int
	lastDraft    services.SendDraft
}

func (s *sendServiceStub) PreviewSend(_ context.Context, draft services.SendDraft) (services.SendPreview, error) {
	s.previewCalls++
	s.lastDraft = draft
	return services.SendPreview{}, nil
}
func (s *sendServiceStub) CreateSendIntent(_ context.Context, draft services.SendDraft) (intents.Intent, error) {
	s.createCalls++
	s.lastDraft = draft
	return intents.Intent{}, nil
}

func TestSendPreviewCallsOnlyValidationBoundary(t *testing.T) {
	service := &sendServiceStub{}
	handler := sendPreviewHandler(service)
	in := SendDraftInput{RequestID: "request", ClientRequestID: "client", Nonce: "nonce", WalletBindingID: "binding", Token: "USDC", Recipient: "0x2222222222222222222222222222222222222222", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}, Deadline: time.Date(2026, 9, 27, 11, 0, 0, 0, time.UTC), PolicyReference: "policy:1"}
	if _, _, err := handler(context.Background(), nil, in); err != nil {
		t.Fatal(err)
	}
	if service.previewCalls != 1 || service.createCalls != 0 || service.executeCalls != 0 || service.statusCalls != 0 {
		t.Fatalf("unexpected calls: %+v", service)
	}
}
func (s *sendServiceStub) ExecuteSend(context.Context, string, string, string, uint64) (execution.Request, error) {
	s.executeCalls++
	return execution.Request{}, nil
}
func (s *sendServiceStub) SendStatus(context.Context, string) (execution.Execution, error) {
	s.statusCalls++
	return execution.Execution{}, nil
}

func TestSendRegistryHasOnlyBoundedDomainTools(t *testing.T) {
	registry, err := NewSendRegistry(&sendServiceStub{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{SendCreateIntentName, SendExecuteName, SendPreviewName, SendStatusName}
	definitions := registry.Definitions()
	if len(definitions) != len(want) {
		t.Fatalf("tool count = %d", len(definitions))
	}
	for i := range want {
		if definitions[i].Name() != want[i] {
			t.Fatalf("tool %d = %s, want %s", i, definitions[i].Name(), want[i])
		}
	}
}

func TestSendSchemasRejectArbitraryCallAndExecuteOverrides(t *testing.T) {
	registry, err := NewSendRegistry(&sendServiceStub{})
	if err != nil {
		t.Fatal(err)
	}
	preview, _ := registry.Lookup(SendPreviewName)
	valid := map[string]any{"request_id": "req", "client_request_id": "client", "nonce": "nonce", "wallet_binding_id": "binding", "token": "USDC", "recipient": "0x2222222222222222222222222222222222222222", "amount": map[string]any{"decimal": "1", "base_units": "1000000", "decimals": float64(6)}, "deadline": "2026-09-27T11:00:00Z", "policy_reference": "policy:1"}
	if err := preview.ValidateInput(valid); err != nil {
		t.Fatalf("valid preview rejected: %v", err)
	}
	for _, field := range []string{"calldata", "contract_address", "router", "spender", "sender", "private_key", "signing_secret"} {
		bad := make(map[string]any, len(valid)+1)
		for key, value := range valid {
			bad[key] = value
		}
		bad[field] = "unsafe"
		if err := preview.ValidateInput(bad); err == nil {
			t.Fatalf("preview accepted %s", field)
		}
	}
	execute, _ := registry.Lookup(SendExecuteName)
	if err := execute.ValidateInput(map[string]any{"request_id": "req", "intent_id": "intent", "approval_id": "approval", "policy_id": "policy", "policy_version": float64(1), "recipient": "override"}); err == nil {
		t.Fatal("execute accepted replacement financial material")
	}
}
