package auth

import "testing"

func TestNewIdentityContextPreservesMetadata(t *testing.T) {
	identity, err := NewIdentity("user_123", "example-provider", IdentityStatusActive)
	if err != nil {
		t.Fatalf("NewIdentity() error = %v", err)
	}
	metadata := RequestMetadata{
		RequestID: "request_123",
		ClientID:  "client_123",
		TraceID:   "trace_123",
	}

	identityContext, err := NewIdentityContext(identity, metadata)
	if err != nil {
		t.Fatalf("NewIdentityContext() error = %v", err)
	}
	if identityContext.Identity().UserID() != identity.UserID() {
		t.Fatalf("context user ID = %q, want %q", identityContext.Identity().UserID(), identity.UserID())
	}
	if identityContext.Metadata() != metadata {
		t.Fatalf("context metadata = %+v, want %+v", identityContext.Metadata(), metadata)
	}
}

func TestNewIdentityContextRequiresRequestID(t *testing.T) {
	identity, err := NewIdentity("user_123", "example-provider", IdentityStatusActive)
	if err != nil {
		t.Fatalf("NewIdentity() error = %v", err)
	}
	if _, err := NewIdentityContext(identity, RequestMetadata{}); err == nil {
		t.Fatal("NewIdentityContext() accepted missing request ID")
	}
}
