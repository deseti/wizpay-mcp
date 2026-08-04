package contracts

// Canonical execution, read, and verification allowlists for known ContractIDs.
// Deployment.Validate requires exact set equality (order-independent) with these
// lists. Substituting, adding, or removing any signature fails closed.

var (
	canonicalPayrollExecution = []string{
		"batchRouteAndPay(address,address[],address[],uint256[],uint256[],string)",
		"batchRouteAndPay(address,address,address[],uint256[],uint256[],string)",
		"routeAndPay(address,address,uint256,uint256,address)",
	}
	canonicalPayrollReads = []string{
		"getBatchEstimatedOutputs(address,address[],uint256[])",
		"getEstimatedOutput(address,address,uint256)",
		"paused()",
		"whitelistEnabled()",
		"whitelistedTokens(address)",
		"feeBps()",
	}
	canonicalPayrollEvents = []string{
		"BatchPaymentRouted(address,address,address,uint256,uint256,uint256,uint256,string)",
		"PaymentRouted(address,address,address,address,uint256,uint256,uint256)",
	}

	canonicalSwapExecution = []string{
		"executeSwap(address,address,address,uint256,uint256,address,uint256)",
	}
	canonicalSwapReads = []string{
		"allowedRouters(address)",
		"allowedTokens(address)",
		"feeBps()",
		"feeRecipient()",
		"paused()",
	}
	canonicalSwapEvents = []string{
		"WizPaySwapExecuted(address,address,address,address,uint256,uint256,uint256,uint256,address)",
	}
)

// sameStringSet reports whether a and b contain the same unique strings,
// ignoring order. Duplicates in either side yield false.
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, value := range a {
		counts[value]++
	}
	for _, value := range b {
		n, ok := counts[value]
		if !ok || n == 0 {
			return false
		}
		counts[value] = n - 1
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}
