package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	mcptools "github.com/deseti/wizpay-mcp/internal/mcp/tools"
	"github.com/deseti/wizpay-mcp/internal/services"
)

type trackFAcceptanceService struct {
	intent      intents.Intent
	previewed   int
	created     int
	executed    int
	statusReads int
}

func (s *trackFAcceptanceService) PreviewSend(context.Context, services.SendDraft) (services.SendPreview, error) {
	s.previewed++
	financial := s.intent.Financial().Send
	return services.SendPreview{Token: financial.Token, Recipient: financial.Recipient, Amount: financial.Amount, ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet}, nil
}

func (s *trackFAcceptanceService) CreateSendIntent(context.Context, services.SendDraft) (intents.Intent, error) {
	s.created++
	return s.intent, nil
}

func (s *trackFAcceptanceService) ExecuteSend(context.Context, string, string, string, uint64) (execution.Request, error) {
	s.executed++
	return execution.Request{}, apperrors.New(apperrors.CodeCapabilityUnavailable, "Arc Mainnet Send authorization provider is unavailable.", false, true, true)
}

func (s *trackFAcceptanceService) SendStatus(context.Context, string) (execution.Execution, error) {
	s.statusReads++
	return execution.Execution{}, apperrors.New(apperrors.CodeExecutionRecoverable, "Execution remains reconcilable.", true, false, false)
}

func (*trackFAcceptanceService) PreviewPayroll(context.Context, services.PayrollDraft) (services.PayrollPreview, error) {
	return services.PayrollPreview{}, fmt.Errorf("not used")
}
func (*trackFAcceptanceService) CreatePayrollIntent(context.Context, services.PayrollDraft) (intents.Intent, error) {
	return intents.Intent{}, fmt.Errorf("not used")
}
func (*trackFAcceptanceService) ExecutePayroll(context.Context, string, string, string, uint64) (execution.Request, error) {
	return execution.Request{}, apperrors.New(apperrors.CodeCapabilityUnavailable, "Arc Mainnet Payroll authorization provider is unavailable.", false, true, true)
}
func (*trackFAcceptanceService) PayrollStatus(context.Context, string) (execution.Execution, error) {
	return execution.Execution{}, apperrors.New(apperrors.CodeExecutionNotFound, "Execution was not found.", false, true, true)
}
func (*trackFAcceptanceService) PreviewSwap(context.Context, services.SwapDraft) (services.SwapPreview, error) {
	return services.SwapPreview{}, fmt.Errorf("not used")
}
func (*trackFAcceptanceService) CreateSwapIntent(context.Context, services.SwapDraft) (intents.Intent, error) {
	return intents.Intent{}, fmt.Errorf("not used")
}
func (*trackFAcceptanceService) ExecuteSwap(context.Context, string, string, string, uint64) (execution.Request, error) {
	return execution.Request{}, apperrors.New(apperrors.CodeCapabilityUnavailable, "Arc Mainnet Swap authorization provider is unavailable.", false, true, true)
}
func (*trackFAcceptanceService) SwapStatus(context.Context, string) (execution.Execution, error) {
	return execution.Execution{}, apperrors.New(apperrors.CodeExecutionNotFound, "Execution was not found.", false, true, true)
}

func TestTrackFMCPProtocolClientAcceptance(t *testing.T) {
	service := &trackFAcceptanceService{intent: trackFSendIntent(t)}
	registrations := make([]mcptools.Tool, 0, 12)
	for _, build := range []func() (*mcptools.Registry, error){
		func() (*mcptools.Registry, error) { return mcptools.NewSendRegistry(service) },
		func() (*mcptools.Registry, error) { return mcptools.NewPayrollRegistry(service) },
		func() (*mcptools.Registry, error) { return mcptools.NewSwapRegistry(service) },
	} {
		registry, err := build()
		if err != nil {
			t.Fatal(err)
		}
		registrations = append(registrations, registry.Tools()...)
	}
	server, err := NewServer(testLogger(), registrations...)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewStreamableHTTPHandler(server, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	// The protocol test includes the same fail-closed bearer boundary used by
	// the application without embedding a real credential in the repository.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer protocol-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		transport.ServeHTTP(w, r)
	})

	unauthorized := trackFProtocolRequest(handler, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	initialize := trackFProtocolRequest(handler, "Bearer protocol-test", `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"track-f-client","version":"1"}}}`)
	if initialize.Code != http.StatusOK || !strings.Contains(initialize.Body.String(), `"name":"wizpay-mcp"`) {
		t.Fatalf("initialize = %d %s", initialize.Code, initialize.Body.String())
	}

	listed := trackFProtocolRequest(handler, "Bearer protocol-test", `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`)
	if listed.Code != http.StatusOK {
		t.Fatalf("tools/list = %d %s", listed.Code, listed.Body.String())
	}
	for _, name := range []string{
		mcptools.SendPreviewName, mcptools.SendCreateIntentName, mcptools.SendExecuteName, mcptools.SendStatusName,
		mcptools.PayrollPreviewName, mcptools.PayrollCreateIntentName, mcptools.PayrollExecuteName, mcptools.PayrollStatusName,
		mcptools.SwapPreviewName, mcptools.SwapCreateIntentName, mcptools.SwapExecuteName, mcptools.SwapStatusName,
	} {
		if !strings.Contains(listed.Body.String(), `"name":"`+name+`"`) {
			t.Errorf("tools/list missing %s", name)
		}
	}
	for _, forbidden := range []string{"raw", "calldata", "contract.execute", "sign_transaction", "private_key", "spender"} {
		if strings.Contains(strings.ToLower(listed.Body.String()), forbidden) {
			t.Errorf("tools/list exposes forbidden surface %q", forbidden)
		}
	}

	deadline := "2026-09-28T12:20:00Z"
	args := `{"request_id":"req-track-f","client_request_id":"client-track-f","nonce":"nonce-track-f","wallet_binding_id":"binding-track-f","token":"USDC","recipient":"0x3333333333333333333333333333333333333333","amount":{"decimal":"1","base_units":"1000000","decimals":6},"deadline":"` + deadline + `","policy_reference":"policy:1"}`
	preview := trackFProtocolRequest(handler, "Bearer protocol-test", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"`+mcptools.SendPreviewName+`","arguments":`+args+`}}`)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), contracts.AddressUSDCMainnet) || !strings.Contains(preview.Body.String(), contracts.ChainIDArcMainnet) {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}
	create := trackFProtocolRequest(handler, "Bearer protocol-test", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"`+mcptools.SendCreateIntentName+`","arguments":`+args+`}}`)
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), service.intent.IntentID()) || !strings.Contains(create.Body.String(), service.intent.Digest()) {
		t.Fatalf("create_intent = %d %s", create.Code, create.Body.String())
	}
	execute := trackFProtocolRequest(handler, "Bearer protocol-test", `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"`+mcptools.SendExecuteName+`","arguments":{"request_id":"req-execute","intent_id":"intent-track-f","approval_id":"approval-track-f","policy_id":"policy","policy_version":1}}}`)
	if execute.Code != http.StatusOK || !strings.Contains(execute.Body.String(), string(apperrors.CodeCapabilityUnavailable)) {
		t.Fatalf("execute fail-closed = %d %s", execute.Code, execute.Body.String())
	}
	status := trackFProtocolRequest(handler, "Bearer protocol-test", `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"`+mcptools.SendStatusName+`","arguments":{"request_id":"req-status","execution_id":"exec-track-f"}}}`)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), string(apperrors.CodeExecutionRecoverable)) {
		t.Fatalf("status = %d %s", status.Code, status.Body.String())
	}
	malformed := trackFProtocolRequest(handler, "Bearer protocol-test", `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"`+mcptools.SendPreviewName+`","arguments":{"request_id":"req-malformed"}}}`)
	if malformed.Code != http.StatusOK || !strings.Contains(malformed.Body.String(), `"isError":true`) || !strings.Contains(malformed.Body.String(), "missing properties") {
		t.Fatalf("malformed request = %d %s", malformed.Code, malformed.Body.String())
	}
	if service.previewed != 1 || service.created != 1 || service.executed != 1 || service.statusReads != 1 {
		t.Fatalf("service calls = preview:%d create:%d execute:%d status:%d", service.previewed, service.created, service.executed, service.statusReads)
	}
}

func trackFProtocolRequest(handler http.Handler, authorization, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func trackFSendIntent(t *testing.T) intents.Intent {
	t.Helper()
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	token, err := contracts.CanonicalToken("USDC")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := intents.NewDraft(intents.Params{
		IntentID: "intent-track-f", Version: 1, ClientRequestID: "client-track-f", Nonce: "nonce-track-f", Type: intents.TypeSend,
		Ownership: intents.Ownership{UserID: "user-track-f", IdentityProvider: "test", ProviderUserReference: "provider-user-track-f", WalletBindingID: "binding-track-f", WalletBindingVersion: 1, WalletID: "wallet-track-f", WalletAddress: "0x2222222222222222222222222222222222222222", ChainID: contracts.ChainIDArcMainnet, Network: contracts.NetworkArcMainnet},
		Financial: intents.FinancialParameters{Send: &intents.SendParameters{Token: intents.Token{ChainID: token.ChainID, Standard: "ERC20", Address: token.Address, Symbol: token.Symbol, Decimals: token.Decimals}, Recipient: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}},
		Route:     intents.Route{Type: intents.RouteDirectWallet, Reference: intents.RouteReferenceSend, Version: intents.RouteVersionSend}, Constraints: intents.Constraints{Deadline: now.Add(20 * time.Minute), PolicyReference: "policy:1"}, CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := draft.Transition(intents.StatusCreated, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return created
}
