package providers

import (
	"context"
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/storage"
)

// EvidenceReferenceStore recovers the newest safe provider reference from the
// Phase 7 verification evidence already persisted for an execution.
//
// This is what makes reconciliation restart-safe. The Phase 9 adapter contract
// identifies a reconciliation by execution ID alone, so the reference must be
// read back from durable evidence rather than held in memory. Nothing new is
// stored: it reads the same adapter reference the runtime persisted, which is
// why Phase 11 needs no schema change.
type EvidenceReferenceStore struct {
	evidence storage.VerificationEvidenceRepository
}

func NewEvidenceReferenceStore(evidence storage.VerificationEvidenceRepository) (*EvidenceReferenceStore, error) {
	if evidence == nil {
		return nil, fmt.Errorf("verification evidence repository is required")
	}
	return &EvidenceReferenceStore{evidence: evidence}, nil
}

// LatestReference returns the most recently persisted provider reference for an
// execution.
//
// A reference that cannot be parsed is reported as an error rather than as
// absence. Absence is what permits a first submission, so silently discarding an
// unreadable reference could allow a duplicate submission of an execution that
// already reached the provider.
func (s *EvidenceReferenceStore) LatestReference(ctx context.Context, executionID string) (Reference, bool, error) {
	scope, found := storage.ScopeFromContext(ctx)
	if !found {
		return Reference{}, false, fmt.Errorf("persistence scope is unavailable for provider reconciliation")
	}
	records, err := s.evidence.FindVerificationEvidence(ctx, scope, executionID)
	if err != nil {
		return Reference{}, false, err
	}
	for index := len(records) - 1; index >= 0; index-- {
		encoded := records[index].AdapterReference()
		if encoded == "" {
			continue
		}
		reference, parseErr := ParseReference(encoded)
		if parseErr != nil {
			return Reference{}, false, parseErr
		}
		return reference, true, nil
	}
	return Reference{}, false, nil
}
