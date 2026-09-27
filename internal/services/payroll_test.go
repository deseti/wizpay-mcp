package services

import (
	"context"
	"testing"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestPayrollExecuteFailsClosedWithoutMainnetAuthority(t *testing.T) {
	_, err := (&PersistedPayrollService{}).ExecutePayroll(context.Background(), "intent", "approval", "policy", 1)
	if err == nil || apperrors.ToPublic(err).Code != apperrors.CodeCapabilityUnavailable {
		t.Fatalf("error = %v", err)
	}
}

func TestPayrollFinancialConstructsCrossTokenShape(t *testing.T) {
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	draft := PayrollDraft{TokenSymbol: "USDC", OutputTokenSymbol: "EURC", ReferenceID: "cross", Recipients: []PayrollRecipientDraft{{Address: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}}}, GrossInput: intents.Amount{Decimal: "11", BaseUnits: "11000000", Decimals: 6}, MinTotalOut: intents.Amount{Decimal: "10", BaseUnits: "10000000", Decimals: 6}, MinHopPriceX36: "1", SwapDeadline: now.Add(10 * time.Minute)}
	params, err := payrollFinancial(draft)
	if err != nil {
		t.Fatal(err)
	}
	if !params.CrossTokenExecutable() || !params.Recipients[0].AmountIn.IsZero() || params.CrossToken.GrossInput.BaseUnits != "11000000" {
		t.Fatalf("params=%#v", params)
	}
}

func TestPayrollFinancialOnlyConstructsExactSameTokenShape(t *testing.T) {
	draft := PayrollDraft{TokenSymbol: "EURC", ReferenceID: "payroll-1", Recipients: []PayrollRecipientDraft{{Address: "0x3333333333333333333333333333333333333333", Amount: intents.Amount{Decimal: "1", BaseUnits: "1000000", Decimals: 6}}, {Address: "0x4444444444444444444444444444444444444444", Amount: intents.Amount{Decimal: "2.5", BaseUnits: "2500000", Decimals: 6}}}}
	params, err := payrollFinancial(draft)
	if err != nil {
		t.Fatal(err)
	}
	if !params.SameTokenExecutable() || params.Total.BaseUnits != "3500000" || params.Total.Decimal != "3.5" || params.ReferenceID != draft.ReferenceID {
		t.Fatalf("params = %#v", params)
	}
	for _, recipient := range params.Recipients {
		if recipient.TokenOut != params.TokenIn || recipient.AmountIn != recipient.MinAmountOut {
			t.Fatalf("non-exact line = %#v", recipient)
		}
	}
}
