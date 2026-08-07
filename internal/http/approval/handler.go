// Package approval provides the authenticated HTTP transport for approval
// artifacts. It contains no approval policy or persistence logic.
package approval

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
// /approval/{approvalID}/decision. Authentication is intentionally supplied by
// the application server's existing middleware wrapper.
func (h *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	approvalID, decisionPath, ok := approvalPath(request.URL.Path)
	if !ok {
		writeError(response, http.StatusNotFound, nil)
		return
	}
	if decisionPath {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeError(response, http.StatusMethodNotAllowed, nil)
			return
		}
		h.decide(response, request, approvalID)
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

type approvalOutput struct {
	ApprovalID      string `json:"approval_id"`
	ApprovalVersion uint64 `json:"approval_version"`
	IntentID        string `json:"intent_id"`
	IntentVersion   uint64 `json:"intent_version"`
	Status          string `json:"status"`
	Decision        string `json:"decision"`
	CreatedAt       string `json:"created_at"`
	ExpiresAt       string `json:"expires_at"`
}

type errorOutput struct {
	Error apperrors.PublicError `json:"error"`
}

func (h *Handler) get(response http.ResponseWriter, request *http.Request, approvalID string) {
	value, err := h.service.GetApproval(request.Context(), approvalID)
	if err != nil {
		writeError(response, statusFor(err), err)
		return
	}
	writeJSON(response, http.StatusOK, approvalOutputOf(value))
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

func approvalPath(path string) (string, bool, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 2 && parts[0] == "approval" && parts[1] != "" {
		return parts[1], false, true
	}
	if len(parts) == 3 && parts[0] == "approval" && parts[1] != "" && parts[2] == "decision" {
		return parts[1], true, true
	}
	return "", false, false
}

func approvalOutputOf(value approvals.Approval) approvalOutput {
	return approvalOutput{ApprovalID: value.ApprovalID(), ApprovalVersion: value.Version(), IntentID: value.IntentID(), IntentVersion: value.IntentVersion(), Status: string(value.Status()), Decision: string(value.Decision()), CreatedAt: value.CreatedAt().UTC().Format(time.RFC3339Nano), ExpiresAt: value.ExpiresAt().UTC().Format(time.RFC3339Nano)}
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
	case apperrors.CodeApprovalNotFound:
		return http.StatusNotFound
	case apperrors.CodeValidationError:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
