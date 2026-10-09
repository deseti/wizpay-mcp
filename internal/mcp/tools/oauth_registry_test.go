package tools

import "testing"

func TestOAuthReadOnlyRegistry(t *testing.T) {
	bundle := testBundle(nil, nil)
	r, err := NewOAuthReadOnlyRegistry(bundle.Intents, bundle.Approvals)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Definitions()
	if len(d) != 2 || d[0].Name() != GetApprovalName || d[1].Name() != GetIntentName {
		t.Fatalf("unexpected OAuth allowlist: %v", d)
	}
	if _, err := NewOAuthReadOnlyRegistry(nil, bundle.Approvals); err == nil {
		t.Fatal("missing intent service accepted")
	}
	if _, err := NewOAuthReadOnlyRegistry(bundle.Intents, nil); err == nil {
		t.Fatal("missing approval service accepted")
	}
	legacy, err := NewFoundationRegistry(bundle)
	if err != nil || len(legacy.Tools()) != 6 {
		t.Fatal("legacy foundation registry changed")
	}
}
