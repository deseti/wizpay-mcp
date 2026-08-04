package arc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/providers"
)

func TestArcHealthNotConfigured(t *testing.T) {
	checker, err := NewHealthChecker(Config{}, nil, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	health := checker.Check(context.Background())
	if health.Status != providers.HealthNotConfigured {
		t.Fatalf("status = %s", health.Status)
	}
}

func TestArcHealthProbeHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "eth_chainId":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x4cef52"}`)) // 5042002
		case "eth_blockNumber":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x10"}`))
		default:
			t.Fatalf("unexpected method %s", req.Method)
		}
	}))
	defer server.Close()

	config := Config{
		Enabled: true, ChainID: ChainIDTestnet, Network: NetworkTestnet,
		RPCURL: server.URL, ExplorerURL: ExplorerTestnet, MinConfirmations: 1, Timeout: 2 * time.Second,
	}
	// httptest uses http URL; config validation requires https. Bypass by
	// constructing client without Validate path for test: use NewClient only
	// after patching - actually Validate requires https. Use a custom checker
	// with a direct client for unit test of Check mapping instead.
	//
	// Build health checker via internal client with forced config by skipping
	// NewHealthChecker validation: set Enabled and use NewClientWithBreaker only
	// when URL is https. For httptest, test HealthCheck on Client through a
	// verifier-style fake by testing Check with a stub client path.
	//
	// Simplest path: temporarily allow test server by constructing HealthChecker
	// manually after creating client that doesn't re-validate... NewClient
	// validates. Change approach: unit-test only NOT_CONFIGURED here and
	// integration test for live; for fake, inject by using https via
	// httptest which is http only.
	//
	// Use a transport that rewrites to the test server while Config has https URL.
	httpClient := &http.Client{Transport: &rewriteTransport{target: server.URL}, Timeout: 2 * time.Second}
	config.RPCURL = "https://rpc.testnet.arc.io"
	client, err := NewClientWithBreaker(config, httpClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	chainID, blockNumber, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if chainID != ChainIDTestnet || blockNumber == 0 {
		t.Fatalf("chain=%s block=%d", chainID, blockNumber)
	}
	checker := &HealthChecker{config: config, client: client, now: time.Now}
	health := checker.Check(context.Background())
	if health.Status != providers.HealthHealthy {
		t.Fatalf("status = %s detail=%q", health.Status, health.Detail)
	}
}

type rewriteTransport struct {
	target string
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(r.target)
	if err != nil {
		return nil, err
	}
	// Build a fresh request against the test server so the POST body is preserved.
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
	}
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	if req.URL.RawQuery != "" {
		path += "?" + req.URL.RawQuery
	}
	endpoint := target.Scheme + "://" + target.Host + path
	out, err := http.NewRequestWithContext(req.Context(), req.Method, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	out.Header = req.Header.Clone()
	return http.DefaultTransport.RoundTrip(out)
}
