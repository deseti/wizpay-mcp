package policies

import (
	"testing"

	"github.com/deseti/wizpay-mcp/internal/intents"
)

func TestSameTokenPayrollPolicyUsesMaximumEmployerExposure(t *testing.T) {
	obligations := intents.Amount{Decimal: "100", BaseUnits: "100000000", Decimals: 6}
	exposure := payrollMaximumEmployerExposure(obligations)
	if exposure.BaseUnits != "101000000" || exposure.Decimal != "101" {
		t.Fatalf("exposure = %#v", exposure)
	}
}
