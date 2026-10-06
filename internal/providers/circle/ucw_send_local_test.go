package circle_test

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/providers/circle"
	"github.com/deseti/wizpay-mcp/internal/providers/wiring"
	"github.com/deseti/wizpay-mcp/internal/send"
	"github.com/deseti/wizpay-mcp/internal/wallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestLocalUCWSendAuthorizationAndRetry(t *testing.T) {
	for _, kind := range []uint8{types.LegacyTxType, types.AccessListTxType, types.DynamicFeeTxType} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			raw, address := localEnvelope(t, kind, nil)
			s := localSend(t, address)
			f := &localUCW{}
			key, err := f.create(s.request)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.observe(key, raw); err == nil {
				t.Fatal("unsigned user decision accepted")
			}
			declined := &localUCW{}
			declinedKey, err := declined.create(s.request)
			if err != nil {
				t.Fatal(err)
			}
			if err := declined.userDecision(declinedKey, false); err != nil {
				t.Fatal(err)
			}
			if err := declined.observe(declinedKey, raw); err == nil {
				t.Fatal("rejection accepted")
			}
			if err := declined.userDecision(declinedKey, true); err == nil {
				t.Fatal("rejected challenge reauthorized")
			}
			if err := f.userDecision(key, true); err != nil {
				t.Fatal(err)
			}
			if err := f.observe(key, nil); err == nil {
				t.Fatal("unknown result accepted")
			}
			retry, err := execution.NewRequest(s.intent, s.approval, s.result, s.now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			retried, err := f.create(retry)
			if err != nil || retried != key || retry.ExecutionID() != s.request.ExecutionID() || f.creations != 1 {
				t.Fatalf("retry created another request: %v", err)
			}
			if err := f.observe(key, raw); err != nil {
				t.Fatal(err)
			}
			if err := f.validate(key, s.intent, []wallet.Binding{s.binding}); err != nil {
				t.Fatal(err)
			}
			if err := f.observe(key, raw); err != nil {
				t.Fatal(err)
			}
			conflicting := bytes.Clone(raw)
			conflicting[len(conflicting)-1]++
			if err := f.observe(key, conflicting); err == nil {
				t.Fatal("conflicting signature accepted")
			}
			if err := f.validate(key, s.intent, nil); err == nil {
				t.Fatal("missing binding accepted")
			}
			if err := f.validate(key, s.intent, []wallet.Binding{s.binding, s.binding}); err == nil {
				t.Fatal("ambiguous bindings accepted")
			}
			revoked, err := s.binding.Transition(wallet.BindingStatusRevoked, s.now, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := f.validate(key, s.intent, []wallet.Binding{revoked}); err == nil {
				t.Fatal("revoked binding accepted")
			}
			if (circle.Config{}).Configured() {
				t.Fatal("Circle execution enabled")
			}
			plane, err := wiring.Build(wiring.Config{}, wiring.Dependencies{Now: func() time.Time { return s.now }})
			if err != nil || plane.Adapter != nil || plane.Verifier != nil || len(plane.ProviderFeatures("5042", "MAINNET")) != 0 {
				t.Fatalf("production plane activated: %v", err)
			}
		})
	}
}

func TestLocalUCWSendEnvelopeMismatches(t *testing.T) {
	raw, address := localEnvelope(t, types.DynamicFeeTxType, nil)
	s := localSend(t, address)
	tests := []struct {
		name   string
		change func(*types.DynamicFeeTx)
	}{
		{"testnet", func(p *types.DynamicFeeTx) { p.ChainID = big.NewInt(5042002) }},
		{"other chain", func(p *types.DynamicFeeTx) { p.ChainID = big.NewInt(1) }},
		{"token", func(p *types.DynamicFeeTx) { a := common.HexToAddress(contracts.AddressEURCMainnet); p.To = &a }},
		{"creation", func(p *types.DynamicFeeTx) { p.To = nil }},
		{"value", func(p *types.DynamicFeeTx) { p.Value = big.NewInt(1) }},
		{"selector", func(p *types.DynamicFeeTx) { p.Data[0]++ }},
		{"recipient", func(p *types.DynamicFeeTx) { p.Data[35]++ }},
		{"amount", func(p *types.DynamicFeeTx) { p.Data[67]++ }},
		{"padding", func(p *types.DynamicFeeTx) { p.Data[4] = 1 }},
		{"trailing calldata", func(p *types.DynamicFeeTx) { p.Data = append(p.Data, 0) }},
		{"short calldata", func(p *types.DynamicFeeTx) { p.Data = p.Data[:67] }},
		{"invalid signature", func(p *types.DynamicFeeTx) { p.R = new(big.Int) }},
		{"different signer", func(p *types.DynamicFeeTx) { p.S = big.NewInt(2) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad, _ := localEnvelope(t, types.DynamicFeeTxType, tt.change)
			if err := send.ValidateSignedTransaction(s.intent, s.binding, bad); err == nil {
				t.Fatal("mismatch accepted")
			}
		})
	}
	for _, bad := range [][]byte{nil, {0xff}, raw[:len(raw)-1], append(bytes.Clone(raw), 0), make([]byte, 4097)} {
		if err := send.ValidateSignedTransaction(s.intent, s.binding, bad); err == nil {
			t.Fatal("malformed bytes accepted")
		}
	}
}

func TestLocalUCWSendAuthorizationBindingsFailClosed(t *testing.T) {
	_, address := localEnvelope(t, types.DynamicFeeTxType, nil)
	s := localSend(t, address)
	bad := s.result
	bad.IntentDigest = "wrong"
	if _, err := execution.NewRequest(s.intent, s.approval, bad, s.now); err == nil {
		t.Fatal("policy digest mismatch accepted")
	}
	bad = s.result
	bad.Decision = policies.DecisionDeny
	if _, err := execution.NewRequest(s.intent, s.approval, bad, s.now); err == nil {
		t.Fatal("policy denial accepted")
	}
	disabled, err := s.policy.Transition(policies.StatusDisabled, s.now)
	if err != nil {
		t.Fatal(err)
	}
	denied, err := policies.Evaluate(disabled, s.intent, s.identity, s.binding, s.now)
	if err != nil || denied.Decision != policies.DecisionDeny {
		t.Fatalf("disabled policy: %v", err)
	}
	if _, err := execution.NewRequest(s.intent, s.approval, denied, s.now); err == nil {
		t.Fatal("revoked policy accepted")
	}
	if _, err := execution.NewRequest(s.intent, s.approval, s.result, s.now.Add(time.Hour)); err == nil {
		t.Fatal("expired intent accepted")
	}
}

func TestLocalUCWSendFrozenFieldsAndBindingVersions(t *testing.T) {
	raw, address := localEnvelope(t, types.DynamicFeeTxType, nil)
	s := localSend(t, address)
	if err := s.explicitApproval.EnsureAuthorizes(s.intent, s.now); err != nil {
		t.Fatal(err)
	}
	bindingParams := wallet.BindingParams{BindingID: s.binding.BindingID(), Version: s.binding.Version(), UserID: s.binding.OwnerUserID(), Provider: s.binding.Provider(), ProviderUserReference: s.binding.ProviderUserReference(), WalletID: s.binding.WalletID(), Address: s.binding.Address(), ChainID: s.binding.ChainID(), Network: s.binding.Network(), Status: s.binding.Status(), VerificationReference: s.binding.VerificationReference(), CreatedAt: s.binding.CreatedAt(), VerifiedAt: s.binding.VerifiedAt()}
	for _, tt := range []struct {
		name   string
		change func(*wallet.BindingParams)
	}{
		{"binding ID", func(p *wallet.BindingParams) { p.BindingID = "replacement" }},
		{"binding version", func(p *wallet.BindingParams) { p.Version++ }},
		{"owner", func(p *wallet.BindingParams) { p.UserID = "other-user" }},
		{"provider", func(p *wallet.BindingParams) { p.Provider = "other-provider" }},
		{"provider user", func(p *wallet.BindingParams) { p.ProviderUserReference = "other-circle-user" }},
		{"wallet ID", func(p *wallet.BindingParams) { p.WalletID = "other-wallet" }},
		{"address", func(p *wallet.BindingParams) { p.Address = "0x3333333333333333333333333333333333333333" }},
		{"chain", func(p *wallet.BindingParams) { p.ChainID = "5042002" }},
		{"network", func(p *wallet.BindingParams) { p.Network = "TESTNET" }},
		{"unverified", func(p *wallet.BindingParams) {
			p.Status = wallet.BindingStatusPending
			p.VerifiedAt = time.Time{}
			p.VerificationReference = ""
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := bindingParams
			tt.change(&p)
			b, err := wallet.NewBinding(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := send.ValidateSignedTransaction(s.intent, b, raw); err == nil {
				t.Fatal("binding substitution accepted")
			}
			if _, err := policies.Evaluate(s.policy, s.intent, s.identity, b, s.now); err == nil {
				t.Fatal("policy accepted binding substitution")
			}
		})
	}
	params := func() intents.Params {
		return intents.Params{IntentID: s.intent.IntentID(), Version: s.intent.Version(), ClientRequestID: s.intent.ClientRequestID(), Nonce: s.intent.Nonce(), Type: s.intent.Type(), Ownership: s.intent.Ownership(), Financial: s.intent.Financial(), Route: s.intent.Route(), Constraints: s.intent.Constraints(), CreatedAt: s.intent.CreatedAt(), ExpiresAt: s.intent.ExpiresAt()}
	}
	for _, tt := range []struct {
		name   string
		change func(*intents.Params)
	}{
		{"user", func(p *intents.Params) { p.Ownership.UserID = "other-user" }},
		{"binding", func(p *intents.Params) { p.Ownership.WalletBindingID = "other-binding" }},
		{"version", func(p *intents.Params) { p.Ownership.WalletBindingVersion++ }},
		{"wallet", func(p *intents.Params) { p.Ownership.WalletID = "other-wallet" }},
		{"wallet address", func(p *intents.Params) { p.Ownership.WalletAddress = "0x3333333333333333333333333333333333333333" }},
		{"chain ID", func(p *intents.Params) { p.Ownership.ChainID = "5042002"; p.Financial.Send.Token.ChainID = "5042002" }},
		{"canonical token", func(p *intents.Params) {
			token, _ := contracts.CanonicalToken("EURC")
			p.Financial.Send.Token.Address = token.Address
			p.Financial.Send.Token.Symbol = token.Symbol
		}},
		{"recipient", func(p *intents.Params) { p.Financial.Send.Recipient = "0x3333333333333333333333333333333333333333" }},
		{"amount", func(p *intents.Params) {
			p.Financial.Send.Amount.Decimal = "2"
			p.Financial.Send.Amount.BaseUnits = "2000000"
		}},
	} {
		t.Run("frozen "+tt.name, func(t *testing.T) {
			p := params()
			tt.change(&p)
			if _, err := intents.Restore(p, s.intent.Status(), s.intent.Digest(), s.intent.LifecycleRevision()); err == nil {
				t.Fatal("mutated frozen fields retained digest")
			}
			// Even a newly frozen, otherwise valid replacement cannot reuse approval.
			replacement, err := intents.NewDraft(p)
			if err != nil {
				if tt.name != "chain ID" {
					t.Fatal(err)
				}
				return // Non-Mainnet is rejected at construction.
			}
			replacement, err = replacement.Transition(intents.StatusCreated, s.now)
			if err != nil {
				t.Fatal(err)
			}
			if replacement.Digest() == s.intent.Digest() {
				t.Fatal("mutation did not change digest")
			}
			if err := s.explicitApproval.EnsureAuthorizes(replacement, s.now); err == nil {
				t.Fatal("approval transferred to replacement")
			}
			f := &localUCW{}
			key, err := f.create(s.request)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.userDecision(key, true); err != nil {
				t.Fatal(err)
			}
			if err := f.observe(key, raw); err != nil {
				t.Fatal(err)
			}
			if err := f.validate(key, replacement, []wallet.Binding{s.binding}); err == nil {
				t.Fatal("challenge transferred to replacement")
			}
		})
	}
	if _, err := send.NewPlanner().Plan(intents.Intent{}); err == nil {
		t.Fatal("unapproved empty intent accepted")
	}
}
