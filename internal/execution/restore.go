package execution

import (
	"time"

	"github.com/deseti/wizpay-mcp/internal/policies"
)

type RestoreRequestParams struct {
	RequestID             string
	RequestKey            string
	Version               uint64
	ExecutionID           string
	OperationKey          string
	OperationVersion      uint64
	IntentID              string
	IntentVersion         uint64
	IntentDigest          string
	ApprovalID            string
	ApprovalVersion       uint64
	PolicyID              string
	PolicyVersion         uint64
	PolicyEvaluationKey   string
	PolicyEvaluationStage policies.EvaluationStage
	PolicyEvaluatedAt     time.Time
	CreatedAt             time.Time
}

func RestoreRequest(p RestoreRequestParams) (Request, error) {
	value := Request{requestID: p.RequestID, requestKey: p.RequestKey, version: p.Version, executionID: p.ExecutionID, operationKey: p.OperationKey, operationVersion: p.OperationVersion, intentID: p.IntentID, intentVersion: p.IntentVersion, intentDigest: p.IntentDigest, approvalID: p.ApprovalID, approvalVersion: p.ApprovalVersion, policyID: p.PolicyID, policyVersion: p.PolicyVersion, policyEvaluationKey: p.PolicyEvaluationKey, policyEvaluationStage: p.PolicyEvaluationStage, policyEvaluatedAt: p.PolicyEvaluatedAt.UTC(), createdAt: p.CreatedAt.UTC()}
	if err := value.Validate(); err != nil {
		return Request{}, err
	}
	return value, nil
}

type RestoreExecutionParams struct {
	Request    Request
	Status     Status
	Revision   uint64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Failure    Failure
	FailedFrom Status
	Recovery   Recovery
}

func Restore(p RestoreExecutionParams) (Execution, error) {
	value := Execution{request: p.Request, status: p.Status, revision: p.Revision, createdAt: p.CreatedAt.UTC(), updatedAt: p.UpdatedAt.UTC(), failure: p.Failure, failedFrom: p.FailedFrom, recovery: p.Recovery}
	if err := value.Validate(); err != nil {
		return Execution{}, err
	}
	return value, nil
}
