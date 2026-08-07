package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/audit"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

type failingAutonomyAudit struct{ err error }

func (f failingAutonomyAudit) AppendAudit(context.Context, storage.Scope, audit.Record) error {
	return f.err
}
func (f failingAutonomyAudit) FindAuditByResource(context.Context, storage.Scope, string, string) ([]audit.Record, error) {
	return nil, nil
}

func TestAutonomyAuditFailureIsReturned(t *testing.T) {
	scope, err := storage.NewScope("tenant_1", "user_1", "request_1", "trace_1")
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("audit unavailable")
	service := PersistedAutonomyService{Audit: failingAutonomyAudit{err: want}, Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	if err := service.appendAudit(context.Background(), scope, audit.EventAutonomyScheduleCreated, "schedule_1", "", "ACTIVE"); !errors.Is(err, want) {
		t.Fatalf("audit error = %v, want %v", err, want)
	}
}
