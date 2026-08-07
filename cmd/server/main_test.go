package main

import (
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/mcp/tools"
)

func TestNewFoundationBundleRegistersFoundationTools(t *testing.T) {
	bundle := newFoundationBundle(nil, auth.NewPermissionAuthorizer(), time.Now)
	foundation, err := tools.NewFoundationRegistry(bundle)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{tools.CreateIntentName, tools.GetIntentName, tools.RequestApprovalName, tools.GetApprovalName, tools.EvaluatePolicyName, tools.PrepareExecutionName}
	for _, name := range want {
		if _, ok := foundation.Lookup(name); !ok {
			t.Fatalf("foundation tool %q was not registered", name)
		}
	}
}
