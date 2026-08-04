package contracts

import (
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

func validationError(message string, cause error) error {
	if cause == nil {
		return apperrors.New(apperrors.CodeValidationError, message, false, true, true)
	}
	return apperrors.Wrap(apperrors.CodeValidationError, message, false, true, true, cause)
}

func contractNotFound() error {
	return apperrors.New(apperrors.CodeContractNotFound, "Contract deployment was not found.", false, true, true)
}

func contractUnavailable() error {
	return apperrors.New(apperrors.CodeContractUnavailable, "Contract deployment is unavailable.", false, true, false)
}

func contractConflict() error {
	return apperrors.New(apperrors.CodeContractConflict, "Contract deployment conflicts with an existing definition.", false, true, true)
}
