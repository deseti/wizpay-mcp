package circle_test

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/send"
	"github.com/deseti/wizpay-mcp/internal/wallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// This fake is compiled only into tests. There is no HTTP client, SDK, token,
// signing key, broadcaster, or production execution-authority implementation.
// The externally simulated user decision is distinct from WizPay approval.
type localUCWChallenge struct {
	request    execution.Request
	authorized bool
	rejected   bool
	result     []byte
}
type localUCW struct {
	challenges map[string]*localUCWChallenge
	creations  int
}

func (f *localUCW) create(request execution.Request) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	key, err := providers.IdempotencyKey(request)
	if err != nil {
		return "", err
	}
	if old, ok := f.challenges[key]; ok {
		if old.request.IntentDigest() != request.IntentDigest() || old.request.ExecutionID() != request.ExecutionID() {
			return "", fmt.Errorf("challenge mismatch")
		}
		return key, nil
	}
	if f.challenges == nil {
		f.challenges = make(map[string]*localUCWChallenge)
	}
	f.challenges[key] = &localUCWChallenge{request: request}
	f.creations++
	return key, nil
}
func (f *localUCW) userDecision(key string, approved bool) error {
	c := f.challenges[key]
	if c == nil {
		return fmt.Errorf("unknown challenge")
	}
	if c.rejected || (c.authorized && !approved) {
		return fmt.Errorf("challenge decision is terminal")
	}
	c.authorized = approved
	c.rejected = !approved
	return nil
}
func (f *localUCW) observe(key string, raw []byte) error {
	c := f.challenges[key]
	if c == nil || !c.authorized {
		return fmt.Errorf("user authorization required")
	}
	if len(raw) == 0 {
		return fmt.Errorf("result unknown: reconcile same challenge")
	}
	if len(c.result) != 0 && !bytes.Equal(c.result, raw) {
		return fmt.Errorf("conflicting result: reconciliation required")
	}
	c.result = bytes.Clone(raw)
	return nil
}
func (f *localUCW) validate(key string, intent intents.Intent, bindings []wallet.Binding) error {
	c := f.challenges[key]
	if c == nil || !c.authorized || len(c.result) == 0 {
		return fmt.Errorf("authorization/result unavailable")
	}
	// No address-only inference or ambiguous selection. This is a test-harness
	// rule, not a new global production one-wallet-per-user storage constraint.
	if len(bindings) != 1 {
		return fmt.Errorf("exactly one verified binding required")
	}
	if c.request.IntentID() != intent.IntentID() || c.request.IntentVersion() != intent.Version() || c.request.IntentDigest() != intent.Digest() {
		return fmt.Errorf("intent challenge mismatch")
	}
	return send.ValidateSignedTransaction(intent, bindings[0], c.result)
}

// Synthetic public signature scalars r=s=1 allow real ECDSA sender recovery
// without generating, knowing, using, or storing any private key. The recovered
// address is fixture metadata only, not a verified Circle identity. Zero gas
// and fees ensure this fixture is not an executable transfer.
func localEnvelope(t *testing.T, kind uint8, change func(*types.DynamicFeeTx)) ([]byte, string) {
	t.Helper()
	to := common.HexToAddress(contracts.AddressUSDCMainnet)
	data := make([]byte, 68)
	copy(data, []byte{0xa9, 0x05, 0x9c, 0xbb})
	copy(data[16:36], common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes())
	big.NewInt(1000000).FillBytes(data[36:])
	p := &types.DynamicFeeTx{ChainID: big.NewInt(5042), To: &to, Value: new(big.Int), GasFeeCap: new(big.Int), GasTipCap: new(big.Int), Data: data, V: new(big.Int), R: big.NewInt(1), S: big.NewInt(1)}
	if change != nil {
		change(p)
	}
	var tx *types.Transaction
	switch kind {
	case types.LegacyTxType:
		tx = types.NewTx(&types.LegacyTx{To: p.To, Value: p.Value, GasPrice: new(big.Int), Data: p.Data, V: new(big.Int).Add(new(big.Int).Mul(p.ChainID, big.NewInt(2)), big.NewInt(35)), R: p.R, S: p.S})
	case types.AccessListTxType:
		tx = types.NewTx(&types.AccessListTx{ChainID: p.ChainID, To: p.To, Value: p.Value, GasPrice: new(big.Int), Data: p.Data, V: p.V, R: p.R, S: p.S})
	default:
		tx = types.NewTx(p)
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	sender, recoveryErr := types.Sender(types.LatestSignerForChainID(p.ChainID), tx)
	if change == nil && recoveryErr != nil {
		t.Fatal(recoveryErr)
	}
	return raw, contracts.NormalizeAddress(sender.Hex())
}

type localSendFixture struct {
	intent           intents.Intent
	approval         approvals.Approval
	explicitApproval approvals.Approval
	policy           policies.Policy
	result           policies.Result
	request          execution.Request
	binding          wallet.Binding
	identity         auth.IdentityContext
	now              time.Time
}

func localSend(t *testing.T, address string) localSendFixture {
	t.Helper()
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	binding, err := wallet.NewBinding(wallet.BindingParams{BindingID: "local-binding", Version: 1, UserID: "local-user", Provider: "circle", ProviderUserReference: "local-circle-user", WalletID: "local-wallet", Address: address, ChainID: "5042", Network: "MAINNET", Status: wallet.BindingStatusActive, VerificationReference: "local-mock-evidence", CreatedAt: now.Add(-time.Hour), VerifiedAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentity("local-user", "circle", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	identityContext, err := auth.NewIdentityContext(identity, auth.RequestMetadata{RequestID: "local-request"})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.NewDraft(policies.Params{PolicyID: "local-policy", Version: 1, Name: "Local SEND only", Scope: policies.Scope{UserID: "local-user", WalletBindingID: binding.BindingID(), IntentTypes: []intents.Type{intents.TypeSend}}, Rules: []policies.Rule{{RuleID: "send-only", OnViolation: policies.DecisionDeny, OperationAllowlist: &policies.OperationAllowlistRule{Allowed: []intents.Type{intents.TypeSend}}}}, CreatedAt: now, ValidFrom: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	policy, err = policy.Transition(policies.StatusActive, now)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := contracts.CanonicalToken("USDC")
	intent, err := intents.NewDraft(intents.Params{IntentID: "local-send", Version: 1, ClientRequestID: "local-client", Nonce: "local-nonce", Type: intents.TypeSend, Ownership: intents.Ownership{UserID: binding.OwnerUserID(), IdentityProvider: binding.Provider(), ProviderUserReference: binding.ProviderUserReference(), WalletBindingID: binding.BindingID(), WalletBindingVersion: binding.Version(), WalletID: binding.WalletID(), WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network()}, Financial: intents.FinancialParameters{Send: &intents.SendParameters{Token: intents.Token{ChainID: token.ChainID, Standard: "ERC20", Address: token.Address, Symbol: token.Symbol, Decimals: token.Decimals}, Recipient: "0x2222222222222222222222222222222222222222", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}}, Route: intents.Route{Type: intents.RouteDirectWallet, Reference: intents.RouteReferenceSend, Version: intents.RouteVersionSend}, Constraints: intents.Constraints{Deadline: now.Add(time.Hour), PolicyReference: policy.Reference()}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Transition(intents.StatusCreated, now)
	if err != nil {
		t.Fatal(err)
	}
	before, err := policies.EvaluateForApproval(policy, intent, identityContext, binding, now)
	// Result is a value carrying the exact immutable digest, not signing authority.
	if err != nil || before.Decision != policies.DecisionAllow || before.IntentDigest != intent.Digest() {
		t.Fatalf("pre-approval policy: %+v %v", before, err)
	}
	intent, err = intent.Transition(intents.StatusApprovalRequired, now)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.New(approvals.Params{ApprovalID: "local-approval", Version: 1, ApprovalRequestID: "local-approval-request", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, intent)
	if err != nil {
		t.Fatal(err)
	}
	approval, err = approval.Approve(now)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.Approve(approval, now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := policies.Evaluate(policy, intent, identityContext, binding, now)
	if err != nil || result.Decision != policies.DecisionAllow {
		t.Fatalf("policy: %+v %v", result, err)
	}
	operation, err := intents.NewOperationIdentity(intent)
	if err != nil {
		t.Fatal(err)
	}
	explicitApproval := approval
	approval, err = approval.Consume(now, operation)
	if err != nil {
		t.Fatal(err)
	}
	request, err := execution.NewRequest(intent, approval, result, now)
	if err != nil {
		t.Fatal(err)
	}
	return localSendFixture{intent: intent, approval: approval, explicitApproval: explicitApproval, policy: policy, result: result, request: request, binding: binding, identity: identityContext, now: now}
}
