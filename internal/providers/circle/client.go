package circle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/providers"
	"github.com/deseti/wizpay-mcp/internal/providers/circuit"
)

// maxResponseBytes bounds how much of a provider response is read into memory.
const maxResponseBytes = 1 << 20

// transportError carries a classified transport or provider failure. It holds a
// safe reason code only: no response body, no headers, no credentials.
type transportError struct {
	class      providers.Classification
	reasonCode string
}

func (e *transportError) Error() string { return "circle provider request failed" }

func (e *transportError) classification() (providers.Classification, string) {
	return e.class, e.reasonCode
}

// client is the narrow Circle HTTP boundary. It exposes only the three
// documented operations this phase needs and no general-purpose request method.
type client struct {
	config  Config
	http    *http.Client
	breaker *circuit.Breaker
}

func newClient(config Config, httpClient *http.Client) (*client, error) {
	return newClientWithBreaker(config, httpClient, nil)
}

func newClientWithBreaker(config Config, httpClient *http.Client, breaker *circuit.Breaker) (*client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, fmt.Errorf("Circle provider is not enabled")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.Timeout}
	}
	return &client{config: config, http: httpClient, breaker: breaker}, nil
}

// transferRequest is the documented request body for creating a user-controlled
// transfer challenge. Only documented fields are sent.
type transferRequest struct {
	IdempotencyKey     string   `json:"idempotencyKey"`
	DestinationAddress string   `json:"destinationAddress"`
	Amounts            []string `json:"amounts"`
	WalletID           string   `json:"walletId"`
	TokenID            string   `json:"tokenId"`
	RefID              string   `json:"refId,omitempty"`
	FeeLevel           string   `json:"feeLevel,omitempty"`
}

type transferResponse struct {
	Data struct {
		ChallengeID string `json:"challengeId"`
	} `json:"data"`
}

// createTransferChallenge initiates a transfer. It returns a challenge ID,
// which is a request for the user's authorization and is NOT a submitted
// transaction and NOT evidence of success.
//
// The submitted flag returned alongside the error reports whether the request
// may have reached Circle. Callers must treat a true value as potentially
// executed and reconcile rather than retry.
func (c *client) createTransferChallenge(ctx context.Context, authorization providers.UserAuthorization, body transferRequest) (string, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", &transportError{class: providers.ClassPreSubmissionValidationFailure, reasonCode: "PROVIDER_REQUEST_INVALID"}
	}
	response, err := c.do(ctx, http.MethodPost, "/v1/w3s/user/transactions/transfer", nil, encoded, authorization, true)
	if err != nil {
		return "", err
	}
	var decoded transferResponse
	if err := json.Unmarshal(response, &decoded); err != nil || decoded.Data.ChallengeID == "" {
		// The request was accepted but the response is unusable. Submission may
		// have occurred, so this is ambiguous rather than a clean failure.
		return "", &transportError{class: providers.ClassAmbiguousSubmission, reasonCode: "PROVIDER_RESPONSE_UNREADABLE"}
	}
	return decoded.Data.ChallengeID, nil
}

// transaction is the subset of the documented Circle transaction object needed
// for reconciliation. Undocumented and sensitive fields are deliberately not
// decoded, so no raw provider payload can enter the system.
type transaction struct {
	ID          string           `json:"id"`
	State       TransactionState `json:"state"`
	TxHash      string           `json:"txHash"`
	Blockchain  string           `json:"blockchain"`
	WalletID    string           `json:"walletId"`
	RefID       string           `json:"refId"`
	Operation   string           `json:"operation"`
	CustodyType string           `json:"custodyType"`
}

type transactionResponse struct {
	Data struct {
		Transaction transaction `json:"transaction"`
	} `json:"data"`
}

type transactionListResponse struct {
	Data struct {
		Transactions []transaction `json:"transactions"`
	} `json:"data"`
}

// getTransaction reads one transaction by its Circle ID.
func (c *client) getTransaction(ctx context.Context, authorization providers.UserAuthorization, transactionID string) (transaction, error) {
	if transactionID == "" {
		return transaction{}, &transportError{class: providers.ClassAmbiguousSubmission, reasonCode: "PROVIDER_REFERENCE_MISSING"}
	}
	path := "/v1/w3s/transactions/" + url.PathEscape(transactionID)
	response, err := c.do(ctx, http.MethodGet, path, nil, nil, authorization, false)
	if err != nil {
		return transaction{}, err
	}
	var decoded transactionResponse
	if err := json.Unmarshal(response, &decoded); err != nil || decoded.Data.Transaction.ID == "" {
		return transaction{}, &transportError{class: providers.ClassAmbiguousSubmission, reasonCode: "PROVIDER_RESPONSE_UNREADABLE"}
	}
	return decoded.Data.Transaction, nil
}

// findTransactionByRef locates a transaction by the reference this system set at
// submission time. This is the reconciliation path used when a challenge was
// created but the resulting transaction ID was never observed.
//
// Circle exposes no refId query filter, so the reference is matched client-side
// over the wallet's recent transactions.
func (c *client) findTransactionByRef(ctx context.Context, authorization providers.UserAuthorization, walletID, refID string) (transaction, bool, error) {
	if walletID == "" || refID == "" {
		return transaction{}, false, &transportError{class: providers.ClassAmbiguousSubmission, reasonCode: "PROVIDER_REFERENCE_MISSING"}
	}
	query := url.Values{}
	query.Set("walletIds", walletID)
	query.Set("blockchain", string(c.config.Blockchain))
	query.Set("txType", "OUTBOUND")
	query.Set("pageSize", "50")
	query.Set("order", "DESC")
	response, err := c.do(ctx, http.MethodGet, "/v1/w3s/transactions", query, nil, authorization, false)
	if err != nil {
		return transaction{}, false, err
	}
	var decoded transactionListResponse
	if err := json.Unmarshal(response, &decoded); err != nil {
		return transaction{}, false, &transportError{class: providers.ClassAmbiguousSubmission, reasonCode: "PROVIDER_RESPONSE_UNREADABLE"}
	}
	for _, candidate := range decoded.Data.Transactions {
		if candidate.RefID == refID && candidate.WalletID == walletID {
			return candidate, true, nil
		}
	}
	return transaction{}, false, nil
}

type challenge struct {
	ID             string          `json:"id"`
	Status         ChallengeStatus `json:"status"`
	Type           string          `json:"type"`
	CorrelationIDs []string        `json:"correlationIds"`
}

type challengeListResponse struct {
	Data struct {
		Challenges []challenge `json:"challenges"`
	} `json:"data"`
}

// findChallenge locates an outstanding challenge by ID. Circle documents no
// single-challenge endpoint, so the outstanding-challenge list is used.
//
// Absence is meaningful but not conclusive: the list only returns PENDING and
// IN_PROGRESS challenges, so a completed, failed, or expired challenge is simply
// missing. Callers must not infer failure from absence.
func (c *client) findChallenge(ctx context.Context, authorization providers.UserAuthorization, challengeID string) (challenge, bool, error) {
	if challengeID == "" {
		return challenge{}, false, &transportError{class: providers.ClassAmbiguousSubmission, reasonCode: "PROVIDER_REFERENCE_MISSING"}
	}
	response, err := c.do(ctx, http.MethodGet, "/v1/w3s/user/challenges", nil, nil, authorization, false)
	if err != nil {
		return challenge{}, false, err
	}
	var decoded challengeListResponse
	if err := json.Unmarshal(response, &decoded); err != nil {
		return challenge{}, false, &transportError{class: providers.ClassAmbiguousSubmission, reasonCode: "PROVIDER_RESPONSE_UNREADABLE"}
	}
	for _, candidate := range decoded.Data.Challenges {
		if candidate.ID == challengeID {
			return candidate, true, nil
		}
	}
	return challenge{}, false, nil
}

// do performs one Circle request.
//
// The API key and the ephemeral user token are attached here and nowhere else.
// Neither value, nor any request or response body, is ever logged or returned:
// failures surface only as a classification and a safe reason code.
//
// Circuit-breaker accounting records only infrastructure/provider-service
// failures. Validation failures and missing user authorization never open the
// breaker. An open breaker never resubmits an ambiguous operation — callers
// still reconcile through GetStatus after submission may have occurred.
func (c *client) do(ctx context.Context, method, path string, query url.Values, body []byte, authorization providers.UserAuthorization, submitting bool) ([]byte, error) {
	if !authorization.Present() {
		// Authentication to WizPay is not Circle signing authority. Without the
		// user's own ephemeral token, no provider call may be attempted.
		// This is not a provider infrastructure failure.
		return nil, &transportError{class: providers.ClassUserAuthorizationRequired, reasonCode: "USER_AUTHORIZATION_REQUIRED"}
	}
	if c.breaker != nil {
		if err := c.breaker.Allow(); err != nil {
			return nil, &transportError{class: providers.ClassTransientProviderError, reasonCode: "PROVIDER_CIRCUIT_OPEN"}
		}
	}
	endpoint := strings.TrimSuffix(c.config.BaseURL, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		// Local request construction failure is validation, not provider outage.
		return nil, &transportError{class: providers.ClassPreSubmissionValidationFailure, reasonCode: "PROVIDER_REQUEST_INVALID"}
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+c.config.APIKey.reveal())
	request.Header.Set("X-User-Token", authorization.Reveal())

	response, err := c.http.Do(request)
	if err != nil {
		c.recordInfrastructureFailure()
		// A transport failure on a submission is the ambiguous case: the
		// request may have reached Circle even though no response returned.
		if submitting {
			return nil, &transportError{class: providers.ClassAmbiguousSubmission, reasonCode: "PROVIDER_SUBMISSION_UNCONFIRMED"}
		}
		return nil, &transportError{class: providers.ClassTransientProviderError, reasonCode: "PROVIDER_UNREACHABLE"}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		_ = response.Body.Close()
	}()

	payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		class, reasonCode := classifyHTTPStatus(response.StatusCode, submitting)
		if countsTowardCircuit(class, response.StatusCode) {
			c.recordInfrastructureFailure()
		}
		return nil, &transportError{class: class, reasonCode: reasonCode}
	}
	if readErr != nil {
		c.recordInfrastructureFailure()
		if submitting {
			return nil, &transportError{class: providers.ClassAmbiguousSubmission, reasonCode: "PROVIDER_RESPONSE_UNREADABLE"}
		}
		return nil, &transportError{class: providers.ClassTransientProviderError, reasonCode: "PROVIDER_RESPONSE_UNREADABLE"}
	}
	c.recordInfrastructureSuccess()
	return payload, nil
}

func (c *client) recordInfrastructureSuccess() {
	if c.breaker != nil {
		c.breaker.RecordSuccess()
	}
}

func (c *client) recordInfrastructureFailure() {
	if c.breaker != nil {
		c.breaker.RecordFailure()
	}
}

// countsTowardCircuit reports whether an HTTP status should open the breaker.
// Auth rejections against a bad key are infrastructure; permanent request
// validation rejections (400/422) are not.
func countsTowardCircuit(class providers.Classification, statusCode int) bool {
	switch class {
	case providers.ClassTransientProviderError, providers.ClassAmbiguousSubmission:
		return true
	case providers.ClassPermanentProviderRejection:
		// 401/403/5xx-like permanent auth rejections count; 400/422 body rejections do not.
		return statusCode == 401 || statusCode == 403 || statusCode >= 500
	default:
		return false
	}
}

// healthProbe performs a non-financial, API-key-only reachability probe against
// Circle. It never creates wallets, transfers tokens, or completes challenges.
// A 2xx response is healthy; 401/403 means the host is reachable but credentials
// are rejected (degraded); network/5xx failures are unavailable.
func (c *client) healthProbe(ctx context.Context) (statusCode int, err error) {
	if c.breaker != nil {
		if allowErr := c.breaker.Allow(); allowErr != nil {
			return 0, circuit.ErrOpen
		}
	}
	// /v1/ping is a conventional non-financial reachability path. Even when the
	// path is absent, a 404 proves the API host answered without performing a
	// financial action.
	endpoint := strings.TrimSuffix(c.config.BaseURL, "/") + "/v1/ping"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.config.APIKey.reveal())
	// Intentionally no X-User-Token: this probe is infrastructure-only.

	response, err := c.http.Do(request)
	if err != nil {
		c.recordInfrastructureFailure()
		return 0, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		_ = response.Body.Close()
	}()
	if response.StatusCode >= 500 {
		c.recordInfrastructureFailure()
		return response.StatusCode, fmt.Errorf("circle health probe status %d", response.StatusCode)
	}
	// Reachable host (including 401/403/404) is a successful infrastructure probe
	// from the breaker's perspective: the service answered.
	c.recordInfrastructureSuccess()
	return response.StatusCode, nil
}
