package policies

// Restore reconstructs a persisted policy through the same canonicalization
// and validation used at creation.
func Restore(params Params, status Status, lifecycleRevision uint64) (Policy, error) {
	value, err := NewDraft(params)
	if err != nil {
		return Policy{}, err
	}
	value.status = status
	value.lifecycleRevision = lifecycleRevision
	if err := value.Validate(); err != nil {
		return Policy{}, err
	}
	return value, nil
}
