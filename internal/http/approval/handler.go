// Package approval provides the authenticated HTTP transport for approval
// artifacts. It contains no approval policy or persistence logic.
package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/services"
)

type Handler struct {
	service services.ApprovalService
}

func NewHandler(service services.ApprovalService) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("approval service is required")
	}
	return &Handler{service: service}, nil
}

// ServeHTTP handles /approval/{approvalID} and
// /approval/{approvalID}/decision and /approval/{approvalID}/authorize-execution. Authentication is intentionally supplied by
// the application server's existing middleware wrapper.
func (h *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/approvals" {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeError(response, http.StatusMethodNotAllowed, nil)
			return
		}
		h.list(response, request)
		return
	}
	approvalID, path, ok := approvalPath(request.URL.Path)
	if !ok {
		writeError(response, http.StatusNotFound, nil)
		return
	}
	if path == "decision" {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeError(response, http.StatusMethodNotAllowed, nil)
			return
		}
		h.decide(response, request, approvalID)
		return
	}
	if path == "authorize-execution" {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeError(response, http.StatusMethodNotAllowed, nil)
			return
		}
		h.authorizeExecution(response, request, approvalID)
		return
	}
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", http.MethodGet)
		writeError(response, http.StatusMethodNotAllowed, nil)
		return
	}
	h.get(response, request, approvalID)
}

type decisionInput struct {
	Decision string `json:"decision"`
}

type authorizationInput struct {
	IntentID             string `json:"intent_id"`
	WalletBindingID      string `json:"wallet_binding_id"`
	WalletBindingVersion uint64 `json:"wallet_binding_version"`
}

type approvalOutput struct {
	ApprovalID             string `json:"approval_id"`
	ApprovalVersion        uint64 `json:"approval_version"`
	IntentID               string `json:"intent_id"`
	IntentVersion          uint64 `json:"intent_version"`
	Status                 string `json:"status"`
	Decision               string `json:"decision"`
	CreatedAt              string `json:"created_at"`
	ExpiresAt              string `json:"expires_at"`
	Amount                 string `json:"amount,omitempty"`
	Token                  string `json:"token,omitempty"`
	WalletBindingReference string `json:"wallet_binding_reference,omitempty"`
	WalletBindingVersion   uint64 `json:"wallet_binding_version,omitempty"`
	WalletReference        string `json:"wallet_reference,omitempty"`
	Recipient              string `json:"recipient,omitempty"`
	AgentIdentity          string `json:"agent_identity,omitempty"`
}

type authorizationOutput struct {
	AuthorizationID      string `json:"execution_authorization_id"`
	ApprovalID           string `json:"approval_id"`
	IntentID             string `json:"intent_id"`
	WalletBindingID      string `json:"wallet_binding_reference"`
	WalletBindingVersion uint64 `json:"wallet_binding_version"`
	Status               string `json:"status"`
	Amount               string `json:"amount,omitempty"`
	Token                string `json:"token,omitempty"`
	WalletReference      string `json:"wallet_reference,omitempty"`
	Recipient            string `json:"recipient,omitempty"`
	AgentIdentity        string `json:"agent_identity,omitempty"`
}

type errorOutput struct {
	Error apperrors.PublicError `json:"error"`
}

type approvalListOutput struct {
	ApprovalID    string `json:"approval_id"`
	Status        string `json:"status"`
	Decision      string `json:"decision"`
	IntentID      string `json:"intent_id"`
	IntentVersion uint64 `json:"intent_version"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
}

type approvalListResponse struct {
	Approvals  []approvalListOutput `json:"approvals"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

func (h *Handler) list(response http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(response, http.StatusBadRequest, apperrors.New(apperrors.CodeValidationError, "Approval list limit is invalid.", false, true, true))
			return
		}
		limit = parsed
	}
	page, err := h.service.ListApprovals(request.Context(), limit, query.Get("cursor"), query.Get("status"))
	if err != nil {
		writeError(response, statusFor(err), err)
		return
	}
	result := approvalListResponse{Approvals: make([]approvalListOutput, 0, len(page.Approvals)), NextCursor: page.NextCursor}
	for _, value := range page.Approvals {
		result.Approvals = append(result.Approvals, approvalListOutput{ApprovalID: value.ApprovalID(), Status: string(value.Status()), Decision: string(value.Decision()), IntentID: value.IntentID(), IntentVersion: value.IntentVersion(), CreatedAt: value.CreatedAt().UTC().Format(time.RFC3339Nano), ExpiresAt: value.ExpiresAt().UTC().Format(time.RFC3339Nano)})
	}
	writeJSON(response, http.StatusOK, result)
}

func (h *Handler) get(response http.ResponseWriter, request *http.Request, approvalID string) {
	value, err := h.service.GetApproval(request.Context(), approvalID)
	if err != nil {
		writeError(response, statusFor(err), err)
		return
	}
	output := approvalOutputOf(value)
	if detailService, ok := h.service.(interface {
		GetExecutionConfirmation(context.Context, string) (services.ExecutionAuthorization, error)
	}); ok {
		if confirmation, detailErr := detailService.GetExecutionConfirmation(request.Context(), approvalID); detailErr == nil {
			output = output.withConfirmation(confirmation)
		}
	}
	writeJSON(response, http.StatusOK, output)
}

func (h *Handler) decide(response http.ResponseWriter, request *http.Request, approvalID string) {
	var input decisionInput
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(response, http.StatusBadRequest, apperrors.New(apperrors.CodeValidationError, "Request body is invalid.", false, true, true))
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(response, http.StatusBadRequest, apperrors.New(apperrors.CodeValidationError, "Request body is invalid.", false, true, true))
		return
	}
	if input.Decision != string(approvals.DecisionApproved) && input.Decision != string(approvals.DecisionRejected) {
		writeError(response, http.StatusBadRequest, apperrors.New(apperrors.CodeValidationError, "Decision must be APPROVED or REJECTED.", false, true, true))
		return
	}
	value, err := h.service.DecideApproval(request.Context(), approvalID, approvals.Decision(input.Decision))
	if err != nil {
		writeError(response, statusFor(err), err)
		return
	}
	writeJSON(response, http.StatusOK, approvalOutputOf(value))
}

func (h *Handler) authorizeExecution(response http.ResponseWriter, request *http.Request, approvalID string) {
	var input authorizationInput
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.IntentID == "" || input.WalletBindingID == "" || input.WalletBindingVersion == 0 {
		writeError(response, http.StatusBadRequest, apperrors.New(apperrors.CodeValidationError, "Authorization request is invalid.", false, true, true))
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(response, http.StatusBadRequest, apperrors.New(apperrors.CodeValidationError, "Authorization request is invalid.", false, true, true))
		return
	}
	value, err := h.service.AuthorizeExecution(request.Context(), approvalID, input.IntentID, input.WalletBindingID, input.WalletBindingVersion)
	if err != nil {
		writeError(response, statusFor(err), err)
		return
	}
	writeJSON(response, http.StatusOK, authorizationOutputOf(value))
}

func approvalPath(path string) (string, string, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 2 && parts[0] == "approval" && parts[1] != "" {
		return parts[1], "", true
	}
	if len(parts) == 3 && parts[0] == "approval" && parts[1] != "" && parts[2] == "decision" {
		return parts[1], "decision", true
	}
	if len(parts) == 3 && parts[0] == "approval" && parts[1] != "" && parts[2] == "authorize-execution" {
		return parts[1], "authorize-execution", true
	}
	return "", "", false
}

func authorizationOutputOf(value services.ExecutionAuthorization) authorizationOutput {
	return authorizationOutput{AuthorizationID: value.AuthorizationID, ApprovalID: value.ApprovalID, IntentID: value.IntentID, WalletBindingID: value.WalletBindingID, WalletBindingVersion: value.WalletBindingVersion, Status: string(value.Status), Amount: value.Amount, Token: value.Token, WalletReference: value.WalletReference, Recipient: value.Recipient, AgentIdentity: value.AgentIdentity}
}

func approvalOutputOf(value approvals.Approval) approvalOutput {
	return approvalOutput{ApprovalID: value.ApprovalID(), ApprovalVersion: value.Version(), IntentID: value.IntentID(), IntentVersion: value.IntentVersion(), Status: string(value.Status()), Decision: string(value.Decision()), CreatedAt: value.CreatedAt().UTC().Format(time.RFC3339Nano), ExpiresAt: value.ExpiresAt().UTC().Format(time.RFC3339Nano)}
}

func (o approvalOutput) withConfirmation(value services.ExecutionAuthorization) approvalOutput {
	o.Amount, o.Token, o.WalletBindingReference, o.WalletBindingVersion, o.WalletReference, o.Recipient, o.AgentIdentity = value.Amount, value.Token, value.WalletBindingID, value.WalletBindingVersion, value.WalletReference, value.Recipient, value.AgentIdentity
	return o
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func writeError(response http.ResponseWriter, status int, err error) {
	if err == nil {
		writeJSON(response, status, errorOutput{Error: apperrors.PublicError{Code: apperrors.CodeInternalError, Message: "An internal error occurred.", Terminal: false}})
		return
	}
	writeJSON(response, status, errorOutput{Error: apperrors.ToPublic(err)})
}

func statusFor(err error) int {
	public := apperrors.ToPublic(err)
	switch public.Code {
	case apperrors.CodeAuthenticationRequired:
		return http.StatusUnauthorized
	case apperrors.CodeAuthorizationRequired, apperrors.CodeIdentitySuspended, apperrors.CodeIdentityRevoked:
		return http.StatusForbidden
	case apperrors.CodeApprovalRequired, apperrors.CodeApprovalExpired, apperrors.CodeApprovalRejected, apperrors.CodeWalletMismatch, apperrors.CodeWalletNotBound, apperrors.CodeWalletRevoked:
		return http.StatusBadRequest
	case apperrors.CodeApprovalNotFound:
		return http.StatusNotFound
	case apperrors.CodeValidationError:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
