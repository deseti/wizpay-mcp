package auth

import (
	"fmt"
	"strings"
)

// RequestMetadata is transport-neutral correlation metadata. It contains no
// authentication credential or HTTP-specific value.
type RequestMetadata struct {
	RequestID string
	ClientID  string
	TraceID   string
}

// IdentityContext carries the already-resolved identity into future
// application and MCP tool boundaries.
type IdentityContext struct {
	identity Identity
	metadata RequestMetadata
}

// NewIdentityContext validates and preserves resolved request identity data.
func NewIdentityContext(identity Identity, metadata RequestMetadata) (IdentityContext, error) {
	if err := identity.Validate(); err != nil {
		return IdentityContext{}, fmt.Errorf("invalid identity context: %w", err)
	}

	metadata.RequestID = strings.TrimSpace(metadata.RequestID)
	metadata.ClientID = strings.TrimSpace(metadata.ClientID)
	metadata.TraceID = strings.TrimSpace(metadata.TraceID)
	if metadata.RequestID == "" {
		return IdentityContext{}, fmt.Errorf("request ID is required")
	}
	if len(metadata.RequestID) > maxIdentityFieldLength ||
		len(metadata.ClientID) > maxIdentityFieldLength ||
		len(metadata.TraceID) > maxIdentityFieldLength {
		return IdentityContext{}, fmt.Errorf("request metadata exceeds %d characters", maxIdentityFieldLength)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "request ID", value: metadata.RequestID},
		{name: "client ID", value: metadata.ClientID},
		{name: "trace ID", value: metadata.TraceID},
	} {
		if field.value != "" {
			if err := validateIdentityField(field.name, field.value); err != nil {
				return IdentityContext{}, err
			}
		}
	}

	return IdentityContext{identity: identity, metadata: metadata}, nil
}

func (c IdentityContext) Identity() Identity        { return c.identity }
func (c IdentityContext) Metadata() RequestMetadata { return c.metadata }
