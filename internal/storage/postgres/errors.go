package postgres

import (
	"context"
	stderrors "errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return apperrors.Wrap(apperrors.CodeInternalError, "Database operation was interrupted.", true, false, false, err)
	}
	if stderrors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var pgError *pgconn.PgError
	if stderrors.As(err, &pgError) {
		switch pgError.Code {
		case "23505":
			return apperrors.Wrap(apperrors.CodeExecutionConflict, "A concurrent persistence conflict occurred.", true, false, false, err)
		case "40001", "40P01":
			return apperrors.Wrap(apperrors.CodeExecutionConflict, "A concurrent persistence conflict occurred.", true, false, false, err)
		case "23502", "23503", "23514", "22000", "22P02", "55000":
			return apperrors.Wrap(apperrors.CodeValidationError, "Persistence constraints rejected the operation.", false, true, true, err)
		}
	}
	return apperrors.Wrap(apperrors.CodeInternalError, "A persistence error occurred.", true, false, false, err)
}

func isUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return stderrors.As(err, &pgError) && pgError.Code == "23505"
}

func notFound(code apperrors.Code, message string) error {
	return apperrors.New(code, message, false, true, true)
}
