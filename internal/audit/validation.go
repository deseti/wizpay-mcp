package audit

import (
	"fmt"
	"strings"
	"unicode"
)

func (r Record) Validate() error {
	if r.Event.EventID == "" || r.Event.Type == "" || r.Event.OccurredAt.IsZero() {
		return fmt.Errorf("audit event identity and time are required")
	}
	for name, value := range map[string]string{"actor type": r.ActorType, "actor ID": r.ActorID, "request ID": r.RequestID, "resource type": r.ResourceType, "resource ID": r.ResourceID, "source component": r.SourceComponent} {
		if err := safeField(name, value, true); err != nil {
			return err
		}
	}
	for name, value := range map[string]string{"trace ID": r.TraceID, "previous state": r.PreviousState, "new state": r.NewState, "safe reason code": r.SafeReasonCode} {
		if err := safeField(name, value, false); err != nil {
			return err
		}
	}
	for _, value := range []string{r.ActorType, r.ActorID, r.RequestID, r.TraceID, r.ResourceType, r.ResourceID, r.SafeReasonCode, r.SourceComponent, r.Event.IntentID, r.Event.IntentDigest, r.Event.ApprovalID, r.Event.PolicyID, r.Event.PolicyEvaluationKey, r.Event.ExecutionID, r.Event.ExecutionRequestID, r.Event.ExecutionRequestKey, r.Event.RecoveryReasonCode, r.Event.WalletBindingID, r.Event.UserID, r.Event.OperationKey} {
		if containsForbidden(value) {
			return fmt.Errorf("audit record contains forbidden material")
		}
	}
	return nil
}
func safeField(name, value string, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}
func containsForbidden(value string) bool {
	lower := strings.ToLower(value)
	for _, word := range []string{"private_key", "private key", "seed phrase", "mnemonic", "signing share", "oauth token", "access_token", "refresh_token", "authorization: bearer", "client_secret", "api_key"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}
