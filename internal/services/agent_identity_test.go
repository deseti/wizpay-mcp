package services

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"testing"
	"time"
)

// Any accidental repository call panics, proving denial happens before delegation lookup.
type noAgentRepositoryAccess struct{ storage.AutonomyRepository }

func TestScheduleCannotAssertAnotherAgent(t *testing.T) {
	for _, client := range []string{"verified-client", ""} {
		t.Run(client, func(t *testing.T) {
			p, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", IdentityProvider: "issuer", ProviderSubject: "subject", ClientID: client, ExpiresAt: time.Now().Add(time.Hour), Permissions: []auth.Permission{auth.PermissionAutonomyControl}})
			if err != nil {
				t.Fatal(err)
			}
			i, err := auth.NewIdentityWithSubject("user", "issuer", "subject", auth.IdentityStatusActive)
			if err != nil {
				t.Fatal(err)
			}
			r, err := auth.NewTrustedRequest(p, i, auth.RequestMetadata{RequestID: "request", ClientID: "asserted-agent"})
			if err != nil {
				t.Fatal(err)
			}
			s := PersistedAutonomyService{Repository: noAgentRepositoryAccess{}, Authorizer: auth.NewPermissionAuthorizer(), Now: time.Now}
			_, err = s.CreateSchedule(auth.WithTrustedRequest(context.Background(), r), autonomy.Schedule{Principal: autonomy.Principal{AgentID: "asserted-agent"}})
			if err == nil {
				t.Fatal("caller selected acting agent")
			}
		})
	}
}

// Other repository operations deliberately panic if reached after a denied agent check.
type anotherAgentScheduleRepository struct{ storage.AutonomyRepository }

func (anotherAgentScheduleRepository) LoadAutonomySchedule(context.Context, storage.Scope, string, uint64) (autonomy.Schedule, error) {
	return autonomy.Schedule{Principal: autonomy.Principal{AgentID: "other-agent"}}, nil
}

func TestScheduleStatusCannotControlAnotherAgent(t *testing.T) {
	for _, client := range []string{"verified-client", ""} {
		p, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant", ActorID: "user", IdentityProvider: "issuer", ProviderSubject: "subject", ClientID: client, ExpiresAt: time.Now().Add(time.Hour), Permissions: []auth.Permission{auth.PermissionAutonomyControl}})
		if err != nil {
			t.Fatal(err)
		}
		i, err := auth.NewIdentityWithSubject("user", "issuer", "subject", auth.IdentityStatusActive)
		if err != nil {
			t.Fatal(err)
		}
		r, err := auth.NewTrustedRequest(p, i, auth.RequestMetadata{RequestID: "request"})
		if err != nil {
			t.Fatal(err)
		}
		s := PersistedAutonomyService{Repository: anotherAgentScheduleRepository{}, Authorizer: auth.NewPermissionAuthorizer(), Now: time.Now}
		if _, err := s.SetScheduleStatus(auth.WithTrustedRequest(context.Background(), r), "schedule", 1, autonomy.ScheduleActive); err == nil {
			t.Fatal("caller controlled another acting agent")
		}
	}
}
