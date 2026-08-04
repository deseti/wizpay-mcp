package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/audit"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

type Config struct {
	WorkerID      string
	LeaseDuration time.Duration
	RetryInterval time.Duration
}

func (c Config) Validate() error {
	if c.WorkerID == "" || c.LeaseDuration <= 0 || c.LeaseDuration > time.Hour || c.RetryInterval <= 0 || c.RetryInterval > time.Hour {
		return fmt.Errorf("valid worker ID, lease duration, and retry interval are required")
	}
	return nil
}

type Service struct {
	executions storage.ExecutionRepository
	evidence   storage.VerificationEvidenceRepository
	runtime    storage.ExecutionRuntimeRepository
	adapter    execution.Adapter
	verifier   Verifier
	config     Config
	now        func() time.Time
}

func NewService(executions storage.ExecutionRepository, evidence storage.VerificationEvidenceRepository, runtimeRepository storage.ExecutionRuntimeRepository, adapter execution.Adapter, verifier Verifier, config Config, now func() time.Time) (*Service, error) {
	if executions == nil || evidence == nil || runtimeRepository == nil || adapter == nil || verifier == nil || now == nil {
		return nil, fmt.Errorf("execution runtime dependencies are required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Service{executions: executions, evidence: evidence, runtime: runtimeRepository, adapter: adapter, verifier: verifier, config: config, now: now}, nil
}

func (s *Service) Start(ctx context.Context, scope storage.Scope, request execution.Request) (execution.Execution, error) {
	if err := scope.Validate(); err != nil {
		return execution.Execution{}, err
	}
	value, err := execution.New(request)
	if err != nil {
		return execution.Execution{}, err
	}
	result, err := s.executions.CreateExecution(ctx, scope, value)
	if err != nil {
		return execution.Execution{}, err
	}
	return result.Execution, nil
}

func (s *Service) Process(ctx context.Context, scope storage.Scope, executionID string) (execution.Execution, error) {
	now := s.now().UTC()
	claim, acquired, err := s.runtime.ClaimExecutionWork(ctx, scope, executionID, s.config.WorkerID, now, s.config.LeaseDuration)
	if err != nil {
		return execution.Execution{}, err
	}
	if !acquired {
		current, findErr := s.executions.FindExecutionByID(ctx, scope, executionID)
		return current, findErr
	}
	value, processErr := s.processClaim(ctx, claim, now)
	nextRun := now.Add(s.config.RetryInterval)
	if value.Terminal() {
		nextRun = now.Add(time.Hour)
	}
	_, releaseErr := s.runtime.ReleaseExecutionWork(ctx, claim, nextRun)
	if processErr != nil {
		return value, processErr
	}
	if releaseErr != nil {
		return value, releaseErr
	}
	return value, nil
}

func (s *Service) ProcessClaim(ctx context.Context, claim storage.ExecutionClaim) (execution.Execution, error) {
	return s.processClaim(ctx, claim, s.now().UTC())
}

func (s *Service) processClaim(ctx context.Context, claim storage.ExecutionClaim, now time.Time) (execution.Execution, error) {
	value := claim.Execution
	switch value.Status() {
	case execution.StatusCreated:
		return s.transition(ctx, claim, value, execution.StatusAuthorized, now, "")
	case execution.StatusAuthorized:
		return s.transition(ctx, claim, value, execution.StatusQueued, now, "")
	case execution.StatusQueued:
		return s.transition(ctx, claim, value, execution.StatusExecuting, now, "")
	case execution.StatusExecuting:
		return s.submitOrReconcile(ctx, claim, value, now)
	case execution.StatusSubmitted:
		return s.transition(ctx, claim, value, execution.StatusConfirming, now, "")
	case execution.StatusConfirming:
		return s.verify(ctx, claim, value, now)
	case execution.StatusConfirmed:
		return s.verify(ctx, claim, value, now)
	case execution.StatusVerified:
		verified, err := hasVerifiedEvidence(ctx, s.evidence, claim.Scope, value)
		if err != nil {
			return value, err
		}
		if !verified {
			return value, apperrors.New(apperrors.CodeExecutionRecoverable, "Verified execution evidence is unavailable.", true, false, false)
		}
		return s.transition(ctx, claim, value, execution.StatusCompleted, now, "")
	case execution.StatusFailed:
		failure, ok := value.Failure()
		if !ok || failure.Eligibility == execution.RecoveryTerminal {
			return value, nil
		}
		next, err := value.Recover(now)
		if err != nil {
			return value, err
		}
		return s.persist(ctx, claim, value, next, now, "")
	case execution.StatusRecoveryRequired:
		next, err := value.Resume(now)
		if err != nil {
			return value, err
		}
		return s.persist(ctx, claim, value, next, now, "")
	case execution.StatusCompleted, execution.StatusCancelled:
		return value, nil
	default:
		return value, apperrors.New(apperrors.CodeExecutionInvalid, "Execution runtime state is invalid.", false, true, true)
	}
}

func (s *Service) submitOrReconcile(ctx context.Context, claim storage.ExecutionClaim, value execution.Execution, now time.Time) (execution.Execution, error) {
	var result execution.Result
	var err error
	if claim.SubmissionStarted {
		result, err = s.adapter.GetStatus(ctx, value.ExecutionID())
	} else {
		marked, started, markErr := s.runtime.MarkSubmissionStarted(ctx, claim, now)
		if markErr != nil {
			return value, markErr
		}
		if !started {
			next, transitionErr := value.RequireRecovery("SUBMISSION_ALREADY_STARTED", now)
			if transitionErr != nil {
				return value, transitionErr
			}
			return s.persist(ctx, claim, value, next, now, "SUBMISSION_ALREADY_STARTED")
		}
		claim = marked
		result, err = s.adapter.Execute(ctx, value.Request())
	}
	if err != nil {
		if classified, ok := adapterFailure(err); ok {
			failure := execution.Failure{Code: classified.ReasonCode, Eligibility: execution.RecoveryRecoverable, RecoveryTarget: execution.StatusExecuting}
			if classified.Kind == AdapterFailurePermanent {
				failure.Eligibility, failure.RecoveryTarget = execution.RecoveryTerminal, ""
			}
			next, transitionErr := value.Fail(failure, now)
			if transitionErr != nil {
				return value, transitionErr
			}
			observation, observationErr := execution.NewResult(execution.ResultParams{ExecutionID: next.ExecutionID(), ExecutionVersion: next.Revision(), Status: execution.StatusFailed, ErrorCode: classified.ReasonCode, ObservedAt: now})
			if observationErr != nil {
				return value, observationErr
			}
			return s.persistFailureObservation(ctx, claim, value, next, observation, now, classified.ReasonCode)
		}
		next, transitionErr := value.RequireRecovery("SUBMISSION_OUTCOME_AMBIGUOUS", now)
		if transitionErr != nil {
			return value, transitionErr
		}
		observation, observationErr := execution.NewResult(execution.ResultParams{ExecutionID: next.ExecutionID(), ExecutionVersion: next.Revision(), Status: execution.StatusRecoveryRequired, ErrorCode: "SUBMISSION_OUTCOME_AMBIGUOUS", ObservedAt: now})
		if observationErr != nil {
			return value, observationErr
		}
		return s.persistFailureObservation(ctx, claim, value, next, observation, now, "SUBMISSION_OUTCOME_AMBIGUOUS")
	}
	if err := result.EnsureMatches(value.Request()); err != nil {
		return value, err
	}
	switch result.Status() {
	case execution.StatusSubmitted:
		return s.persistObservation(ctx, claim, value, result, execution.StatusSubmitted, now)
	case execution.StatusConfirming, execution.StatusConfirmed:
		return s.persistObservation(ctx, claim, value, result, execution.StatusConfirming, now)
	case execution.StatusFailed:
		next, transitionErr := value.Fail(execution.Failure{Code: result.ErrorCode(), Eligibility: execution.RecoveryTerminal}, now)
		if transitionErr != nil {
			return value, transitionErr
		}
		return s.persistFailureObservation(ctx, claim, value, next, result, now, result.ErrorCode())
	case execution.StatusRecoveryRequired:
		next, transitionErr := value.RequireRecovery(result.ErrorCode(), now)
		if transitionErr != nil {
			return value, transitionErr
		}
		return s.persistFailureObservation(ctx, claim, value, next, result, now, result.ErrorCode())
	default:
		return value, apperrors.New(apperrors.CodeExecutionInvalid, "Execution adapter returned an invalid runtime state.", false, true, true)
	}
}

func (s *Service) verify(ctx context.Context, claim storage.ExecutionClaim, value execution.Execution, now time.Time) (execution.Execution, error) {
	reference, err := latestReference(ctx, s.evidence, claim.Scope, value.ExecutionID())
	if err != nil {
		return value, err
	}
	result, err := s.verifier.Verify(ctx, value, reference)
	if err != nil {
		if _, ok := verificationFailure(err); ok {
			return value, nil
		}
		next, transitionErr := value.RequireRecovery("VERIFICATION_OUTCOME_AMBIGUOUS", now)
		if transitionErr != nil {
			return value, transitionErr
		}
		return s.persist(ctx, claim, value, next, now, "VERIFICATION_OUTCOME_AMBIGUOUS")
	}
	if err := result.Validate(); err != nil {
		return value, apperrors.Wrap(apperrors.CodeExecutionInvalid, "Verification result is invalid.", false, true, true, err)
	}
	switch result.Outcome {
	case VerificationPending:
		return value, nil
	case VerificationFailed:
		next, transitionErr := value.Fail(execution.Failure{Code: result.ReasonCode, Eligibility: execution.RecoveryTerminal}, now)
		if transitionErr != nil {
			return value, transitionErr
		}
		observation, observationErr := execution.NewResult(execution.ResultParams{ExecutionID: next.ExecutionID(), ExecutionVersion: next.Revision(), Status: execution.StatusFailed, ErrorCode: result.ReasonCode, ObservedAt: result.ObservedAt})
		if observationErr != nil {
			return value, observationErr
		}
		return s.persistFailureObservation(ctx, claim, value, next, observation, now, result.ReasonCode)
	case VerificationVerified:
		confirmed := value
		if value.Status() == execution.StatusConfirming {
			var transitionErr error
			confirmed, transitionErr = value.Transition(execution.StatusConfirmed, now)
			if transitionErr != nil {
				return value, transitionErr
			}
			confirmed, err = s.persist(ctx, claim, value, confirmed, now, "")
			if err != nil {
				return value, err
			}
			claim.Execution = confirmed
		}
		verified, transitionErr := confirmed.Transition(execution.StatusVerified, now)
		if transitionErr != nil {
			return confirmed, transitionErr
		}
		evidence, evidenceErr := execution.NewResult(execution.ResultParams{ExecutionID: verified.ExecutionID(), ExecutionVersion: verified.Revision(), Status: execution.StatusVerified, AdapterReference: result.Reference, ObservedAt: result.ObservedAt})
		if evidenceErr != nil {
			return confirmed, evidenceErr
		}
		record := auditRecord(claim.Scope, claim.LeaseOwner, confirmed, verified, now, "")
		return s.runtime.PersistClaimedEvidence(ctx, claim, evidence, verified, confirmed.Revision(), record, now)
	default:
		return value, apperrors.New(apperrors.CodeExecutionInvalid, "Verification result is invalid.", false, true, true)
	}
}

func (s *Service) persistObservation(ctx context.Context, claim storage.ExecutionClaim, current execution.Execution, result execution.Result, status execution.Status, now time.Time) (execution.Execution, error) {
	next, err := current.Transition(status, now)
	if err != nil {
		return current, err
	}
	evidence, err := execution.NewResult(execution.ResultParams{ExecutionID: next.ExecutionID(), ExecutionVersion: next.Revision(), Status: next.Status(), AdapterReference: result.AdapterReference(), ObservedAt: result.ObservedAt()})
	if err != nil {
		return current, err
	}
	return s.runtime.PersistClaimedObservation(ctx, claim, evidence, next, current.Revision(), auditRecord(claim.Scope, claim.LeaseOwner, current, next, now, ""), now)
}

func (s *Service) persistFailureObservation(ctx context.Context, claim storage.ExecutionClaim, current, next execution.Execution, result execution.Result, now time.Time, reason string) (execution.Execution, error) {
	observation, err := execution.NewResult(execution.ResultParams{ExecutionID: next.ExecutionID(), ExecutionVersion: next.Revision(), Status: next.Status(), AdapterReference: result.AdapterReference(), ErrorCode: result.ErrorCode(), ObservedAt: result.ObservedAt()})
	if err != nil {
		return current, err
	}
	return s.runtime.PersistClaimedObservation(ctx, claim, observation, next, current.Revision(), auditRecord(claim.Scope, claim.LeaseOwner, current, next, now, reason), now)
}

func (s *Service) transition(ctx context.Context, claim storage.ExecutionClaim, current execution.Execution, status execution.Status, now time.Time, reason string) (execution.Execution, error) {
	next, err := current.Transition(status, now)
	if err != nil {
		return current, err
	}
	return s.persist(ctx, claim, current, next, now, reason)
}
func (s *Service) persist(ctx context.Context, claim storage.ExecutionClaim, current, next execution.Execution, now time.Time, reason string) (execution.Execution, error) {
	return s.runtime.UpdateClaimedExecution(ctx, claim, next, current.Revision(), auditRecord(claim.Scope, claim.LeaseOwner, current, next, now, reason), now)
}

func latestReference(ctx context.Context, repository storage.VerificationEvidenceRepository, scope storage.Scope, executionID string) (string, error) {
	values, err := repository.FindVerificationEvidence(ctx, scope, executionID)
	if err != nil {
		return "", err
	}
	for index := len(values) - 1; index >= 0; index-- {
		if values[index].AdapterReference() != "" {
			return values[index].AdapterReference(), nil
		}
	}
	return "", apperrors.New(apperrors.CodeExecutionRecoverable, "Execution verification reference is unavailable.", true, false, false)
}

func hasVerifiedEvidence(ctx context.Context, repository storage.VerificationEvidenceRepository, scope storage.Scope, value execution.Execution) (bool, error) {
	values, err := repository.FindVerificationEvidence(ctx, scope, value.ExecutionID())
	if err != nil {
		return false, err
	}
	for _, evidence := range values {
		if evidence.Status() == execution.StatusVerified && evidence.ExecutionVersion() == value.Revision() {
			return true, nil
		}
	}
	return false, nil
}

func auditRecord(scope storage.Scope, workerID string, previous, next execution.Execution, at time.Time, reason string) audit.Record {
	request := next.Request()
	typeByStatus := map[execution.Status]audit.EventType{
		execution.StatusAuthorized: audit.EventExecutionAuthorized, execution.StatusQueued: audit.EventExecutionQueued,
		execution.StatusExecuting: audit.EventExecutionExecuting,
		execution.StatusSubmitted: audit.EventExecutionSubmitted, execution.StatusConfirming: audit.EventExecutionConfirming,
		execution.StatusConfirmed: audit.EventExecutionConfirmed, execution.StatusVerified: audit.EventExecutionVerified,
		execution.StatusCompleted: audit.EventExecutionCompleted, execution.StatusFailed: audit.EventExecutionFailed,
		execution.StatusRecoveryRequired: audit.EventExecutionRecoveryRequired,
	}
	eventType := typeByStatus[next.Status()]
	if previous.Status() == execution.StatusRecoveryRequired {
		eventType = audit.EventExecutionRecovered
	}
	if eventType == "" {
		eventType = audit.EventExecutionRecovered
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", next.ExecutionID(), next.Revision(), next.Status())))
	return audit.Record{Event: audit.Event{EventID: "event_" + hex.EncodeToString(digest[:]), Type: eventType, OccurredAt: at, IntentID: request.IntentID(), IntentVersion: request.IntentVersion(), IntentDigest: request.IntentDigest(), ApprovalID: request.ApprovalID(), PolicyID: request.PolicyID(), PolicyVersion: request.PolicyVersion(), PolicyDecision: "ALLOW", PolicyEvaluationKey: request.PolicyEvaluationKey(), ExecutionID: next.ExecutionID(), ExecutionRevision: next.Revision(), ExecutionRequestID: request.RequestID(), ExecutionRequestKey: request.RequestKey(), ExecutionStatus: string(next.Status()), RecoveryReasonCode: reason, UserID: scope.ActorID(), OperationKey: request.OperationKey(), OperationVersion: request.OperationVersion()}, ActorType: "system", ActorID: workerID, RequestID: scope.RequestID(), TraceID: scope.TraceID(), ResourceType: "execution", ResourceID: next.ExecutionID(), PreviousState: string(previous.Status()), NewState: string(next.Status()), SafeReasonCode: reason, SourceComponent: "execution_runtime"}
}
