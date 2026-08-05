package circle

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
	"github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/execution/runtime"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

var contractExecNow = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

type recordedRequest struct {
	Method string
	Path   string
	Body   []byte
}

type scriptedTransport struct {
	mu       sync.Mutex
	requests []recordedRequest
	// handler returns status and body for each request in order when set;
	// otherwise route-based defaults are used.
	handler func(req *http.Request, body []byte) (int, []byte)
	posts   atomic.Int64
}

func (t *scriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
	}
	t.mu.Lock()
	t.requests = append(t.requests, recordedRequest{Method: req.Method, Path: req.URL.Path, Body: append([]byte(nil), body...)})
	t.mu.Unlock()
	if req.Method == http.MethodPost {
		t.posts.Add(1)
	}

	status := http.StatusOK
	payload := []byte(`{"data":{}}`)
	if t.handler != nil {
		status, payload = t.handler(req, body)
	} else {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/transactions/contractExecution"):
			payload = []byte(`{"data":{"challengeId":"c4d1da72-111e-4d52-bdbf-2e74a2d803d5"}}`)
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/transactions/transfer"):
			payload = []byte(`{"data":{"challengeId":"a1b2c3d4-111e-4d52-bdbf-2e74a2d803d5"}}`)
		case req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/transactions/"):
			payload = []byte(`{"data":{"transaction":{"id":"tx-1","state":"COMPLETE","txHash":"0x1111111111111111111111111111111111111111111111111111111111111111","blockchain":"ARC-TESTNET","walletId":"wallet-test","refId":"exec"}}}`)
		case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/transactions"):
			payload = []byte(`{"data":{"transactions":[]}}`)
		case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/user/challenges"):
			payload = []byte(`{"data":{"challenges":[{"id":"c4d1da72-111e-4d52-bdbf-2e74a2d803d5","status":"PENDING","type":"TRANSACTION_CONTRACT_EXECUTION"}]}}`)
		}
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(payload)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func (t *scriptedTransport) snapshot() []recordedRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]recordedRequest, len(t.requests))
	copy(out, t.requests)
	return out
}

func contractAdapter(t *testing.T, plan providers.Plan, auth providers.AuthorizationSource, refs ReferenceStore, transport *scriptedTransport, now func() time.Time) *Adapter {
	t.Helper()
	if now == nil {
		now = func() time.Time { return contractExecNow }
	}
	httpClient := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	adapter, err := NewAdapter(adapterConfig(), httpClient, stubPlanner{plan: plan}, auth, refs, now)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	return adapter
}

func mustUserAuth(t *testing.T) providers.UserAuthorization {
	t.Helper()
	auth, err := providers.NewUserAuthorization("user-session-token-test")
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func mustPayrollEncodedCall(t *testing.T) contracts.EncodedCall {
	t.Helper()
	call, err := payroll.EncodeRouteAndPay(nil, payroll.SinglePayment{
		TokenIn: "0x1111111111111111111111111111111111111111", TokenOut: "0x5555555555555555555555555555555555555555",
		AmountIn: big.NewInt(1_250_000), MinAmountOut: big.NewInt(1_200_000),
		Recipient: "0x3333333333333333333333333333333333333333",
	})
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func mustSwapEncodedCall(t *testing.T) contracts.EncodedCall {
	t.Helper()
	call, err := swap.EncodeExecuteSwap(nil, swap.ExecuteSwapInput{
		Router:  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TokenIn: "0x1111111111111111111111111111111111111111", TokenOut: "0x5555555555555555555555555555555555555555",
		AmountIn: big.NewInt(10_000_000), MinAmountOut: big.NewInt(9_000_000),
		Recipient: "0x2222222222222222222222222222222222222222",
		Deadline:  contractExecNow.Add(15 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func contractPlan(t *testing.T, call contracts.EncodedCall, notAfter time.Time) providers.Plan {
	t.Helper()
	plan, err := providers.NewContractExecutionPlan(providers.ContractExecutionParams{
		WalletBindingID: "binding-test", WalletID: "wallet-test",
		WalletAddress: adapterSource,
		ChainID:       "5042002", Network: "TESTNET",
		Call: call, SubmitNotAfter: notAfter,
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func validExecutionRequest(t *testing.T) execution.Request {
	t.Helper()
	now := contractExecNow
	policy, err := policies.NewDraft(policies.Params{
		PolicyID: "policy-test", Version: 1, Name: "Contract exec test",
		Scope: policies.Scope{UserID: "user-test", WalletBindingID: "binding-test"},
		Rules: []policies.Rule{{RuleID: "operations", OnViolation: policies.DecisionDeny,
			OperationAllowlist: &policies.OperationAllowlistRule{Allowed: []intents.Type{intents.TypePayroll, intents.TypeSwap}}}},
		CreatedAt: now.Add(-time.Hour), ValidFrom: now.Add(-30 * time.Minute), ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err = policy.Transition(policies.StatusActive, policy.ValidFrom())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentity("user-test", "circle", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	identityContext, err := auth.NewIdentityContext(identity, auth.RequestMetadata{RequestID: "request-test"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := wallet.NewBinding(wallet.BindingParams{
		BindingID: "binding-test", Version: 1, UserID: "user-test", Provider: "circle",
		ProviderUserReference: "provider-user-test", WalletID: "wallet-test", Address: adapterSource,
		ChainID: "5042002", Network: "TESTNET", Status: wallet.BindingStatusActive,
		VerificationReference: "binding-verification",
		CreatedAt:             now.Add(-2 * time.Hour), VerifiedAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	token := intents.Token{ChainID: "5042002", Standard: "ERC20", Address: "0x1111111111111111111111111111111111111111", Symbol: "USDC", Decimals: 6}
	intent, err := intents.NewDraft(intents.Params{
		IntentID: "intent-contract-exec", Version: 1, ClientRequestID: "client-contract-exec", Nonce: "nonce-contract-exec",
		Type: intents.TypePayroll,
		Ownership: intents.Ownership{
			UserID: "user-test", IdentityProvider: "circle", ProviderUserReference: "provider-user-test",
			WalletBindingID: "binding-test", WalletBindingVersion: 1, WalletID: "wallet-test",
			WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network(),
		},
		Financial: intents.FinancialParameters{Payroll: &intents.PayrollParameters{
			Token:      token,
			Recipients: []intents.Recipient{{Address: adapterDestination, Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}},
			Total:      intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6},
		}},
		Route:       intents.Route{Type: intents.RouteAllowlistedContract, Reference: "route-test", Version: 1},
		Constraints: intents.Constraints{Deadline: now.Add(50 * time.Minute), PolicyReference: policy.Reference()},
		CreatedAt:   now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusApprovalRequired, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.New(approvals.Params{
		ApprovalID: "approval-contract-exec", Version: 1, ApprovalRequestID: "approval-request-contract-exec",
		CreatedAt: now.Add(3 * time.Second), ExpiresAt: now.Add(40 * time.Minute),
	}, intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Approve(now.Add(4 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Approve(approval, now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	policyResult, err := policies.Evaluate(policy, intent, identityContext, binding, now.Add(6*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := intents.NewOperationIdentity(intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Consume(now.Add(7*time.Second), operation)
	if err != nil {
		t.Fatal(err)
	}
	request, err := execution.NewRequest(intent, approval, policyResult, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestExecutePayrollContractExecution(t *testing.T) {
	call := mustPayrollEncodedCall(t)
	plan := contractPlan(t, call, contractExecNow.Add(10*time.Minute))
	transport := &scriptedTransport{}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport, nil)
	request := validExecutionRequest(t)

	result, err := adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status() != execution.StatusRecoveryRequired {
		t.Fatalf("status = %s", result.Status())
	}
	if result.ErrorCode() != "USER_AUTHORIZATION_REQUIRED" {
		t.Fatalf("error = %s", result.ErrorCode())
	}

	reqs := transport.snapshot()
	if len(reqs) != 1 || reqs[0].Method != http.MethodPost || !strings.HasSuffix(reqs[0].Path, "/v1/w3s/user/transactions/contractExecution") {
		t.Fatalf("unexpected requests: %+v", reqs)
	}
	var body map[string]any
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["contractAddress"] != call.To() {
		t.Fatalf("contractAddress = %v want %s", body["contractAddress"], call.To())
	}
	wantCallData := "0x" + hex.EncodeToString(call.CallData())
	if body["callData"] != wantCallData {
		t.Fatalf("callData mismatch")
	}
	if body["walletId"] != "wallet-test" {
		t.Fatalf("walletId = %v", body["walletId"])
	}
	if body["refId"] != request.ExecutionID() {
		t.Fatalf("refId = %v", body["refId"])
	}
	if body["feeLevel"] != feeLevelMedium {
		t.Fatalf("feeLevel = %v", body["feeLevel"])
	}
	// No ABI parameter path: sealed calldata only.
	if _, ok := body["abiFunctionSignature"]; ok {
		t.Fatal("abiFunctionSignature must not be sent when using callData")
	}
	if _, ok := body["abiParameters"]; ok {
		t.Fatal("abiParameters must not be sent when using callData")
	}
	if transport.posts.Load() != 1 {
		t.Fatalf("posts = %d", transport.posts.Load())
	}
}

func TestExecuteSwapContractExecution(t *testing.T) {
	call := mustSwapEncodedCall(t)
	// Swap freshness combines quote expiry and frozen deadline via EarliestDeadline.
	quoteExpiry := contractExecNow.Add(12 * time.Minute)
	swapDeadline := contractExecNow.Add(15 * time.Minute)
	notAfter := providers.EarliestDeadline(quoteExpiry, swapDeadline, contractExecNow.Add(time.Hour))
	plan := contractPlan(t, call, notAfter)
	transport := &scriptedTransport{}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport, nil)
	request := validExecutionRequest(t)

	result, err := adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status() != execution.StatusRecoveryRequired || result.ErrorCode() != "USER_AUTHORIZATION_REQUIRED" {
		t.Fatalf("result status=%s err=%s", result.Status(), result.ErrorCode())
	}
	reqs := transport.snapshot()
	if len(reqs) != 1 || !strings.HasSuffix(reqs[0].Path, "/contractExecution") {
		t.Fatalf("expected contractExecution POST, got %+v", reqs)
	}
	var body map[string]any
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	address, _ := body["contractAddress"].(string)
	if !contracts.AddressesEqual(address, call.To()) || !contracts.AddressesEqual(address, contracts.AddressWizPaySwapExecutor) {
		t.Fatalf("contractAddress = %v want sealed swap executor", body["contractAddress"])
	}
}

func TestExecuteUnsupportedContractRejected(t *testing.T) {
	// Kind CONTRACT_EXECUTION without sealed call is unsupported/invalid.
	plan := providers.Plan{
		Kind:            providers.PlanKindContractExecution,
		WalletBindingID: "binding-test", WalletID: "wallet-test", WalletAddress: adapterSource,
		ChainID: "5042002", Network: "TESTNET",
	}
	transport := &scriptedTransport{}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport, nil)
	_, err := adapter.Execute(context.Background(), validExecutionRequest(t))
	assertPermanent(t, execution.Result{}, err, "SUBMISSION_PLAN_INVALID")
	if transport.posts.Load() != 0 {
		t.Fatal("unsupported plan must never submit")
	}
}

func TestExecuteWrongChainRejected(t *testing.T) {
	call := mustPayrollEncodedCall(t)
	plan, err := providers.NewContractExecutionPlan(providers.ContractExecutionParams{
		WalletBindingID: "binding-test", WalletID: "wallet-test", WalletAddress: adapterSource,
		ChainID: call.ChainID(), Network: call.Network(),
		Call: call, SubmitNotAfter: contractExecNow.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Force plan chain to disagree with adapter config after construction.
	plan.ChainID = "1"
	// Validate path: plan.Validate will fail chain match with encoded call first.
	transport := &scriptedTransport{}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport, nil)
	_, execErr := adapter.Execute(context.Background(), validExecutionRequest(t))
	assertPermanent(t, execution.Result{}, execErr, "SUBMISSION_PLAN_INVALID")
	if transport.posts.Load() != 0 {
		t.Fatal("wrong chain must never submit")
	}
}

func TestExecuteWrongRegistryVersionRejected(t *testing.T) {
	call := mustPayrollEncodedCall(t)
	plan := contractPlan(t, call, contractExecNow.Add(time.Minute))
	// Inject a registry that lacks the expected version binding by using empty registry.
	transport := &scriptedTransport{}
	httpClient := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	adapter, err := NewAdapter(adapterConfig(), httpClient, stubPlanner{plan: plan}, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, func() time.Time { return contractExecNow })
	if err != nil {
		t.Fatal(err)
	}
	adapter.registry = contracts.NewRegistry() // no deployments registered
	_, execErr := adapter.Execute(context.Background(), validExecutionRequest(t))
	assertPermanent(t, execution.Result{}, execErr, "SUBMISSION_PLAN_INVALID")
	if transport.posts.Load() != 0 {
		t.Fatal("unregistered deployment must never submit")
	}
}

func TestExecuteNoCallerOverrideForTargetCalldata(t *testing.T) {
	call := mustPayrollEncodedCall(t)
	plan := contractPlan(t, call, contractExecNow.Add(time.Minute))
	// Attempting to override transfer-style destination fields is rejected.
	plan.DestinationAddress = "0x9999999999999999999999999999999999999999"
	plan.TokenID = "evil"
	plan.Amount = "999"
	transport := &scriptedTransport{}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport, nil)
	_, err := adapter.Execute(context.Background(), validExecutionRequest(t))
	assertPermanent(t, execution.Result{}, err, "SUBMISSION_PLAN_INVALID")
	if transport.posts.Load() != 0 {
		t.Fatal("override attempt must never submit")
	}

	// Sealed plan still posts only EncodedCall.To/CallData when clean.
	clean := contractPlan(t, call, contractExecNow.Add(time.Minute))
	transport = &scriptedTransport{}
	adapter = contractAdapter(t, clean, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport, nil)
	if _, err := adapter.Execute(context.Background(), validExecutionRequest(t)); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(transport.snapshot()[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["contractAddress"] != call.To() {
		t.Fatalf("target must come from EncodedCall only, got %v", body["contractAddress"])
	}
	if body["callData"] != "0x"+hex.EncodeToString(call.CallData()) {
		t.Fatal("calldata must come from EncodedCall only")
	}
}

func TestGetStatusNeverResubmitsAfterChallenge(t *testing.T) {
	// Simulates submission_started=true path: only GetStatus, never POST again.
	call := mustPayrollEncodedCall(t)
	plan := contractPlan(t, call, contractExecNow.Add(time.Minute))
	transport := &scriptedTransport{}
	reference := providers.Reference{
		Provider: providers.ProviderCircleUserControlled, ChainID: "5042002",
		WalletID: "wallet-test", ChallengeID: "c4d1da72-111e-4d52-bdbf-2e74a2d803d5",
	}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true},
		stubReferences{reference: reference, found: true}, transport, nil)

	result, err := adapter.GetStatus(context.Background(), "exec_after_submit")
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if result.Status() != execution.StatusRecoveryRequired {
		t.Fatalf("status = %s", result.Status())
	}
	if transport.posts.Load() != 0 {
		t.Fatalf("GetStatus must not POST, posts=%d", transport.posts.Load())
	}
	for _, req := range transport.snapshot() {
		if req.Method == http.MethodPost {
			t.Fatalf("unexpected POST %s", req.Path)
		}
	}
}

func TestAmbiguousProviderResponseReconcileOnly(t *testing.T) {
	call := mustPayrollEncodedCall(t)
	plan := contractPlan(t, call, contractExecNow.Add(time.Minute))
	transport := &scriptedTransport{handler: func(req *http.Request, body []byte) (int, []byte) {
		if req.Method == http.MethodPost {
			return http.StatusInternalServerError, []byte(`{"code":500,"message":"unavailable"}`)
		}
		return http.StatusOK, []byte(`{"data":{}}`)
	}}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport, nil)
	result, err := adapter.Execute(context.Background(), validExecutionRequest(t))
	if err != nil {
		t.Fatalf("ambiguous transport must not return adapter error, got %v", err)
	}
	if result.Status() != execution.StatusRecoveryRequired {
		t.Fatalf("status = %s", result.Status())
	}
	if result.ErrorCode() != "PROVIDER_UNAVAILABLE" {
		t.Fatalf("error = %s", result.ErrorCode())
	}
	// Runtime would mark submission_started and only call GetStatus next.
	// Adapter must not treat this as clean pre-submission failure (no permanent error).
	var adapterErr *runtime.AdapterError
	if errors.As(err, &adapterErr) {
		t.Fatal("ambiguous submission must not be a typed adapter error that invites resubmit semantics")
	}
}

func TestProviderCompleteIsNotVerified(t *testing.T) {
	call := mustPayrollEncodedCall(t)
	plan := contractPlan(t, call, contractExecNow.Add(time.Minute))
	transport := &scriptedTransport{handler: func(req *http.Request, body []byte) (int, []byte) {
		if strings.Contains(req.URL.Path, "/transactions/") && req.Method == http.MethodGet && !strings.HasSuffix(req.URL.Path, "/transactions") {
			return http.StatusOK, []byte(`{"data":{"transaction":{"id":"tx-complete","state":"COMPLETE","txHash":"0x1111111111111111111111111111111111111111111111111111111111111111","blockchain":"ARC-TESTNET","walletId":"wallet-test","refId":"exec_after_submit"}}}`)
		}
		if strings.HasSuffix(req.URL.Path, "/transactions") {
			return http.StatusOK, []byte(`{"data":{"transactions":[]}}`)
		}
		if strings.HasSuffix(req.URL.Path, "/user/challenges") {
			return http.StatusOK, []byte(`{"data":{"challenges":[]}}`)
		}
		return http.StatusOK, []byte(`{"data":{}}`)
	}}
	reference := providers.Reference{
		Provider: providers.ProviderCircleUserControlled, ChainID: "5042002",
		WalletID: "wallet-test", ProviderTransactionID: "tx-complete",
	}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true},
		stubReferences{reference: reference, found: true}, transport, nil)
	result, err := adapter.GetStatus(context.Background(), "exec_after_submit")
	if err != nil {
		t.Fatal(err)
	}
	// COMPLETE maps to submitted-pending observation, never VERIFIED.
	if result.Status() != execution.StatusSubmitted {
		t.Fatalf("COMPLETE must be submitted/pending observation, got %s", result.Status())
	}
	if result.Status() == execution.StatusVerified || result.Status() == execution.StatusCompleted {
		t.Fatal("provider COMPLETE must never become VERIFIED/COMPLETED")
	}
}

func TestTransferPathStillPasses(t *testing.T) {
	plan := validPlan()
	transport := &scriptedTransport{}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport, nil)
	result, err := adapter.Execute(context.Background(), validExecutionRequest(t))
	if err != nil {
		t.Fatalf("transfer Execute: %v", err)
	}
	if result.Status() != execution.StatusRecoveryRequired {
		t.Fatalf("status = %s", result.Status())
	}
	reqs := transport.snapshot()
	if len(reqs) != 1 || !strings.HasSuffix(reqs[0].Path, "/transactions/transfer") {
		t.Fatalf("expected transfer POST, got %+v", reqs)
	}
	var body map[string]any
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["destinationAddress"] != adapterDestination {
		t.Fatalf("destination = %v", body["destinationAddress"])
	}
	if body["tokenId"] != "token-test" {
		t.Fatalf("tokenId = %v", body["tokenId"])
	}
}

func TestPreFirstSubmissionExpiryRejected(t *testing.T) {
	call := mustPayrollEncodedCall(t)
	// Bound is in the past relative to adapter clock.
	plan := contractPlan(t, call, contractExecNow.Add(-time.Second))
	transport := &scriptedTransport{}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport, nil)
	_, err := adapter.Execute(context.Background(), validExecutionRequest(t))
	assertPermanent(t, execution.Result{}, err, "FINANCIAL_MATERIAL_EXPIRED")
	if transport.posts.Load() != 0 {
		t.Fatal("expired material must never submit")
	}
}

func TestReconcileDespiteLaterExpiry(t *testing.T) {
	call := mustPayrollEncodedCall(t)
	// Plan was fresh at construction; after possible submission the clock advances
	// past SubmitNotAfter. GetStatus must still reconcile and must not POST.
	plan := contractPlan(t, call, contractExecNow.Add(time.Minute))
	transport := &scriptedTransport{}
	reference := providers.Reference{
		Provider: providers.ProviderCircleUserControlled, ChainID: "5042002",
		WalletID: "wallet-test", ChallengeID: "c4d1da72-111e-4d52-bdbf-2e74a2d803d5",
	}
	late := contractExecNow.Add(2 * time.Hour)
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true},
		stubReferences{reference: reference, found: true}, transport, func() time.Time { return late })

	if !plan.FreshnessExpired(late) {
		t.Fatal("test clock must be past freshness bound")
	}
	result, err := adapter.GetStatus(context.Background(), "exec_after_submit")
	if err != nil {
		t.Fatalf("GetStatus after expiry: %v", err)
	}
	if result.Status() == "" {
		t.Fatal("expected a reconciliation result")
	}
	if transport.posts.Load() != 0 {
		t.Fatalf("expiry after submission must not resubmit, posts=%d", transport.posts.Load())
	}
}

func TestSwapFreshnessUsesQuoteAndDeadline(t *testing.T) {
	call := mustSwapEncodedCall(t)
	quoteExpiry := contractExecNow.Add(5 * time.Minute)
	deadline := contractExecNow.Add(20 * time.Minute)
	intentExpiry := contractExecNow.Add(time.Hour)
	notAfter := providers.EarliestDeadline(quoteExpiry, deadline, intentExpiry)
	if !notAfter.Equal(quoteExpiry) {
		t.Fatalf("earliest must be quote expiry, got %s", notAfter)
	}
	plan := contractPlan(t, call, notAfter)
	// At quote expiry boundary, first submission is rejected.
	transport := &scriptedTransport{}
	adapter := contractAdapter(t, plan, stubAuthorization{auth: mustUserAuth(t), found: true}, stubReferences{}, transport,
		func() time.Time { return quoteExpiry })
	_, err := adapter.Execute(context.Background(), validExecutionRequest(t))
	assertPermanent(t, execution.Result{}, err, "FINANCIAL_MATERIAL_EXPIRED")
}
