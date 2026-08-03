package audit

import (
	"testing"
	"time"
)

func TestRecordValidationRejectsForbiddenMaterial(t *testing.T) {
	record := Record{Event: Event{EventID: "event_1", Type: EventIntentCreated, OccurredAt: time.Now()}, ActorType: "user", ActorID: "user_1", RequestID: "request_1", ResourceType: "intent", ResourceID: "intent_1", SourceComponent: "test"}
	if err := record.Validate(); err != nil {
		t.Fatalf("valid record: %v", err)
	}
	record.SafeReasonCode = "oauth token=unsafe"
	if err := record.Validate(); err == nil {
		t.Fatal("forbidden material accepted")
	}
}
