package intents

import (
	"testing"
	"time"
)

func TestNewDraftNormalizesPersistedTimestampsToPostgresPrecision(t *testing.T) {
	t.Run("send", func(t *testing.T) {
		params := sendParams("USDC")
		params.CreatedAt = params.CreatedAt.Add(123456789 * time.Nanosecond)
		params.ExpiresAt = params.ExpiresAt.Add(845678912 * time.Nanosecond)
		params.Constraints.Deadline = params.Constraints.Deadline.Add(234567891 * time.Nanosecond)

		value := mustDraft(t, params)
		assertMicrosecondTimestamp(t, "created_at", value.CreatedAt(), params.CreatedAt)
		assertMicrosecondTimestamp(t, "expires_at", value.ExpiresAt(), params.ExpiresAt)
		assertMicrosecondTimestamp(t, "constraints.deadline", value.Constraints().Deadline, params.Constraints.Deadline)
	})

	t.Run("swap", func(t *testing.T) {
		params := phase12SwapParams()
		params.Financial.Swap.Quote.ExpiresAt = params.Financial.Swap.Quote.ExpiresAt.Add(456789123 * time.Nanosecond)
		params.Financial.Swap.Deadline = params.Financial.Swap.Deadline.Add(567891234 * time.Nanosecond)

		value := mustDraft(t, params)
		financial := value.Financial()
		assertMicrosecondTimestamp(t, "swap.quote.expires_at", financial.Swap.Quote.ExpiresAt, params.Financial.Swap.Quote.ExpiresAt)
		assertMicrosecondTimestamp(t, "swap.deadline", financial.Swap.Deadline, params.Financial.Swap.Deadline)
	})

	t.Run("cross-token payroll", func(t *testing.T) {
		params := trackECrossPayrollParams(true)
		params.Financial.Payroll.CrossToken.Deadline = params.Financial.Payroll.CrossToken.Deadline.Add(678912345 * time.Nanosecond)

		value := mustDraft(t, params)
		financial := value.Financial()
		assertMicrosecondTimestamp(t, "payroll.cross_token.deadline", financial.Payroll.CrossToken.Deadline, params.Financial.Payroll.CrossToken.Deadline)
	})
}

func TestRestoreDoesNotRewriteHistoricalSubMicrosecondTimestamps(t *testing.T) {
	params := sendParams("USDC")
	params.CreatedAt = params.CreatedAt.Add(123456789 * time.Nanosecond)
	params.ExpiresAt = params.ExpiresAt.Add(845678912 * time.Nanosecond)
	params.Constraints.Deadline = params.Constraints.Deadline.Add(234567891 * time.Nanosecond)
	params = normalizeParams(params)
	canonical, err := canonicalMaterial(params)
	if err != nil {
		t.Fatal(err)
	}
	digest := digestBytes(canonical)

	restored, err := Restore(params, StatusCreated, digest, 2)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if restored.Digest() != digest {
		t.Fatalf("restored digest = %q, want %q", restored.Digest(), digest)
	}
	if !restored.CreatedAt().Equal(params.CreatedAt) || restored.CreatedAt().Nanosecond()%int(time.Microsecond) == 0 {
		t.Fatalf("Restore rewrote historical created_at: got %s, want %s", restored.CreatedAt(), params.CreatedAt)
	}
}

func assertMicrosecondTimestamp(t *testing.T, name string, got, input time.Time) {
	t.Helper()
	want := input.UTC().Truncate(time.Microsecond)
	if !got.Equal(want) {
		t.Fatalf("%s = %s, want %s", name, got, want)
	}
	if got.Nanosecond()%int(time.Microsecond) != 0 {
		t.Fatalf("%s retains sub-microsecond precision: %s", name, got)
	}
}
