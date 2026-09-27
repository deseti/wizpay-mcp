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

func TestAttestDeploymentStateUsesOnlyCanonicalGetters(t *testing.T) {
	responses := map[string]string{
		selectorHex("owner()"):           addressWord("0x1111111111111111111111111111111111111111"),
		selectorHex("feeRecipient()"):    addressWord("0x2222222222222222222222222222222222222222"),
		selectorHex("USDC()"):            addressWord(contracts.AddressUSDCMainnet),
		selectorHex("EURC()"):            addressWord(contracts.AddressEURCMainnet),
		selectorHex("poolManager()"):     addressWord(contracts.AddressUniswapV4PoolManager),
		selectorHex("universalRouter()"): addressWord(contracts.AddressUniswapUniversalRouter),
		selectorHex("permit2()"):         addressWord(contracts.AddressPermit2),
		selectorHex("feeBps()"):          uintWord(20),
		selectorHex("paused()"):          uintWord(0),
		selectorHex("poolFee()"):         uintWord(500),
		selectorHex("poolTickSpacing()"): uintWord(10),
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		calls++
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":"%s"}`, result)
	}))
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
	if calls != len(responses) || attestation.FeeBPS != 20 || attestation.Paused || attestation.PoolFee != 500 || attestation.PoolTickSpacing != 10 {
		t.Fatalf("calls=%d attestation=%#v", calls, attestation)
	}
	if err := attestation.ValidateCanonicalResources(); err != nil {
		t.Fatal(err)
	}
	attestation.USDC = "0x3333333333333333333333333333333333333333"
	if err := attestation.ValidateCanonicalResources(); err == nil {
		t.Fatal("mismatched canonical resource must fail closed")
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
