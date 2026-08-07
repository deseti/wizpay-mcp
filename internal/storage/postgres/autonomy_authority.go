package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/jackc/pgx/v5"
)

func (s *Store) SaveAutonomyGrant(ctx context.Context, scope storage.Scope, g autonomy.Grant) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if err := g.Validate(); err != nil {
		return err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	tag, err := s.pool.Exec(b, `INSERT INTO autonomy_grants(tenant_id,grant_id,version,principal_user_id,wallet_binding_id,intent_type,schedule_id,expires_at,paused,revoked,per_action_base_units,aggregate_cap_base_units,rolling_cap_base_units,rolling_window_seconds,step_up_base_units,allowed_recipients,allowed_tokens,allowed_chains) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) ON CONFLICT(tenant_id,grant_id,version) DO NOTHING`, scope.TenantID(), g.ID, g.Version, g.PrincipalUserID, g.WalletBindingID, string(g.Intent), nullableString(g.ScheduleID), g.ExpiresAt.UTC(), g.Paused, g.Revoked, nullableString(g.PerActionBaseUnits), nullableString(g.AggregateCapBaseUnits), nullableString(g.RollingWindowCapBaseUnits), nullableDuration(g.RollingWindow), nullableString(g.StepUpAboveBaseUnits), emptyStrings(g.AllowedRecipients), emptyStrings(g.AllowedTokens), emptyStrings(g.AllowedChains))
	if err := mapDatabaseError(err); err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var principal, wallet, intent string
		var schedule, per, agg, roll, step *string
		var expires time.Time
		var paused, revoked bool
		var window *int64
		var recipients, tokens, chains []string
		if err := s.pool.QueryRow(b, `SELECT principal_user_id,wallet_binding_id,intent_type,schedule_id,expires_at,paused,revoked,per_action_base_units::text,aggregate_cap_base_units::text,rolling_cap_base_units::text,rolling_window_seconds,step_up_base_units::text,allowed_recipients,allowed_tokens,allowed_chains FROM autonomy_grants WHERE tenant_id=$1 AND grant_id=$2 AND version=$3`, scope.TenantID(), g.ID, g.Version).Scan(&principal, &wallet, &intent, &schedule, &expires, &paused, &revoked, &per, &agg, &roll, &window, &step, &recipients, &tokens, &chains); err != nil {
			return mapDatabaseError(err)
		}
		same := principal == g.PrincipalUserID && wallet == g.WalletBindingID && intent == string(g.Intent) && stringValue(schedule) == g.ScheduleID && expires.Equal(g.ExpiresAt) && paused == g.Paused && revoked == g.Revoked && stringValue(per) == g.PerActionBaseUnits && stringValue(agg) == g.AggregateCapBaseUnits && stringValue(roll) == g.RollingWindowCapBaseUnits && stringValue(step) == g.StepUpAboveBaseUnits && reflect.DeepEqual(recipients, emptyStrings(g.AllowedRecipients)) && reflect.DeepEqual(tokens, emptyStrings(g.AllowedTokens)) && reflect.DeepEqual(chains, emptyStrings(g.AllowedChains))
		if window != nil {
			same = same && time.Duration(*window)*time.Second == g.RollingWindow
		} else {
			same = same && g.RollingWindow == 0
		}
		if same {
			return nil
		}
		return fmt.Errorf("grant immutable conflict")
	}
	return nil
}
func nullableDuration(d time.Duration) any {
	if d <= 0 {
		return nil
	}
	return int64(d / time.Second)
}
func emptyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
func (s *Store) LoadAutonomyGrant(ctx context.Context, scope storage.Scope, id string, version uint64) (autonomy.Grant, error) {
	if err := scope.Validate(); err != nil {
		return autonomy.Grant{}, err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return autonomy.Grant{}, err
	}
	defer cancel()
	var g autonomy.Grant
	var v int64
	var intent string
	var schedule *string
	var paused, revoked bool
	var per, agg, roll, step *string
	var window *int64
	err = s.pool.QueryRow(b, `SELECT grant_id,version,principal_user_id,wallet_binding_id,intent_type,schedule_id,expires_at,paused,revoked,per_action_base_units::text,aggregate_cap_base_units::text,rolling_cap_base_units::text,rolling_window_seconds,step_up_base_units::text,allowed_recipients,allowed_tokens,allowed_chains FROM autonomy_grants WHERE tenant_id=$1 AND grant_id=$2 AND version=$3 AND principal_user_id=$4`, scope.TenantID(), id, version, scope.ActorID()).Scan(&g.ID, &v, &g.PrincipalUserID, &g.WalletBindingID, &intent, &schedule, &g.ExpiresAt, &paused, &revoked, &per, &agg, &roll, &window, &step, &g.AllowedRecipients, &g.AllowedTokens, &g.AllowedChains)
	if errors.Is(err, pgx.ErrNoRows) {
		return autonomy.Grant{}, fmt.Errorf("grant not found")
	}
	if err != nil {
		return autonomy.Grant{}, mapDatabaseError(err)
	}
	g.Version = uint64(v)
	g.Intent = autonomy.IntentType(intent)
	g.ScheduleID = stringValue(schedule)
	g.Paused = paused
	g.Revoked = revoked
	g.PerActionBaseUnits = stringValue(per)
	g.AggregateCapBaseUnits = stringValue(agg)
	g.RollingWindowCapBaseUnits = stringValue(roll)
	g.StepUpAboveBaseUnits = stringValue(step)
	if window != nil {
		g.RollingWindow = time.Duration(*window) * time.Second
	}
	if err := g.Validate(); err != nil {
		return autonomy.Grant{}, fmt.Errorf("restore grant: %w", err)
	}
	return g, nil
}

func (s *Store) SaveAutonomyDelegation(ctx context.Context, scope storage.Scope, d autonomy.Delegation) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if err := d.ValidateStructure(); err != nil {
		return err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	capabilities := make([]string, len(d.Capabilities))
	for i, v := range d.Capabilities {
		capabilities[i] = string(v)
	}
	tag, err := s.pool.Exec(b, `INSERT INTO autonomy_delegations(tenant_id,delegation_id,version,principal_user_id,agent_id,capabilities,expires_at,revoked,non_transitive) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(tenant_id,delegation_id,version) DO NOTHING`, scope.TenantID(), d.ID, d.Version, d.PrincipalUserID, d.AgentID, capabilities, d.ExpiresAt.UTC(), d.Revoked, d.NonTransitive)
	if err := mapDatabaseError(err); err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var principal, agent string
		var existing []string
		var expires time.Time
		var revoked, nonTransitive bool
		if err := s.pool.QueryRow(b, `SELECT principal_user_id,agent_id,capabilities,expires_at,revoked,non_transitive FROM autonomy_delegations WHERE tenant_id=$1 AND delegation_id=$2 AND version=$3`, scope.TenantID(), d.ID, d.Version).Scan(&principal, &agent, &existing, &expires, &revoked, &nonTransitive); err != nil {
			return mapDatabaseError(err)
		}
		if principal == d.PrincipalUserID && agent == d.AgentID && reflect.DeepEqual(existing, capabilities) && expires.Equal(d.ExpiresAt) && revoked == d.Revoked && nonTransitive == d.NonTransitive {
			return nil
		}
		return fmt.Errorf("delegation immutable conflict")
	}
	return nil
}
func (s *Store) LoadAutonomyDelegation(ctx context.Context, scope storage.Scope, id string, version uint64) (autonomy.Delegation, error) {
	if err := scope.Validate(); err != nil {
		return autonomy.Delegation{}, err
	}
	b, cancel, err := s.queryContext(ctx)
	if err != nil {
		return autonomy.Delegation{}, err
	}
	defer cancel()
	var d autonomy.Delegation
	var capabilities []string
	err = s.pool.QueryRow(b, `SELECT delegation_id,version,principal_user_id,agent_id,capabilities,expires_at,revoked,non_transitive FROM autonomy_delegations WHERE tenant_id=$1 AND delegation_id=$2 AND version=$3 AND principal_user_id=$4`, scope.TenantID(), id, version, scope.ActorID()).Scan(&d.ID, &d.Version, &d.PrincipalUserID, &d.AgentID, &capabilities, &d.ExpiresAt, &d.Revoked, &d.NonTransitive)
	if errors.Is(err, pgx.ErrNoRows) {
		return autonomy.Delegation{}, fmt.Errorf("delegation not found")
	}
	if err != nil {
		return autonomy.Delegation{}, mapDatabaseError(err)
	}
	for _, v := range capabilities {
		d.Capabilities = append(d.Capabilities, autonomy.IntentType(v))
	}
	return d, nil
}
