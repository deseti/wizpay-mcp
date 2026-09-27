package arc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

func TestAttestDeploymentCodeIsCanonicalAndReadOnly(t *testing.T) {
	var codeAddress string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "eth_chainId":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x13b2"}`))
		case "eth_getCode":
			if len(request.Params) != 2 || request.Params[1] != "latest" {
				t.Fatalf("unexpected getCode params: %#v", request.Params)
			}
			codeAddress, _ = request.Params[0].(string)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x6001600055"}`))
		default:
			t.Fatalf("unexpected RPC method %q", request.Method)
		}
	}))
	defer server.Close()

	config := Config{Enabled: true, ChainID: ChainIDMainnet, Network: NetworkMainnet, RPCURL: RPCMainnet, ExplorerURL: ExplorerMainnet, MinConfirmations: 1, Timeout: 2 * time.Second}
	client, err := NewClient(config, &http.Client{Transport: &rewriteTransport{target: server.URL}, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := client.AttestDeploymentCode(context.Background(), contracts.ContractWizPayPayroll, contracts.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !contracts.AddressesEqual(codeAddress, contracts.AddressWizPayPayroll) || attestation.CodeSize != 5 || attestation.CodeHash == "" {
		t.Fatalf("unexpected attestation: %#v address=%s", attestation, codeAddress)
	}
}

// canonicalOwner is the owner/feeRecipient address used in state attestation
// tests. feeRecipient must equal owner per the corrective patch.
const canonicalOwner = "0x1111111111111111111111111111111111111111"

// canonicalStateResponses returns a fresh response map whose owner and
// feeRecipient are identical (required by ValidateCanonicalResources) and whose
// feeBps is within the contract MAX_FEE_BPS of 100.
func canonicalStateResponses() map[string]string {
	return map[string]string{
		selectorHex("owner()"):           addressWord(canonicalOwner),
		selectorHex("feeRecipient()"):    addressWord(canonicalOwner),
		selectorHex("USDC()"):            addressWord(contracts.AddressUSDCMainnet),
		selectorHex("EURC()"):            addressWord(contracts.AddressEURCMainnet),
		selectorHex("poolManager()"):     addressWord(contracts.AddressUniswapV4PoolManager),
		selectorHex("universalRouter()"): addressWord(contracts.AddressUniswapUniversalRouter),
		selectorHex("permit2()"):         addressWord(contracts.AddressPermit2),
		selectorHex("feeBps()"):          uintWord(25),
		selectorHex("paused()"):          uintWord(0),
		selectorHex("poolFee()"):         uintWord(500),
		selectorHex("poolTickSpacing()"): uintWord(10),
	}
}

func newStateTestServer(t *testing.T, responses map[string]string, callCount *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if request.Method == "eth_chainId" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x13b2"}`))
			return
		}
		if request.Method != "eth_call" || len(request.Params) != 2 {
			t.Errorf("unexpected RPC request: method=%s params=%d", request.Method, len(request.Params))
			return
		}
		var call map[string]string
		if err := json.Unmarshal(request.Params[0], &call); err != nil {
			t.Error(err)
			return
		}
		if !contracts.AddressesEqual(call["to"], contracts.AddressWizPayPayroll) {
			t.Errorf("non-canonical target %q", call["to"])
			return
		}
		result, found := responses[call["data"]]
		if !found {
			t.Errorf("unallowlisted selector %q", call["data"])
			return
		}
		if callCount != nil {
			*callCount++
		}
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":"%s"}`, result)
	}))
}

func TestAttestDeploymentStateUsesOnlyCanonicalGetters(t *testing.T) {
	responses := canonicalStateResponses()
	var calls int
	server := newStateTestServer(t, responses, &calls)
	defer server.Close()

	config := Config{Enabled: true, ChainID: ChainIDMainnet, Network: NetworkMainnet, RPCURL: RPCMainnet, ExplorerURL: ExplorerMainnet, MinConfirmations: 1, Timeout: 2 * time.Second}
	client, err := NewClient(config, &http.Client{Transport: &rewriteTransport{target: server.URL}, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := client.AttestDeploymentState(context.Background(), contracts.ContractWizPayPayroll, contracts.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if calls != len(responses) || attestation.FeeBPS != 25 || attestation.Paused || attestation.PoolFee != 500 || attestation.PoolTickSpacing != 10 {
		t.Fatalf("calls=%d attestation=%#v", calls, attestation)
	}
	if err := attestation.ValidateCanonicalResources(); err != nil {
		t.Fatal(err)
	}
	// Mutating a canonical resource must fail closed.
	mutated := attestation
	mutated.USDC = "0x3333333333333333333333333333333333333333"
	if err := mutated.ValidateCanonicalResources(); err == nil {
		t.Fatal("mismatched canonical USDC resource must fail closed")
	}
}

// TestValidateCanonicalResourcesRejectsFeeRecipientMismatch verifies that an
// attestation where feeRecipient != owner is rejected, regardless of whether
// both addresses are individually valid.
func TestValidateCanonicalResourcesRejectsFeeRecipientMismatch(t *testing.T) {
	responses := canonicalStateResponses()
	// Overwrite feeRecipient with a different valid address.
	responses[selectorHex("feeRecipient()")] = addressWord("0x2222222222222222222222222222222222222222")
	server := newStateTestServer(t, responses, nil)
	defer server.Close()

	config := Config{Enabled: true, ChainID: ChainIDMainnet, Network: NetworkMainnet, RPCURL: RPCMainnet, ExplorerURL: ExplorerMainnet, MinConfirmations: 1, Timeout: 2 * time.Second}
	client, err := NewClient(config, &http.Client{Transport: &rewriteTransport{target: server.URL}, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := client.AttestDeploymentState(context.Background(), contracts.ContractWizPayPayroll, contracts.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := attestation.ValidateCanonicalResources(); err == nil {
		t.Fatal("feeRecipient != owner must fail ValidateCanonicalResources")
	}
}

// TestValidateCanonicalResourcesRejectsExcessiveFeeBPS verifies that a feeBps
// value above the contract MAX_FEE_BPS (100) is rejected.
func TestValidateCanonicalResourcesRejectsExcessiveFeeBPS(t *testing.T) {
	responses := canonicalStateResponses()
	// Set feeBps to 101, which is one above the contract MAX_FEE_BPS.
	responses[selectorHex("feeBps()")] = uintWord(101)
	server := newStateTestServer(t, responses, nil)
	defer server.Close()

	config := Config{Enabled: true, ChainID: ChainIDMainnet, Network: NetworkMainnet, RPCURL: RPCMainnet, ExplorerURL: ExplorerMainnet, MinConfirmations: 1, Timeout: 2 * time.Second}
	client, err := NewClient(config, &http.Client{Transport: &rewriteTransport{target: server.URL}, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := client.AttestDeploymentState(context.Background(), contracts.ContractWizPayPayroll, contracts.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := attestation.ValidateCanonicalResources(); err == nil {
		t.Fatal("feeBps=101 must exceed contract MAX_FEE_BPS and fail ValidateCanonicalResources")
	}
}

// TestValidateCanonicalResourcesAcceptsMaxFeeBPS verifies that a feeBps value
// exactly equal to the contract MAX_FEE_BPS (100) is accepted.
func TestValidateCanonicalResourcesAcceptsMaxFeeBPS(t *testing.T) {
	responses := canonicalStateResponses()
	responses[selectorHex("feeBps()")] = uintWord(contracts.ContractMaxFeeBPS)
	server := newStateTestServer(t, responses, nil)
	defer server.Close()

	config := Config{Enabled: true, ChainID: ChainIDMainnet, Network: NetworkMainnet, RPCURL: RPCMainnet, ExplorerURL: ExplorerMainnet, MinConfirmations: 1, Timeout: 2 * time.Second}
	client, err := NewClient(config, &http.Client{Transport: &rewriteTransport{target: server.URL}, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := client.AttestDeploymentState(context.Background(), contracts.ContractWizPayPayroll, contracts.RegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := attestation.ValidateCanonicalResources(); err != nil {
		t.Fatalf("feeBps=100 (MAX_FEE_BPS) must be accepted: %v", err)
	}
}

func selectorHex(signature string) string {
	selector := contracts.Selector4(signature)
	return fmt.Sprintf("0x%x", selector)
}

func addressWord(address string) string {
	word := common.LeftPadBytes(common.HexToAddress(address).Bytes(), 32)
	return "0x" + hex.EncodeToString(word)
}

func uintWord(value uint64) string {
	word := make([]byte, 32)
	for i := 0; i < 8; i++ {
		word[31-i] = byte(value >> (8 * i))
	}
	return "0x" + hex.EncodeToString(word)
}

func TestAttestDeploymentCodeRejectsUnsupportedResourceBeforeRPC(t *testing.T) {
	config := Config{Enabled: true, ChainID: ChainIDMainnet, Network: NetworkMainnet, RPCURL: RPCMainnet, ExplorerURL: ExplorerMainnet, MinConfirmations: 1, Timeout: 2 * time.Second}
	client, err := NewClient(config, &http.Client{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.AttestDeploymentCode(context.Background(), contracts.ContractID("ARBITRARY"), contracts.RegistryVersion); err == nil {
		t.Fatal("unsupported contract ID must fail closed")
	}
	if _, err := client.AttestDeploymentCode(context.Background(), contracts.ContractWizPayPayroll, 999); err == nil {
		t.Fatal("unsupported registry version must fail closed")
	}
}
