package config

import "testing"

func TestProductionSIWERequiresExplicitSecureConfiguration(t *testing.T) {
	for _, environment := range []string{"staging", "production"} {
		t.Run(environment, func(t *testing.T) {
			values := map[string]string{"APP_ENV": environment, "AUTH_REQUIRED": "true", "OAUTH_ENABLED": "true", "WIZPAY_SIWE_ENABLED": "true", "WIZPAY_SIWE_ORIGIN": "https://connect.wizpay.xyz", "WIZPAY_ONBOARDING_TENANT_ID": "reviewed-tenant", "WIZPAY_AUTONOMY_ENABLED": "false", "DATABASE_MIGRATIONS_ON_START": "false"}
			load := func() error {
				_, e := LoadWithLookup(func(k string) (string, bool) { v, ok := values[k]; return v, ok })
				return e
			}
			if e := load(); e != nil {
				t.Fatal(e)
			}
			for _, key := range []string{"WIZPAY_SIWE_ORIGIN", "WIZPAY_ONBOARDING_TENANT_ID"} {
				v := values[key]
				delete(values, key)
				if load() == nil {
					t.Fatal("missing prerequisite accepted", key)
				}
				values[key] = v
			}
			values["WIZPAY_SIWE_ENABLED"] = "false"
			if e := load(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
