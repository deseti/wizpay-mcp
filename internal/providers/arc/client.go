package arc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/deseti/wizpay-mcp/internal/providers/circuit"
)

const maxResponseBytes = 1 << 20

// rpcRequest is a single JSON-RPC 2.0 call. Only the methods this package
// defines may be issued: there is no exported passthrough, so no caller can
// reach an arbitrary RPC method or contract through this client.
type rpcRequest struct {
	Version string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

// receiptPayload is the subset of an EVM transaction receipt needed to verify
// an execution. Logs and other payload fields are deliberately not decoded.
type receiptPayload struct {
	Status          string `json:"status"`
	TransactionHash string `json:"transactionHash"`
	BlockNumber     string `json:"blockNumber"`
	BlockHash       string `json:"blockHash"`
}

// Client is the narrow read-only Arc JSON-RPC client.
type Client struct {
	config  Config
	http    *http.Client
	breaker *circuit.Breaker

	// chainVerified guards the one-time chain identity check. Reading a receipt
	// from the wrong chain would be false verification evidence, so identity is
	// confirmed before any receipt is trusted.
	mu            sync.Mutex
	chainVerified bool
}

func NewClient(config Config, httpClient *http.Client) (*Client, error) {
	return NewClientWithBreaker(config, httpClient, nil)
}

// NewClientWithBreaker constructs a client that records infrastructure failures
// on the provided breaker. A nil breaker disables breaker integration.
func NewClientWithBreaker(config Config, httpClient *http.Client, breaker *circuit.Breaker) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, fmt.Errorf("Arc chain adapter is not enabled")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.Timeout}
	}
	return &Client{config: config, http: httpClient, breaker: breaker}, nil
}

// ensureChainIdentity confirms the endpoint really serves the configured chain.
// It runs once per client and is required before any receipt is trusted.
func (c *Client) ensureChainIdentity(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.chainVerified {
		return nil
	}
	var encoded string
	if err := c.call(ctx, "eth_chainId", nil, &encoded); err != nil {
		return err
	}
	observed, err := parseQuantity(encoded)
	if err != nil {
		return err
	}
	expected, err := strconv.ParseUint(c.config.ChainID, 10, 64)
	if err != nil {
		return fmt.Errorf("configured Arc chain ID is invalid")
	}
	if observed != expected {
		return fmt.Errorf("Arc endpoint serves chain %d but %d is configured", observed, expected)
	}
	c.chainVerified = true
	return nil
}

// BlockNumber returns the current chain head.
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	var encoded string
	if err := c.call(ctx, "eth_blockNumber", nil, &encoded); err != nil {
		return 0, err
	}
	return parseQuantity(encoded)
}

// Receipt reads a transaction receipt. A missing receipt is reported as not
// found rather than as an error, because an unmined transaction is a normal
// pending state and must never be mistaken for failure.
func (c *Client) Receipt(ctx context.Context, transactionHash string) (receiptPayload, bool, error) {
	if err := c.ensureChainIdentity(ctx); err != nil {
		return receiptPayload{}, false, err
	}
	var raw json.RawMessage
	if err := c.call(ctx, "eth_getTransactionReceipt", []any{transactionHash}, &raw); err != nil {
		return receiptPayload{}, false, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return receiptPayload{}, false, nil
	}
	var payload receiptPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return receiptPayload{}, false, fmt.Errorf("Arc receipt is unreadable")
	}
	return payload, true, nil
}

// call performs one JSON-RPC request. Errors carry no response body.
//
// Only the three methods this package issues may reach this function. When a
// circuit breaker is configured, infrastructure failures open the breaker;
// successful RPC responses close it.
func (c *Client) call(ctx context.Context, method string, params []any, out any) error {
	if c.breaker != nil {
		if err := c.breaker.Allow(); err != nil {
			return fmt.Errorf("Arc endpoint circuit breaker is open")
		}
	}
	if params == nil {
		params = []any{}
	}
	body, err := json.Marshal(rpcRequest{Version: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		// Encoding failure is a local validation problem, not provider outage.
		return fmt.Errorf("Arc request could not be encoded")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.RPCURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("Arc request could not be built")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		c.recordFailure()
		return fmt.Errorf("Arc endpoint is unreachable")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		c.recordFailure()
		return fmt.Errorf("Arc endpoint returned status %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		c.recordFailure()
		return fmt.Errorf("Arc response could not be read")
	}
	var decoded rpcResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		c.recordFailure()
		return fmt.Errorf("Arc response could not be decoded")
	}
	if decoded.Error != nil {
		c.recordFailure()
		return fmt.Errorf("Arc endpoint reported RPC error %d", decoded.Error.Code)
	}
	if err := json.Unmarshal(decoded.Result, out); err != nil {
		c.recordFailure()
		return fmt.Errorf("Arc result could not be decoded")
	}
	c.recordSuccess()
	return nil
}

func (c *Client) recordSuccess() {
	if c.breaker != nil {
		c.breaker.RecordSuccess()
	}
}

func (c *Client) recordFailure() {
	if c.breaker != nil {
		c.breaker.RecordFailure()
	}
}

// HealthCheck probes Arc RPC reachability and chain identity. It uses only the
// allowlisted eth_chainId method (and optionally eth_blockNumber). No secrets
// are returned.
func (c *Client) HealthCheck(ctx context.Context) (chainID string, blockNumber uint64, err error) {
	if err := c.ensureChainIdentity(ctx); err != nil {
		return "", 0, err
	}
	head, err := c.BlockNumber(ctx)
	if err != nil {
		return c.config.ChainID, 0, err
	}
	return c.config.ChainID, head, nil
}

// parseQuantity decodes an EVM hex quantity.
func parseQuantity(value string) (uint64, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "0x")
	if trimmed == "" || len(trimmed) > 16 {
		return 0, fmt.Errorf("Arc quantity %q is invalid", value)
	}
	parsed, err := strconv.ParseUint(trimmed, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("Arc quantity is invalid")
	}
	return parsed, nil
}
