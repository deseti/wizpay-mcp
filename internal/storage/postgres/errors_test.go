package postgres

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func TestDatabaseErrorsMapToSanitizedRepositorySemantics(t *testing.T) {
	tests := []struct {
		name, sqlstate string
		code           apperrors.Code
		retryable      bool
	}{
		{name: "unique", sqlstate: "23505", code: apperrors.CodeExecutionConflict, retryable: true},
		{name: "foreign_key", sqlstate: "23503", code: apperrors.CodeValidationError},
		{name: "check", sqlstate: "23514", code: apperrors.CodeValidationError},
		{name: "serialization", sqlstate: "40001", code: apperrors.CodeExecutionConflict, retryable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapped := mapDatabaseError(&pgconn.PgError{Code: test.sqlstate, Message: "sensitive SQL details"})
			public := apperrors.ToPublic(mapped)
			if public.Code != test.code || public.Retryable != test.retryable {
				t.Fatalf("mapped error = %#v", public)
			}
			if public.Message == "sensitive SQL details" {
				t.Fatal("raw database details reached public error")
			}
		})
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		mapped := mapDatabaseError(cause)
		if !stderrors.Is(mapped, cause) || !apperrors.ToPublic(mapped).Retryable {
			t.Fatalf("context mapping = %v", mapped)
		}
	}
}
