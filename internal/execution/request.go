package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/approvals"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
)

const (
	RequestVersion          uint64 = 1
	executionIdentityDomain        = "WIZPAY_MCP_EXECUTION_V1\n"
	executionRequestDomain         = "WIZPAY_MCP_EXECUTION_REQUEST_V1\n"
	policyEvaluationDomain         = "WIZPAY_MCP_POLICY_EVALUATION_REFERENCE_V1\n"
)

// Request contains authorization references only. It has no transaction data,
// credentials, signatures, provider payloads, or financial replacement fields.
type Request struct {
	requestID             string
	requestKey            string
	version               uint64
	executionID           string
	operationKey          string
	operationVersion      uint64
	intentID              string
	intentVersion         uint64
	intentDigest          string
	approvalID            string
	approvalVersion       uint64
	policyID              string
	policyVersion         uint64
	policyEvaluationKey   string
	policyEvaluationStage policies.EvaluationStage
	policyEvaluatedAt     time.Time
	createdAt             time.Time
}

// NewRequest binds one consumed approval and one allowing pre-execution policy
// result to the Phase 3 logical operation identity.
func NewRequest(intent intents.Intent, approval approvals.Approval, policyResult policies.Result, at time.Time) (Request, error) {
	if err := intent.Validate(); err != nil {
		return Request{}, err
	}
	if err := approval.Validate(); err != nil {
		return Request{}, err
	}
	if at.IsZero() {
		return Request{}, invalidExecution(fmt.Errorf("request creation time is required"))
	}
	at = at.UTC()
	if intent.Status() != intents.StatusApproved {
		return Request{}, apperrors.New(apperrors.CodeExecutionNotAuthorized, "Execution request is not authorized.", false, true, true)
	}
	if !at.Before(intent.ExpiresAt()) || !at.Before(intent.Constraints().Deadline) {
		return Request{}, apperrors.New(apperrors.CodeIntentExpired, "Intent has expired.", false, true, true)
	}
	operation, err := intents.NewOperationIdentity(intent)
	if err != nil {
		return Request{}, err
	}
	owner := intent.Ownership()
	if approval.Status() != approvals.StatusConsumed || approval.Decision() != approvals.DecisionApproved ||
		approval.IntentID() != intent.IntentID() || approval.IntentVersion() != intent.Version() || approval.IntentDigest() != intent.Digest() ||
		approval.UserID() != owner.UserID || approval.WalletBindingID() != owner.WalletBindingID ||
		approval.WalletBindingVersion() != owner.WalletBindingVersion || approval.WalletID() != owner.WalletID ||
		approval.WalletAddress() != owner.WalletAddress || approval.ChainID() != owner.ChainID ||
		approval.OperationKey() != operation.OperationKey() || approval.OperationVersion() != operation.Version() {
		return Request{}, apperrors.New(apperrors.CodeExecutionNotAuthorized, "Execution approval does not match the intent.", false, true, true)
	}
	if approval.ConsumedAt().After(at) {
		return Request{}, invalidExecution(fmt.Errorf("approval consumption cannot follow request creation"))
	}
	if policyResult.Decision != policies.DecisionAllow || policyResult.Stage != policies.EvaluationStageBeforeExecution ||
		len(policyResult.Findings) != 0 || policyResult.PolicyID == "" || policyResult.PolicyVersion == 0 ||
		policyResult.IntentID != intent.IntentID() || policyResult.IntentVersion != intent.Version() || policyResult.IntentDigest != intent.Digest() ||
		policyResult.EvaluatedAt.IsZero() || policyResult.EvaluatedAt.After(approval.ConsumedAt()) ||
		intent.Constraints().PolicyReference != fmt.Sprintf("%s:%d", policyResult.PolicyID, policyResult.PolicyVersion) {
		return Request{}, apperrors.New(apperrors.CodeExecutionNotAuthorized, "Allowing policy evaluation does not match the intent.", false, true, true)
	}

	executionID := "exec_" + hash(executionIdentityDomain, operation.OperationKey(), fmt.Sprint(operation.Version()))
	policyEvaluationKey := hash(policyEvaluationDomain,
		policyResult.PolicyID, fmt.Sprint(policyResult.PolicyVersion), policyResult.IntentID,
		fmt.Sprint(policyResult.IntentVersion), policyResult.IntentDigest, string(policyResult.Stage),
		policyResult.EvaluatedAt.UTC().Format(time.RFC3339Nano), string(policyResult.Decision))
	requestKey := hash(executionRequestDomain,
		operation.OperationKey(), fmt.Sprint(operation.Version()), approval.ApprovalID(), fmt.Sprint(approval.Version()),
		policyEvaluationKey)
	request := Request{
		requestID: "exreq_" + requestKey, requestKey: requestKey, version: RequestVersion, executionID: executionID,
		operationKey: operation.OperationKey(), operationVersion: operation.Version(),
		intentID: intent.IntentID(), intentVersion: intent.Version(), intentDigest: intent.Digest(),
		approvalID: approval.ApprovalID(), approvalVersion: approval.Version(),
		policyID: policyResult.PolicyID, policyVersion: policyResult.PolicyVersion,
		policyEvaluationKey:   policyEvaluationKey,
		policyEvaluationStage: policyResult.Stage, policyEvaluatedAt: policyResult.EvaluatedAt.UTC(), createdAt: approval.ConsumedAt(),
	}
	if err := request.Validate(); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (r Request) Validate() error {
	for _, field := range []struct{ name, value string }{
		{"request ID", r.requestID}, {"request key", r.requestKey}, {"execution ID", r.executionID},
		{"operation key", r.operationKey}, {"intent ID", r.intentID}, {"intent digest", r.intentDigest},
		{"approval ID", r.approvalID}, {"policy ID", r.policyID}, {"policy evaluation key", r.policyEvaluationKey},
	} {
		if err := validateExecutionText(field.name, field.value); err != nil {
			return invalidExecution(err)
		}
	}
	if r.version != RequestVersion || r.operationVersion == 0 || r.intentVersion == 0 || r.approvalVersion == 0 || r.policyVersion == 0 {
		return invalidExecution(fmt.Errorf("execution request version is invalid"))
	}
	if r.policyEvaluationStage != policies.EvaluationStageBeforeExecution {
		return invalidExecution(fmt.Errorf("execution request requires a pre-execution policy evaluation"))
	}
	if r.policyEvaluatedAt.IsZero() || r.createdAt.IsZero() || r.policyEvaluatedAt.After(r.createdAt) {
		return invalidExecution(fmt.Errorf("execution request timestamps are invalid"))
	}
	wantExecutionID := "exec_" + hash(executionIdentityDomain, r.operationKey, fmt.Sprint(r.operationVersion))
	wantPolicyEvaluationKey := hash(policyEvaluationDomain,
		r.policyID, fmt.Sprint(r.policyVersion), r.intentID, fmt.Sprint(r.intentVersion), r.intentDigest,
		string(r.policyEvaluationStage), r.policyEvaluatedAt.UTC().Format(time.RFC3339Nano), string(policies.DecisionAllow))
	wantRequestKey := hash(executionRequestDomain,
		r.operationKey, fmt.Sprint(r.operationVersion), r.approvalID, fmt.Sprint(r.approvalVersion),
		wantPolicyEvaluationKey)
	if r.executionID != wantExecutionID || r.policyEvaluationKey != wantPolicyEvaluationKey || r.requestKey != wantRequestKey || r.requestID != "exreq_"+wantRequestKey {
		return invalidExecution(fmt.Errorf("execution request identity is invalid"))
	}
	return nil
}

func invalidExecution(cause error) error {
	return apperrors.Wrap(apperrors.CodeExecutionInvalid, "Execution request is invalid.", false, true, true, cause)
}

func hash(domain string, parts ...string) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(domain))
	for _, part := range parts {
		_, _ = fmt.Fprintf(digest, "%d:", len(part))
		_, _ = digest.Write([]byte(part))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func (r Request) RequestID() string                               { return r.requestID }
func (r Request) RequestKey() string                              { return r.requestKey }
func (r Request) Version() uint64                                 { return r.version }
func (r Request) ExecutionID() string                             { return r.executionID }
func (r Request) OperationKey() string                            { return r.operationKey }
func (r Request) OperationVersion() uint64                        { return r.operationVersion }
func (r Request) IntentID() string                                { return r.intentID }
func (r Request) IntentVersion() uint64                           { return r.intentVersion }
func (r Request) IntentDigest() string                            { return r.intentDigest }
func (r Request) ApprovalID() string                              { return r.approvalID }
func (r Request) ApprovalVersion() uint64                         { return r.approvalVersion }
func (r Request) PolicyID() string                                { return r.policyID }
func (r Request) PolicyVersion() uint64                           { return r.policyVersion }
func (r Request) PolicyEvaluationKey() string                     { return r.policyEvaluationKey }
func (r Request) PolicyEvaluationStage() policies.EvaluationStage { return r.policyEvaluationStage }
func (r Request) PolicyEvaluatedAt() time.Time                    { return r.policyEvaluatedAt }
func (r Request) CreatedAt() time.Time                            { return r.createdAt }
