package services

import (
	"context"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestSwapExecuteFailsClosedWithoutMainnetAuthority(t *testing.T) {
	_, err := (&PersistedSwapService{}).ExecuteSwap(context.Background(), "intent", "approval", "policy", 1)
	if err == nil || apperrors.ToPublic(err).Code != apperrors.CodeCapabilityUnavailable {
		t.Fatalf("error = %v", err)
	}
}

func TestSwapFinancialConstructsOnlyCanonicalPair(t *testing.T) {
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	draft := SwapDraft{TokenIn: "USDC", TokenOut: "EURC", AmountIn: intents.Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}, ExpectedOutput: intents.Amount{Decimal: "9", BaseUnits: "9000000", Decimals: 6}, MinAmountOut: intents.Amount{Decimal: "8.91", BaseUnits: "8910000", Decimals: 6}, MaxSlippageBPS: 100, MinHopPriceX36: "123", QuoteID: "quote", QuoteSource: "source", EvidenceReference: "evidence", QuoteExpiresAt: now.Add(15 * time.Minute), SwapDeadline: now.Add(10 * time.Minute)}
	params, err := swapFinancial(draft, "0x2222222222222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if !params.Phase12Executable() || params.MinHopPriceX36 != "123" || params.Recipient != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("params = %#v", params)
	}
	draft.TokenOut = "USDC"
	if _, err := swapFinancial(draft, params.Recipient); err == nil {
		t.Fatal("same-token swap must fail")
	}
}
