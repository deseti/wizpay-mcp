package postgres

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

func (s *Store) ListAutonomySchedules(ctx context.Context, scope storage.Scope) ([]autonomy.Schedule, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	rows, err := s.pool.Query(b, `SELECT schedule_id, version FROM autonomy_schedules WHERE tenant_id=$1 AND user_id=$2 ORDER BY schedule_id, version`, scope.TenantID(), scope.ActorID())
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	defer rows.Close()
	var result []autonomy.Schedule
	for rows.Next() {
		var id string
		var version int64
		if err := rows.Scan(&id, &version); err != nil {
			return nil, mapDatabaseError(err)
		}
		value, err := s.LoadAutonomySchedule(ctx, scope, id, uint64(version))
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDatabaseError(err)
	}
	return result, nil
}

func (s *Store) SaveAutonomySchedule(ctx context.Context, scope storage.Scope, value autonomy.Schedule) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	tag, err := s.pool.Exec(b, `INSERT INTO autonomy_schedules
	(tenant_id,schedule_id,version,user_id,agent_id,wallet_binding_id,wallet_binding_version,intent_type,grant_id,grant_version,delegation_id,delegation_version,status,spec_digest,schedule_digest,timezone,start_at,end_at,max_recipients,recurrence,missed_run,concurrency,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
	ON CONFLICT (tenant_id,schedule_id,version) DO NOTHING`, scope.TenantID(), value.ID, value.Version, value.Principal.UserID, value.Principal.AgentID,
		value.WalletBindingID, value.WalletBindingVersion, string(value.Spec.Intent), value.GrantID, value.GrantVersion, value.DelegationID, value.DelegationVersion, string(value.Status), value.Spec.TemplateDigest, value.Digest,
		value.Spec.Recurrence.Location, value.Spec.Recurrence.Start.UTC(), nullableTime(value.Spec.Recurrence.End), value.Spec.MaxRecipients, string(value.Spec.Recurrence.Frequency), string(value.Spec.Missed), string(value.Spec.Concurrency), value.CreatedAt.UTC(), value.UpdatedAt.UTC())
	if err := mapDatabaseError(err); err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var existingDigest, existingStatus string
		if err := s.pool.QueryRow(b, `SELECT schedule_digest,status FROM autonomy_schedules WHERE tenant_id=$1 AND schedule_id=$2 AND version=$3`, scope.TenantID(), value.ID, value.Version).Scan(&existingDigest, &existingStatus); err != nil {
			return mapDatabaseError(err)
		}
		if existingDigest == value.Digest && existingStatus == string(value.Status) {
			return nil
		}
		return fmt.Errorf("schedule immutable conflict")
	}
	return nil
}

func (s *Store) LoadAutonomySchedule(ctx context.Context, scope storage.Scope, id string, version uint64) (autonomy.Schedule, error) {
	if err := scope.Validate(); err != nil {
		return autonomy.Schedule{}, err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return autonomy.Schedule{}, err
	}
	defer cancel()
	var v int64
	if version > uint64(^uint64(0)>>1) {
		return autonomy.Schedule{}, fmt.Errorf("version is outside PostgreSQL range")
	}
	v = int64(version)
	var value autonomy.Schedule
	var status, intentType, location, recurrence, missed, concurrency, templateDigest, digest string
	var end *time.Time
	var maxRecipients int
	var start, created, updated time.Time
	var walletVersion int64
	var grantVersion, delegationVersion int64
	err = s.pool.QueryRow(b, `SELECT schedule_id,version,user_id,agent_id,wallet_binding_id,wallet_binding_version,intent_type,grant_id,grant_version,delegation_id,delegation_version,status,spec_digest,schedule_digest,timezone,start_at,end_at,max_recipients,recurrence,missed_run,concurrency,created_at,updated_at FROM autonomy_schedules WHERE tenant_id=$1 AND schedule_id=$2 AND version=$3 AND user_id=$4`, scope.TenantID(), id, v, scope.ActorID()).Scan(&value.ID, &v, &value.Principal.UserID, &value.Principal.AgentID, &value.WalletBindingID, &walletVersion, &intentType, &value.GrantID, &grantVersion, &value.DelegationID, &delegationVersion, &status, &templateDigest, &digest, &location, &start, &end, &maxRecipients, &recurrence, &missed, &concurrency, &created, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return autonomy.Schedule{}, notFound(apperrors.CodeAuthorizationRequired, "Schedule is not accessible.")
	}
	if err != nil {
		return autonomy.Schedule{}, mapDatabaseError(err)
	}
	value.Version = uint64(v)
	value.Principal.TenantID = scope.TenantID()
	value.WalletBindingVersion = uint64(walletVersion)
	value.GrantVersion = uint64(grantVersion)
	value.DelegationVersion = uint64(delegationVersion)
	loc, locErr := time.LoadLocation(location)
	if locErr != nil {
		return autonomy.Schedule{}, fmt.Errorf("restore schedule timezone: %w", locErr)
	}
	start = start.In(loc)
	value.Spec = autonomy.ScheduleSpec{Intent: autonomy.IntentType(intentType), TemplateDigest: templateDigest, Missed: autonomy.MissedRunPolicy(missed), Concurrency: autonomy.ConcurrencyPolicy(concurrency), MaxRecipients: maxRecipients, Recurrence: autonomy.Recurrence{Frequency: autonomy.Frequency(recurrence), Start: start, Location: location}}
	if end != nil {
		value.Spec.Recurrence.End = end.In(loc)
	}
	value.Status = autonomy.ScheduleStatus(status)
	value.Digest = digest
	value.CreatedAt = created
	value.UpdatedAt = updated
	if err := value.Validate(); err != nil {
		return autonomy.Schedule{}, fmt.Errorf("restore autonomy schedule: %w", err)
	}
	return value, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func (s *Store) SaveAutonomyOccurrence(ctx context.Context, scope storage.Scope, value autonomy.Occurrence) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if value.ID == "" || value.Key == "" || value.ScheduleID == "" || value.ScheduleVersion == 0 {
		return fmt.Errorf("invalid occurrence")
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	if err := value.Validate(); err != nil {
		return err
	}
	tag, err := s.pool.Exec(b, `INSERT INTO autonomy_occurrences(tenant_id,occurrence_id,occurrence_key,schedule_id,schedule_version,schedule_digest,scheduled_at,status,grant_id,grant_version,intent_id,approval_id,execution_id,lease_owner,lease_until,fence,reason_code,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) ON CONFLICT(tenant_id,occurrence_key) DO NOTHING`, scope.TenantID(), value.ID, value.Key, value.ScheduleID, value.ScheduleVersion, value.ScheduleDigest, value.ScheduledAt.UTC(), string(value.Status), value.GrantID, value.GrantVersion, nullableString(value.IntentID), nullableString(value.ApprovalID), nullableString(value.ExecutionID), nullableString(value.LeaseOwner), nullableTime(value.LeaseUntil), value.Fence, string(value.Reason), value.CreatedAt.UTC(), time.Now().UTC())
	if err := mapDatabaseError(err); err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var digest, grantID string
		var grantVersion int64
		if err := s.pool.QueryRow(b, `SELECT schedule_digest,grant_id,grant_version FROM autonomy_occurrences WHERE tenant_id=$1 AND occurrence_key=$2`, scope.TenantID(), value.Key).Scan(&digest, &grantID, &grantVersion); err != nil {
			return mapDatabaseError(err)
		}
		if digest == value.ScheduleDigest && grantID == value.GrantID && uint64(grantVersion) == value.GrantVersion {
			return nil
		}
		return fmt.Errorf("occurrence immutable conflict")
	}
	return nil
}
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) LoadAutonomyOccurrence(ctx context.Context, scope storage.Scope, id string) (autonomy.Occurrence, error) {
	if err := scope.Validate(); err != nil {
		return autonomy.Occurrence{}, err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return autonomy.Occurrence{}, err
	}
	defer cancel()
	var o autonomy.Occurrence
	var status, reason string
	var fence int64
	var scheduleVersion int64
	var intentID, approvalID, executionID, leaseOwner *string
	var grantVersion int64
	var leaseUntil *time.Time
	err = s.pool.QueryRow(b, `SELECT o.occurrence_id,o.occurrence_key,o.schedule_id,o.schedule_version,o.schedule_digest,o.scheduled_at,o.status,o.grant_id,o.grant_version,o.intent_id,o.approval_id,o.execution_id,o.lease_owner,o.lease_until,o.fence,o.reason_code,o.created_at FROM autonomy_occurrences o JOIN autonomy_schedules s ON s.tenant_id=o.tenant_id AND s.schedule_id=o.schedule_id AND s.version=o.schedule_version WHERE o.tenant_id=$1 AND o.occurrence_id=$2 AND s.user_id=$3`, scope.TenantID(), id, scope.ActorID()).Scan(&o.ID, &o.Key, &o.ScheduleID, &scheduleVersion, &o.ScheduleDigest, &o.ScheduledAt, &status, &o.GrantID, &grantVersion, &intentID, &approvalID, &executionID, &leaseOwner, &leaseUntil, &fence, &reason, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return autonomy.Occurrence{}, fmt.Errorf("occurrence not found")
	}
	if err != nil {
		return autonomy.Occurrence{}, mapDatabaseError(err)
	}
	o.ScheduleVersion = uint64(scheduleVersion)
	o.GrantVersion = uint64(grantVersion)
	o.Status = autonomy.OccurrenceStatus(status)
	o.IntentID = stringValue(intentID)
	o.ApprovalID = stringValue(approvalID)
	o.ExecutionID = stringValue(executionID)
	o.LeaseOwner = stringValue(leaseOwner)
	if leaseUntil != nil {
		o.LeaseUntil = *leaseUntil
	}
	o.Fence = uint64(fence)
	o.Reason = autonomy.ReasonCode(reason)
	if err := o.Validate(); err != nil {
		return autonomy.Occurrence{}, fmt.Errorf("restore autonomy occurrence: %w", err)
	}
	return o, nil
}
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Store) ClaimAutonomyDue(ctx context.Context, scope storage.Scope, now time.Time, worker string, lease time.Duration) (autonomy.Occurrence, bool, error) {
	if err := scope.Validate(); err != nil {
		return autonomy.Occurrence{}, false, err
	}
	if worker == "" || lease <= 0 {
		return autonomy.Occurrence{}, false, fmt.Errorf("worker and lease are required")
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return autonomy.Occurrence{}, false, err
	}
	defer cancel()
	// Row locking plus SKIP LOCKED provides the claim winner semantics; using
	// the default isolation avoids turning independent users' due scans into a
	// spurious serialization conflict.
	tx, err := s.pool.BeginTx(b, pgx.TxOptions{})
	if err != nil {
		return autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	defer tx.Rollback(context.Background())
	var id string
	err = tx.QueryRow(b, `SELECT o.occurrence_id FROM autonomy_occurrences o JOIN autonomy_schedules s ON s.tenant_id=o.tenant_id AND s.schedule_id=o.schedule_id AND s.version=o.schedule_version WHERE o.tenant_id=$1 AND s.user_id=$2 AND s.status='ACTIVE' AND o.scheduled_at <= $3 AND o.status IN ('DUE','CLAIMED') AND (o.lease_until IS NULL OR o.lease_until <= $3) AND NOT EXISTS (SELECT 1 FROM autonomy_occurrences active WHERE active.tenant_id=o.tenant_id AND active.schedule_id=o.schedule_id AND active.status IN ('CLAIMED','DISPATCHED','RECONCILIATION_ONLY') AND active.occurrence_id <> o.occurrence_id) ORDER BY o.scheduled_at,o.occurrence_id FOR UPDATE OF s, o SKIP LOCKED LIMIT 1`, scope.TenantID(), scope.ActorID(), now.UTC()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return autonomy.Occurrence{}, false, nil
	}
	if err != nil {
		return autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	var fence int64
	err = tx.QueryRow(b, `UPDATE autonomy_occurrences SET status='CLAIMED',lease_owner=$1,lease_until=$2,fence=fence+1,updated_at=$2 WHERE tenant_id=$3 AND occurrence_id=$4 RETURNING fence`, worker, now.UTC().Add(lease), scope.TenantID(), id).Scan(&fence)
	if err != nil {
		return autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	if err := tx.Commit(b); err != nil {
		return autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	o, err := s.LoadAutonomyOccurrence(ctx, scope, id)
	if err != nil {
		return autonomy.Occurrence{}, false, err
	}
	o.Fence = uint64(fence)
	return o, true, nil
}

func (s *Store) ClaimNextAutonomyDue(ctx context.Context, worker string, now time.Time, lease time.Duration) (storage.Scope, autonomy.Occurrence, bool, error) {
	if worker == "" || lease <= 0 {
		return storage.Scope{}, autonomy.Occurrence{}, false, fmt.Errorf("worker and lease are required")
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.Scope{}, autonomy.Occurrence{}, false, err
	}
	defer cancel()
	var tenant, user, id string
	// The selected row is locked and the update occurs in this same
	// transaction. The returned scope is therefore derived from the claimed
	// row, never from a second selection.
	tx, err := s.pool.BeginTx(b, pgx.TxOptions{})
	if err != nil {
		return storage.Scope{}, autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	defer tx.Rollback(context.Background())
	err = tx.QueryRow(b, `SELECT o.tenant_id,s.user_id,o.occurrence_id FROM autonomy_occurrences o JOIN autonomy_schedules s ON s.tenant_id=o.tenant_id AND s.schedule_id=o.schedule_id AND s.version=o.schedule_version WHERE s.status='ACTIVE' AND o.scheduled_at <= $1 AND o.status IN ('DUE','CLAIMED') AND (o.lease_until IS NULL OR o.lease_until <= $1) AND NOT EXISTS (SELECT 1 FROM autonomy_occurrences active WHERE active.tenant_id=o.tenant_id AND active.schedule_id=o.schedule_id AND active.status IN ('CLAIMED','DISPATCHED','RECONCILIATION_ONLY') AND active.occurrence_id <> o.occurrence_id) ORDER BY o.scheduled_at,o.occurrence_id FOR UPDATE OF s, o SKIP LOCKED LIMIT 1`, now.UTC()).Scan(&tenant, &user, &id)
	if errors.Is(err, pgx.ErrNoRows) {
		return storage.Scope{}, autonomy.Occurrence{}, false, nil
	}
	if err != nil {
		return storage.Scope{}, autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	scope, err := storage.NewScope(tenant, user, "autonomy-"+worker, "")
	if err != nil {
		return storage.Scope{}, autonomy.Occurrence{}, false, err
	}
	var fence int64
	err = tx.QueryRow(b, `UPDATE autonomy_occurrences SET status='CLAIMED',lease_owner=$1,lease_until=$2,fence=fence+1,updated_at=$2 WHERE tenant_id=$3 AND occurrence_id=$4 RETURNING fence`, worker, now.UTC().Add(lease), tenant, id).Scan(&fence)
	if err != nil {
		return storage.Scope{}, autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	if err := tx.Commit(b); err != nil {
		return storage.Scope{}, autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	value, err := s.LoadAutonomyOccurrence(ctx, scope, id)
	if err != nil {
		return storage.Scope{}, autonomy.Occurrence{}, false, err
	}
	value.Fence = uint64(fence)
	return scope, value, true, nil
}
func (s *Store) BlockAutonomyOccurrence(ctx context.Context, scope storage.Scope, value autonomy.Occurrence, reason autonomy.ReasonCode) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	return s.withTimeout(ctx, func(b context.Context) error {
		_, err := s.pool.Exec(b, `UPDATE autonomy_occurrences o SET status='BLOCKED',reason_code=$1,lease_owner=NULL,lease_until=NULL,updated_at=$2 FROM autonomy_schedules s WHERE o.tenant_id=s.tenant_id AND o.schedule_id=s.schedule_id AND o.schedule_version=s.version AND s.user_id=$3 AND o.tenant_id=$4 AND o.occurrence_id=$5 AND o.fence=$6`, string(reason), time.Now().UTC(), scope.ActorID(), scope.TenantID(), value.ID, value.Fence)
		return err
	})
}
func (s *Store) BindAutonomyApproval(ctx context.Context, scope storage.Scope, value autonomy.Occurrence, approvalID string) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if approvalID == "" {
		return fmt.Errorf("approval ID is required")
	}
	return s.withTimeout(ctx, func(b context.Context) error {
		_, err := s.pool.Exec(b, `UPDATE autonomy_occurrences o SET approval_id=$1,status='APPROVAL_REQUIRED',updated_at=$2 FROM autonomy_schedules s WHERE o.tenant_id=s.tenant_id AND o.schedule_id=s.schedule_id AND o.schedule_version=s.version AND s.user_id=$3 AND o.tenant_id=$4 AND o.occurrence_id=$5 AND o.fence=$6 AND o.approval_id IS NULL`, approvalID, time.Now().UTC(), scope.ActorID(), scope.TenantID(), value.ID, value.Fence)
		return err
	})
}
func (s *Store) CheckAutonomyDispatchFence(ctx context.Context, scope storage.Scope, value autonomy.Occurrence, worker string, fence uint64, at time.Time) (autonomy.Occurrence, bool, error) {
	if err := scope.Validate(); err != nil {
		return autonomy.Occurrence{}, false, err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return autonomy.Occurrence{}, false, err
	}
	defer cancel()
	tx, err := s.pool.BeginTx(b, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	defer tx.Rollback(context.Background())
	result, err := tx.Exec(b, `UPDATE autonomy_occurrences o SET updated_at=o.updated_at FROM autonomy_schedules s WHERE o.tenant_id=s.tenant_id AND o.schedule_id=s.schedule_id AND o.schedule_version=s.version AND s.user_id=$1 AND o.tenant_id=$2 AND o.occurrence_id=$3 AND o.status='CLAIMED' AND o.lease_owner=$4 AND o.fence=$5 AND o.lease_until>$6`, scope.ActorID(), scope.TenantID(), value.ID, worker, fence, at.UTC())
	if err != nil {
		return autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	if result.RowsAffected() != 1 {
		if err := tx.Commit(b); err != nil {
			return autonomy.Occurrence{}, false, mapDatabaseError(err)
		}
		current, loadErr := s.LoadAutonomyOccurrence(ctx, scope, value.ID)
		if loadErr != nil {
			return autonomy.Occurrence{}, false, nil
		}
		return current, false, nil
	}
	if err := tx.Commit(b); err != nil {
		return autonomy.Occurrence{}, false, mapDatabaseError(err)
	}
	current, err := s.LoadAutonomyOccurrence(ctx, scope, value.ID)
	if err != nil {
		return autonomy.Occurrence{}, false, err
	}
	return current, true, nil
}

func (s *Store) ReserveAutonomySpend(ctx context.Context, scope storage.Scope, grant autonomy.Grant, occurrence, amount string, at time.Time) (bool, error) {
	if err := scope.Validate(); err != nil {
		return false, err
	}
	if err := grant.Validate(); err != nil {
		return false, err
	}
	z, ok := new(big.Int).SetString(amount, 10)
	if !ok || z.Sign() <= 0 {
		return false, fmt.Errorf("invalid spend amount")
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	tx, err := s.pool.BeginTx(b, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, mapDatabaseError(err)
	}
	defer tx.Rollback(context.Background())
	var existing string
	err = tx.QueryRow(b, `SELECT amount_base_units::text FROM autonomy_spend_reservations WHERE tenant_id=$1 AND grant_id=$2 AND grant_version=$3 AND occurrence_id=$4`, scope.TenantID(), grant.ID, grant.Version, occurrence).Scan(&existing)
	if err == nil {
		return existing == amount, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, mapDatabaseError(err)
	}
	var total string
	err = tx.QueryRow(b, `SELECT COALESCE(SUM(amount_base_units),0)::text FROM autonomy_spend_reservations WHERE tenant_id=$1 AND grant_id=$2 AND grant_version=$3 AND state IN ('RESERVED','COMMITTED')`, scope.TenantID(), grant.ID, grant.Version).Scan(&total)
	if err != nil {
		return false, mapDatabaseError(err)
	}
	if grant.AggregateCapBaseUnits != "" {
		sum, _ := new(big.Int).SetString(total, 10)
		sum.Add(sum, z)
		cap, _ := new(big.Int).SetString(grant.AggregateCapBaseUnits, 10)
		if sum.Cmp(cap) > 0 {
			return false, nil
		}
	}
	if grant.RollingWindowCapBaseUnits != "" {
		var window string
		if err := tx.QueryRow(b, `SELECT COALESCE(SUM(amount_base_units),0)::text FROM autonomy_spend_reservations WHERE tenant_id=$1 AND grant_id=$2 AND grant_version=$3 AND reserved_at >= $4 AND state IN ('RESERVED','COMMITTED')`, scope.TenantID(), grant.ID, grant.Version, at.Add(-grant.RollingWindow)).Scan(&window); err != nil {
			return false, mapDatabaseError(err)
		}
		sum, _ := new(big.Int).SetString(window, 10)
		sum.Add(sum, z)
		cap, _ := new(big.Int).SetString(grant.RollingWindowCapBaseUnits, 10)
		if sum.Cmp(cap) > 0 {
			return false, nil
		}
	}
	_, err = tx.Exec(b, `INSERT INTO autonomy_spend_reservations(tenant_id,grant_id,grant_version,occurrence_id,amount_base_units,reserved_at,state) VALUES($1,$2,$3,$4,$5,$6,'RESERVED')`, scope.TenantID(), grant.ID, grant.Version, occurrence, amount, at.UTC())
	if err != nil {
		return false, mapDatabaseError(err)
	}
	if err := tx.Commit(b); err != nil {
		return false, mapDatabaseError(err)
	}
	return true, nil
}
func (s *Store) ReleaseAutonomySpend(ctx context.Context, scope storage.Scope, occurrence string) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	return s.withTimeout(ctx, func(b context.Context) error {
		_, err := s.pool.Exec(b, `UPDATE autonomy_spend_reservations SET state='RELEASED' WHERE tenant_id=$1 AND occurrence_id=$2 AND state='RESERVED'`, scope.TenantID(), occurrence)
		return err
	})
}
func (s *Store) LoadAutonomyEmergencyStop(ctx context.Context, scope storage.Scope) (autonomy.EmergencyStop, error) {
	if err := scope.Validate(); err != nil {
		return autonomy.EmergencyStop{}, err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return autonomy.EmergencyStop{}, err
	}
	defer cancel()
	var e autonomy.EmergencyStop
	err = s.pool.QueryRow(b, `SELECT active,scope,actor_id,reason_code,changed_at FROM autonomy_emergency_stop WHERE tenant_id=$1`, scope.TenantID()).Scan(&e.Active, &e.Scope, &e.Actor, &e.Reason, &e.ChangedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return autonomy.EmergencyStop{}, nil
	}
	if err != nil {
		return autonomy.EmergencyStop{}, mapDatabaseError(err)
	}
	if err := e.Validate(); err != nil {
		return autonomy.EmergencyStop{}, fmt.Errorf("malformed emergency stop state: %w", err)
	}
	return e, nil
}
func (s *Store) SetAutonomyEmergencyStop(ctx context.Context, scope storage.Scope, e autonomy.EmergencyStop) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if err := e.Validate(); err != nil {
		return err
	}
	return s.withTimeout(ctx, func(b context.Context) error {
		_, err := s.pool.Exec(b, `INSERT INTO autonomy_emergency_stop(tenant_id,active,scope,actor_id,reason_code,changed_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id) DO UPDATE SET active=EXCLUDED.active,scope=EXCLUDED.scope,actor_id=EXCLUDED.actor_id,reason_code=EXCLUDED.reason_code,changed_at=EXCLUDED.changed_at`, scope.TenantID(), e.Active, e.Scope, e.Actor, e.Reason, e.ChangedAt.UTC())
		return err
	})
}
