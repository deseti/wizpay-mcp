package services

import (
	"context"
	"testing"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestSendExecuteFailsClosedWithoutMainnetAuthority(t *testing.T) {
	service := &PersistedSendService{}
	_, err := service.ExecuteSend(context.Background(), "intent", "approval", "policy", 1)
	if public := apperrors.ToPublic(err); public.Code != apperrors.CodeCapabilityUnavailable {
		t.Fatalf("error = %v, public = %+v", err, public)
	}
}
