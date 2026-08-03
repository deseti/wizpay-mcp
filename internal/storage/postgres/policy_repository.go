package postgres

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"reflect"

	"github.com/jackc/pgx/v5"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
)

func policyCreateParams(scope storage.Scope, value policies.Policy) (dbsqlc.CreatePolicyParams, error) {
	version, err := dbVersion(value.Version())
	if err != nil {
		return dbsqlc.CreatePolicyParams{}, err
	}
	rules, err := json.Marshal(value.Rules())
	if err != nil {
		return dbsqlc.CreatePolicyParams{}, err
	}
	policyScope := value.Scope()
	types := make([]string, len(policyScope.IntentTypes))
	for i, kind := range policyScope.IntentTypes {
		types[i] = string(kind)
	}
	lifecycleRevision, err := dbVersion(value.LifecycleRevision())
	if err != nil {
		return dbsqlc.CreatePolicyParams{}, err
	}
	return dbsqlc.CreatePolicyParams{TenantID: scope.TenantID(), PolicyID: value.PolicyID(), PolicyVersion: version, Name: value.Name(), UserID: policyScope.UserID, WalletBindingID: dbOptionalString(policyScope.WalletBindingID), IntentTypes: types, Rules: rules, Status: string(value.Status()), CreatedAt: dbTime(value.CreatedAt()), ValidFrom: dbTime(value.ValidFrom()), ExpiresAt: dbTime(value.ExpiresAt()), LifecycleVersion: lifecycleRevision}, nil
}
func policyFromRow(row dbsqlc.Policy) (policies.Policy, error) {
	version, err := domainVersion(row.PolicyVersion)
	if err != nil {
		return policies.Policy{}, err
	}
	var rules []policies.Rule
	if err := json.Unmarshal(row.Rules, &rules); err != nil {
		return policies.Policy{}, apperrors.Wrap(apperrors.CodePolicyInvalid, "Persisted policy is invalid.", false, true, true, err)
	}
	types := make([]intents.Type, len(row.IntentTypes))
	for i, kind := range row.IntentTypes {
		types[i] = intents.Type(kind)
	}
	lifecycleRevision, err := domainVersion(row.LifecycleVersion)
	if err != nil {
		return policies.Policy{}, err
	}
	return policies.Restore(policies.Params{PolicyID: row.PolicyID, Version: version, Name: row.Name, Scope: policies.Scope{UserID: row.UserID, WalletBindingID: domainOptionalString(row.WalletBindingID), IntentTypes: types}, Rules: rules, CreatedAt: domainTime(row.CreatedAt), ValidFrom: domainTime(row.ValidFrom), ExpiresAt: domainTime(row.ExpiresAt)}, policies.Status(row.Status), lifecycleRevision)
}
func equalPolicy(a, b policies.Policy) bool {
	return a.PolicyID() == b.PolicyID() && a.Version() == b.Version() && a.Name() == b.Name() && reflect.DeepEqual(a.Scope(), b.Scope()) && reflect.DeepEqual(a.Rules(), b.Rules()) && a.Status() == b.Status() && a.CreatedAt().Equal(b.CreatedAt()) && a.ValidFrom().Equal(b.ValidFrom()) && a.ExpiresAt().Equal(b.ExpiresAt()) && a.LifecycleRevision() == b.LifecycleRevision()
}

func equalPolicyResult(a, b policies.Result) bool {
	if a.PolicyID != b.PolicyID || a.PolicyVersion != b.PolicyVersion || a.IntentID != b.IntentID || a.IntentVersion != b.IntentVersion || a.IntentDigest != b.IntentDigest || a.Stage != b.Stage || a.Decision != b.Decision || !a.EvaluatedAt.Equal(b.EvaluatedAt) || len(a.Findings) != len(b.Findings) {
		return false
	}
	for index := range a.Findings {
		if a.Findings[index] != b.Findings[index] {
			return false
		}
	}
	return true
}
func (s *Store) CreatePolicy(ctx context.Context, scope storage.Scope, value policies.Policy) (storage.CreatePolicyResult, error) {
	if err := scope.Validate(); err != nil {
		return storage.CreatePolicyResult{}, err
	}
	if err := value.Validate(); err != nil {
		return storage.CreatePolicyResult{}, err
	}
	if value.Scope().UserID != scope.ActorID() {
		return storage.CreatePolicyResult{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Policy is not accessible.", false, true, true)
	}
	params, err := policyCreateParams(scope, value)
	if err != nil {
		return storage.CreatePolicyResult{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.CreatePolicyResult{}, err
	}
	defer cancel()
	row, err := s.queries.CreatePolicy(bounded, params)
	if err == nil {
		restored, err := policyFromRow(row)
		return storage.CreatePolicyResult{Policy: restored, Created: true}, err
	}
	mapped := mapDatabaseError(err)
	var appErr *apperrors.Error
	if !stderrors.As(mapped, &appErr) || appErr.Code != apperrors.CodeExecutionConflict {
		return storage.CreatePolicyResult{}, mapped
	}
	existing, findErr := s.FindPolicyByID(ctx, scope, value.PolicyID(), value.Version())
	if findErr != nil {
		return storage.CreatePolicyResult{}, mapped
	}
	if !equalPolicy(existing, value) {
		return storage.CreatePolicyResult{}, mapped
	}
	return storage.CreatePolicyResult{Policy: existing, Created: false}, nil
}
func (s *Store) FindPolicyByID(ctx context.Context, scope storage.Scope, id string, version uint64) (policies.Policy, error) {
	if err := scope.Validate(); err != nil {
		return policies.Policy{}, err
	}
	dbv, err := dbVersion(version)
	if err != nil {
		return policies.Policy{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return policies.Policy{}, err
	}
	defer cancel()
	row, err := s.queries.FindPolicyByID(bounded, dbsqlc.FindPolicyByIDParams{TenantID: scope.TenantID(), PolicyID: id, PolicyVersion: dbv, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return policies.Policy{}, notFound(apperrors.CodePolicyNotFound, "Policy is not accessible.")
	}
	if err != nil {
		return policies.Policy{}, mapDatabaseError(err)
	}
	return policyFromRow(row)
}
func (s *Store) FindApplicablePolicies(ctx context.Context, scope storage.Scope, input policies.Applicability) ([]policies.Policy, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if input.UserID != scope.ActorID() || input.UserID == "" || !input.IntentType.Valid() || input.EvaluatedAt.IsZero() {
		return nil, apperrors.New(apperrors.CodePolicyInvalid, "Policy applicability input is invalid.", false, true, true)
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	rows, err := s.queries.FindApplicablePolicies(bounded, dbsqlc.FindApplicablePoliciesParams{TenantID: scope.TenantID(), UserID: input.UserID, WalletBindingID: dbOptionalString(input.WalletBindingID), IntentType: string(input.IntentType), EvaluatedAt: dbTime(input.EvaluatedAt)})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	result := make([]policies.Policy, 0, len(rows))
	for _, row := range rows {
		value, err := policyFromRow(dbsqlc.Policy{TenantID: row.TenantID, PolicyID: row.PolicyID, PolicyVersion: row.PolicyVersion, Name: row.Name, UserID: row.UserID, WalletBindingID: row.WalletBindingID, IntentTypes: row.IntentTypes, Rules: row.Rules, Status: row.Status, CreatedAt: row.CreatedAt, ValidFrom: row.ValidFrom, ExpiresAt: row.ExpiresAt, LifecycleVersion: row.LifecycleVersion})
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}
func (s *Store) UpdatePolicy(ctx context.Context, scope storage.Scope, value policies.Policy, expectedRevision uint64) (policies.Policy, error) {
	if err := scope.Validate(); err != nil {
		return policies.Policy{}, err
	}
	if err := value.Validate(); err != nil {
		return policies.Policy{}, err
	}
	if value.Scope().UserID != scope.ActorID() {
		return policies.Policy{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Policy is not accessible.", false, true, true)
	}
	version, err := dbVersion(value.Version())
	if err != nil {
		return policies.Policy{}, err
	}
	revision, err := dbVersion(value.LifecycleRevision())
	if err != nil {
		return policies.Policy{}, err
	}
	expected, err := dbVersion(expectedRevision)
	if err != nil {
		return policies.Policy{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return policies.Policy{}, err
	}
	defer cancel()
	row, err := s.queries.UpdatePolicy(bounded, dbsqlc.UpdatePolicyParams{TenantID: scope.TenantID(), PolicyID: value.PolicyID(), PolicyVersion: version, Status: string(value.Status()), LifecycleVersion: revision, LifecycleVersion_2: expected, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return policies.Policy{}, apperrors.New(apperrors.CodeExecutionConflict, "Policy changed concurrently.", false, true, true)
	}
	if err != nil {
		return policies.Policy{}, mapDatabaseError(err)
	}
	return policyFromRow(row)
}

func (s *Store) CreatePolicyEvaluation(ctx context.Context, scope storage.Scope, result policies.Result) (policies.Result, error) {
	if err := scope.Validate(); err != nil {
		return policies.Result{}, err
	}
	if result.PolicyID == "" || result.PolicyVersion == 0 || result.IntentID == "" || result.IntentVersion == 0 || result.IntentDigest == "" || result.EvaluatedAt.IsZero() || !result.Decision.Valid() || (result.Stage != policies.EvaluationStageBeforeApproval && result.Stage != policies.EvaluationStageBeforeExecution) {
		return policies.Result{}, apperrors.New(apperrors.CodePolicyInvalid, "Policy evaluation is invalid.", false, true, true)
	}
	key := execution.PolicyEvaluationKey(result)
	policyVersion, _ := dbVersion(result.PolicyVersion)
	intentVersion, _ := dbVersion(result.IntentVersion)
	err := s.withTx(ctx, func(txctx context.Context, q *dbsqlc.Queries) error {
		_, err := q.CreatePolicyEvaluation(txctx, dbsqlc.CreatePolicyEvaluationParams{TenantID: scope.TenantID(), EvaluationKey: key, PolicyID: result.PolicyID, PolicyVersion: policyVersion, UserID: scope.ActorID(), IntentID: result.IntentID, IntentVersion: intentVersion, IntentDigest: result.IntentDigest, Stage: string(result.Stage), Decision: string(result.Decision), EvaluatedAt: dbTime(result.EvaluatedAt)})
		if err != nil {
			return err
		}
		for i, finding := range result.Findings {
			if err := q.CreatePolicyFinding(txctx, dbsqlc.CreatePolicyFindingParams{TenantID: scope.TenantID(), EvaluationKey: key, FindingIndex: int32(i), RuleID: finding.RuleID, RuleType: string(finding.RuleType), Decision: string(finding.Decision), Reason: string(finding.Reason)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if !isUniqueViolation(err) {
			return policies.Result{}, err
		}
		existing, findErr := s.FindPolicyEvaluation(ctx, scope, key)
		if findErr == nil && equalPolicyResult(existing, result) {
			return existing, nil
		}
		return policies.Result{}, apperrors.New(apperrors.CodeExecutionConflict, "Policy evaluation conflicts with existing evidence.", false, true, true)
	}
	return result, nil
}
func (s *Store) FindPolicyEvaluation(ctx context.Context, scope storage.Scope, key string) (policies.Result, error) {
	if err := scope.Validate(); err != nil {
		return policies.Result{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return policies.Result{}, err
	}
	defer cancel()
	row, err := s.queries.FindPolicyEvaluation(bounded, dbsqlc.FindPolicyEvaluationParams{TenantID: scope.TenantID(), EvaluationKey: key, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return policies.Result{}, notFound(apperrors.CodePolicyNotFound, "Policy evaluation is not accessible.")
	}
	if err != nil {
		return policies.Result{}, mapDatabaseError(err)
	}
	findings, err := s.queries.FindPolicyFindings(bounded, dbsqlc.FindPolicyFindingsParams{TenantID: scope.TenantID(), EvaluationKey: key})
	if err != nil {
		return policies.Result{}, mapDatabaseError(err)
	}
	result := policies.Result{PolicyID: row.PolicyID, PolicyVersion: uint64(row.PolicyVersion), IntentID: row.IntentID, IntentVersion: uint64(row.IntentVersion), IntentDigest: row.IntentDigest, Stage: policies.EvaluationStage(row.Stage), Decision: policies.Decision(row.Decision), EvaluatedAt: domainTime(row.EvaluatedAt)}
	for _, finding := range findings {
		result.Findings = append(result.Findings, policies.Finding{RuleID: finding.RuleID, RuleType: policies.RuleType(finding.RuleType), Decision: policies.Decision(finding.Decision), Reason: policies.Reason(finding.Reason)})
	}
	return result, nil
}

var _ storage.PolicyRepository = (*Store)(nil)
var _ storage.PolicyEvaluationRepository = (*Store)(nil)
