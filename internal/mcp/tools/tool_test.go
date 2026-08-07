package tools

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/services"
)

type intentServiceStub struct {
	createCalls int
	getCalls    int
	err         error
}

func (s *intentServiceStub) CreateIntent(context.Context, services.CreateIntentCommand) (intents.Intent, error) {
	s.createCalls++
	return intents.Intent{}, s.err
}
func (s *intentServiceStub) GetIntent(context.Context, string) (intents.Intent, error) {
	s.getCalls++
	return intents.Intent{}, s.err
}

type approvalServiceStub struct{}

func (*approvalServiceStub) ListApprovals(context.Context, int, string, string) (services.ApprovalPage, error) {
	return services.ApprovalPage{}, nil
}

func (*approvalServiceStub) RequestApproval(context.Context, string) (approvals.Approval, error) {
	return approvals.Approval{}, nil
}
func (*approvalServiceStub) GetApproval(context.Context, string) (approvals.Approval, error) {
	return approvals.Approval{}, nil
}
func (*approvalServiceStub) DecideApproval(context.Context, string, approvals.Decision) (approvals.Approval, error) {
	return approvals.Approval{}, nil
}
func (*approvalServiceStub) AuthorizeExecution(context.Context, string, string, string, uint64) (services.ExecutionAuthorization, error) {
	return services.ExecutionAuthorization{}, nil
}

type policyServiceStub struct{}

func (*policyServiceStub) EvaluatePolicy(context.Context, string, string, uint64, policies.EvaluationStage) (policies.Result, error) {
	return policies.Result{}, nil
}

type executionServiceStub struct {
	calls                          int
	intentID, approvalID, policyID string
	policyVersion                  uint64
}

func (s *executionServiceStub) PrepareExecution(_ context.Context, intentID, approvalID, policyID string, policyVersion uint64) (execution.Request, error) {
	s.calls++
	s.intentID, s.approvalID, s.policyID, s.policyVersion = intentID, approvalID, policyID, policyVersion
	return execution.Request{}, nil
}

func testBundle(intentService services.IntentService, executionService services.ExecutionService) services.Bundle {
	if intentService == nil {
		intentService = &intentServiceStub{}
	}
	if executionService == nil {
		executionService = &executionServiceStub{}
	}
	return services.Bundle{Intents: intentService, Approvals: &approvalServiceStub{}, Policies: &policyServiceStub{}, Executions: executionService}
}

func TestFoundationRegistryContainsOnlyPermittedTools(t *testing.T) {
	registry, err := NewFoundationRegistry(testBundle(nil, nil))
	if err != nil {
		t.Fatalf("NewFoundationRegistry() error = %v", err)
	}
	want := []string{CreateIntentName, EvaluatePolicyName, GetApprovalName, GetIntentName, PrepareExecutionName, RequestApprovalName}
	definitions := registry.Definitions()
	if len(definitions) != len(want) {
		t.Fatalf("tool count = %d, want %d", len(definitions), len(want))
	}
	for index, definition := range definitions {
		if definition.Name() != want[index] {
			t.Errorf("tool[%d] = %q, want %q", index, definition.Name(), want[index])
		}
		if definition.Description() == "" || definition.InputSchema() == nil || definition.OutputSchema() == nil {
			t.Errorf("tool %q has incomplete metadata", definition.Name())
		}
		lower := strings.ToLower(definition.Name())
		if strings.Contains(lower, "execute") && definition.Name() != PrepareExecutionName {
			t.Errorf("unsafe execution tool registered: %q", definition.Name())
		}
		if strings.Contains(lower, "sign") || strings.Contains(lower, "submit") || strings.Contains(lower, "transaction") {
			t.Errorf("unsafe tool registered: %q", definition.Name())
		}
	}
}

func TestRegistryRejectsDuplicateAndSupportsLookup(t *testing.T) {
	registry, err := NewFoundationRegistry(testBundle(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Lookup(GetIntentName)
	if !ok {
		t.Fatal("get-intent definition not found")
	}
	if err := registry.Register(definition); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Register() error = %v, want duplicate", err)
	}
}

func TestRegistryRejectsInvalidDefinition(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Definition{name: "incomplete"}); err == nil {
		t.Fatal("incomplete definition accepted")
	}
}

func TestDefinitionSchemaValidation(t *testing.T) {
	registry, err := NewFoundationRegistry(testBundle(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := registry.Lookup(GetIntentName)
	if err := definition.ValidateInput(map[string]any{"request_id": "req_1", "intent_id": "intent_1"}); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if err := definition.ValidateInput(map[string]any{"request_id": "req_1"}); err == nil {
		t.Fatal("missing intent_id accepted")
	}
	if err := definition.ValidateInput(map[string]any{"request_id": "req_1", "intent_id": "intent_1", "extra": true}); err == nil {
		t.Fatal("unknown property accepted")
	}
	validOutput := map[string]any{"error": map[string]any{"code": "intent_not_found", "message": "Intent not found.", "request_id": "req_1", "retryable": false, "user_action_required": true, "terminal": true}}
	if err := definition.ValidateOutput(validOutput); err != nil {
		t.Fatalf("valid output rejected: %v", err)
	}
	if err := definition.ValidateOutput(map[string]any{"unsafe": "value"}); err == nil {
		t.Fatal("unknown output property accepted")
	}
}

func TestDefinitionsRegisterWithOfficialSDK(t *testing.T) {
	registry, err := NewFoundationRegistry(testBundle(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	for _, definition := range registry.Definitions() {
		if err := definition.Register(server); err != nil {
			t.Fatalf("Register(%s) error = %v", definition.Name(), err)
		}
	}
}

func TestInvalidInputDoesNotCallDomainService(t *testing.T) {
	service := &intentServiceStub{}
	handler := getIntentHandler(service)
	result, response, err := handler(context.Background(), nil, GetIntentInput{RequestID: "req_1"})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result == nil || !result.IsError || response.Error == nil || response.Error.Code != "validation_error" {
		t.Fatalf("response = %#v, result = %#v", response, result)
	}
	if service.getCalls != 0 {
		t.Fatalf("domain calls = %d, want 0", service.getCalls)
	}
}

func TestUnknownDomainErrorIsRedacted(t *testing.T) {
	service := &intentServiceStub{err: stderrors.New("database password secret")}
	handler := getIntentHandler(service)
	result, response, err := handler(context.Background(), nil, GetIntentInput{RequestID: "req_1", IntentID: "intent_1"})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result == nil || !result.IsError || response.Error == nil {
		t.Fatalf("response = %#v, result = %#v", response, result)
	}
	if response.Error.Code != "internal_error" || response.Error.RequestID != "req_1" {
		t.Fatalf("public error = %#v", response.Error)
	}
	if strings.Contains(response.Error.Message, "password") || strings.Contains(response.Error.Message, "secret") {
		t.Fatalf("unsafe error leaked: %q", response.Error.Message)
	}
}

func TestPrepareExecutionOnlyCallsPreparationPort(t *testing.T) {
	service := &executionServiceStub{}
	handler := prepareExecutionHandler(service)
	result, response, err := handler(context.Background(), nil, PrepareExecutionInput{RequestID: "req_1", IntentID: "intent_1", ApprovalID: "approval_1", PolicyID: "policy_1", PolicyVersion: 2})
	if err != nil || result != nil {
		t.Fatalf("handler result = %#v, error = %v", result, err)
	}
	if response.Result == nil || response.Result.Status != string(execution.StatusCreated) {
		t.Fatalf("response = %#v", response)
	}
	if service.calls != 1 || service.intentID != "intent_1" || service.approvalID != "approval_1" || service.policyID != "policy_1" || service.policyVersion != 2 {
		t.Fatalf("preparation call = %#v", service)
	}
}

func TestCreateIntentRejectsAmbiguousFinancialUnion(t *testing.T) {
	service := &intentServiceStub{}
	handler := createIntentHandler(service)
	_, response, err := handler(context.Background(), nil, CreateIntentInput{RequestID: "req_1", ClientRequestID: "client_1", Nonce: "nonce_1", WalletBindingID: "binding_1", IntentType: intents.TypeSwap, PolicyReference: "policy_1:1"})
	if err != nil || response.Error == nil || response.Error.Code != "validation_error" {
		t.Fatalf("response = %#v, error = %v", response, err)
	}
	if service.createCalls != 0 {
		t.Fatalf("domain calls = %d, want 0", service.createCalls)
	}
}
