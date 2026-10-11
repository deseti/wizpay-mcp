package onboarding

import (
	"testing"
	"time"
)

func pending(now time.Time) Request {
	// Public metadata only; no generated signing key or verification assertion.
	return Request{Action: "STAGE", RequestID: "request-1", TenantID: "tenant-1", UserID: "user-1", BindingID: "binding-1", Issuer: "reviewed-issuer", Subject: "opaque-subject", Address: "0x0000000000000000000000000000000000000001", ReviewReference: "review-1", ExpiresAt: now.Add(time.Minute)}
}
func TestPendingValidation(t *testing.T) {
	now := time.Now().UTC()
	valid := pending(now)
	if err := valid.Validate(now); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Request){
		"activation":              func(r *Request) { r.Action = "ACTIVATE" },
		"verify":                  func(r *Request) { r.Action = "VERIFY" },
		"missing review":          func(r *Request) { r.ReviewReference = "" },
		"credential in reference": func(r *Request) { r.ReviewReference = "Bearer secret value" },
		"expiry":                  func(r *Request) { r.ExpiresAt = now },
		"unbounded":               func(r *Request) { r.ExpiresAt = now.Add(16 * time.Minute) },
		"no tenant":               func(r *Request) { r.TenantID = "" },
		"address":                 func(r *Request) { r.Address = "invalid" },
		"no identity":             func(r *Request) { r.Subject = "" },
	} {
		t.Run(name, func(t *testing.T) {
			r := valid
			change(&r)
			if r.Validate(now) == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}
func TestRevocationHasExactTarget(t *testing.T) {
	now := time.Now().UTC()
	for _, action := range []string{"REVOKE_TENANT", "REVOKE_IDENTITY", "REVOKE_BINDING"} {
		r := Request{Action: action, RequestID: "revoke-1", TenantID: "tenant-1", ReviewReference: "review-1", ExpiresAt: now.Add(time.Minute)}
		if action != "REVOKE_TENANT" {
			r.UserID = "user-1"
		}
		if action == "REVOKE_BINDING" {
			r.BindingID = "binding-1"
		}
		if r.Validate(now) != nil {
			t.Fatal("valid revocation denied")
		}
		r.Address = "0x0000000000000000000000000000000000000001"
		if r.Validate(now) == nil {
			t.Fatal("revocation accepted enrollment payload")
		}
	}
}
