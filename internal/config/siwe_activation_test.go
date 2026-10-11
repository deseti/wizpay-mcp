package config

import "testing"

func TestRealWalletActivationCannotBeEnabledByFlags(t *testing.T) {
	for _, environment := range []string{"staging", "production"} {
		t.Run(environment, func(t *testing.T) {
			values := map[string]string{"APP_ENV": environment, "AUTH_REQUIRED": "true", "OAUTH_ENABLED": "true", "WIZPAY_SIWE_ENABLED": "true", "WIZPAY_SIWE_ORIGIN": "https://connect.wizpay.xyz", "WIZPAY_ONBOARDING_TENANT_ID": "reviewed-tenant", "WIZPAY_AUTONOMY_ENABLED": "false", "DATABASE_MIGRATIONS_ON_START": "false", "ONBOARDING_ADMIN_APPROVED": "yes"}
			if _, err := LoadWithLookup(func(k string) (string, bool) { v, ok := values[k]; return v, ok }); err == nil {
				t.Fatal("production enrollment policy bypassed")
			}
			values["WIZPAY_SIWE_ENABLED"] = "false"
			if _, err := LoadWithLookup(func(k string) (string, bool) { v, ok := values[k]; return v, ok }); err != nil {
				t.Fatal("disabled profile rejected", err)
			}
		})
	}
}
