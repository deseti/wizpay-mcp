package approval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/services"
)

type serviceStub struct {
	value    approvals.Approval
	decision approvals.Decision
	err      error
}

func (s *serviceStub) RequestApproval(context.Context, string) (approvals.Approval, error) {
	return s.value, nil
}
func (s *serviceStub) GetApproval(context.Context, string) (approvals.Approval, error) {
	return s.value, s.err
}
func (s *serviceStub) DecideApproval(_ context.Context, _ string, decision approvals.Decision) (approvals.Approval, error) {
	s.decision = decision
	return s.value, s.err
}

func TestGetApprovalReturnsSafeMetadata(t *testing.T) {
	service := &serviceStub{value: approvals.Approval{}}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/approval/apr_1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"approval_id":""`) {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "wallet") || strings.Contains(response.Body.String(), "provider") {
		t.Fatal("approval response exposed sensitive fields")
	}
}

func TestApprovalDecisionApproveAndReject(t *testing.T) {
	for _, want := range []approvals.Decision{approvals.DecisionApproved, approvals.DecisionRejected} {
		t.Run(string(want), func(t *testing.T) {
			service := &serviceStub{}
			handler, err := NewHandler(service)
			if err != nil {
				t.Fatal(err)
			}
			body := strings.NewReader(`{"decision":"` + string(want) + `"}`)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/approval/apr_1/decision", body))
			if response.Code != http.StatusOK || service.decision != want {
				t.Fatalf("status=%d decision=%q", response.Code, service.decision)
			}
		})
	}
}

func TestApprovalDecisionRejectsInvalidInput(t *testing.T) {
	service := &serviceStub{}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/approval/apr_1/decision", strings.NewReader(`{"decision":"MAYBE"}`)))
	if response.Code != http.StatusBadRequest || service.decision != "" {
		t.Fatalf("status=%d decision=%q", response.Code, service.decision)
	}
}

func TestWrongUserCannotAccessApproval(t *testing.T) {
	service := &serviceStub{err: apperrors.New(apperrors.CodeAuthorizationRequired, "Access is forbidden.", false, true, true)}
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/approval/apr_1", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}

var _ services.ApprovalService = (*serviceStub)(nil)
