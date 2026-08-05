package intents

import (
	"fmt"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

// Restore reconstructs a persisted intent and revalidates its immutable
// material and digest. It performs no I/O and grants no authorization.
//
// Historical restore invariants:
//   - Representation normalization only (trim/UTC/clone); no schema stamping.
//   - No address-case rewriting (would change digests frozen before Phase 12).
//   - No invention of execution-critical Phase 12 fields.
//   - Legacy schema_version 0 remains 0; Phase12Executable stays false.
//   - No silent upgrade or migration of approved material.
func Restore(params Params, status Status, digest string, lifecycleRevision uint64) (Intent, error) {
	params = normalizeParams(params)
	if err := validateParams(params); err != nil {
		return Intent{}, apperrors.Wrap(apperrors.CodeValidationError, "Intent is invalid.", false, true, true, err)
	}
	value := Intent{params: params, status: status, digest: digest, lifecycleRevision: lifecycleRevision}
	if err := value.Validate(); err != nil {
		return Intent{}, err
	}
	if status != StatusDraft {
		canonical, err := canonicalMaterial(params)
		if err != nil {
			return Intent{}, err
		}
		if digest != digestBytes(canonical) {
			return Intent{}, apperrors.New(apperrors.CodeIntentMutated, "Intent material does not match its digest.", false, true, true)
		}
	}
	if !status.Valid() {
		return Intent{}, fmt.Errorf("invalid persisted intent status")
	}
	return value, nil
}
