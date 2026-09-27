package payroll_test

import (
	"bytes"
	"testing"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/contracts/payroll"
)

func TestMainnetPayrollEventSignatures(t *testing.T) {
	signatures := []string{payroll.SigPayrollBatchExecuted, payroll.SigPayrollPayment, payroll.SigPayrollReferenceConsumed, payroll.SigPayrollSurplusRefunded, payroll.SigPayrollSwapExecuted}
	for _, signature := range signatures {
		event, err := payroll.EventBySignature(signature)
		if err != nil {
			t.Fatalf("%s: %v", signature, err)
		}
		if !bytes.Equal(event.ID.Bytes(), contracts.EventTopic0(signature)) {
			t.Fatalf("%s topic mismatch", signature)
		}
		if len(event.Inputs) < 3 || !event.Inputs[0].Indexed || !event.Inputs[1].Indexed || !event.Inputs[2].Indexed {
			t.Fatalf("%s indexed layout mismatch", signature)
		}
	}
	if _, err := payroll.EventBySignature("PaymentRouted(address,address,address,address,uint256,uint256,uint256)"); err == nil {
		t.Fatal("Testnet event must not remain allowlisted")
	}
}
