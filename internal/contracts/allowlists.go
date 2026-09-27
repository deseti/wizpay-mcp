package contracts

// Canonical execution, read, and verification allowlists for known ContractIDs.
// Deployment.Validate requires exact set equality (order-independent) with these
// lists. Substituting, adding, or removing any signature fails closed.

var (
	canonicalPayrollExecution = []string{
		"executeCrossTokenPayroll(address,address,address[],uint256[],uint256,uint256,uint256,uint256,string)",
		"executeSameTokenPayroll(address,address[],uint256[],string)",
	}
	canonicalPayrollReads = []string{
		"ARC_MAINNET_CHAIN_ID()",
		"ARC_MAINNET_POOL_FEE()",
		"ARC_MAINNET_POOL_TICK_SPACING()",
		"ARC_NATIVE_USDC_SCALE()",
		"EURC()",
		"MAX_BATCH_SIZE()",
		"MAX_DEADLINE_WINDOW()",
		"MAX_FEE_BPS()",
		"MAX_REFERENCE_ID_LENGTH()",
		"USDC()",
		"feeBps()",
		"feeRecipient()",
		"owner()",
		"paused()",
		"permit2()",
		"poolFee()",
		"poolManager()",
		"poolTickSpacing()",
		"universalRouter()",
		"usedReferenceHashes(bytes32)",
	}
	canonicalPayrollEvents = []string{
		"PayrollBatchExecuted(address,address,address,uint256,uint256,uint256,uint256,string)",
		"PayrollPayment(bytes32,address,address,address,uint256,uint256)",
		"PayrollReferenceConsumed(bytes32,address,address,address,bytes32,uint256,uint256,uint256,uint256,string)",
		"PayrollSurplusRefunded(bytes32,address,address,uint256)",
		"PayrollSwapExecuted(bytes32,address,address,address,uint256,uint256,uint256,uint256,uint256)",
	}

	canonicalSwapExecution = []string{
		"executeSwap(address,address,uint256,uint256,uint256,uint256)",
	}
	canonicalSwapReads = []string{
		"ARC_MAINNET_CHAIN_ID()",
		"ARC_MAINNET_POOL_FEE()",
		"ARC_MAINNET_POOL_TICK_SPACING()",
		"ARC_NATIVE_USDC_SCALE()",
		"EURC()",
		"MAX_DEADLINE_WINDOW()",
		"MAX_FEE_BPS()",
		"USDC()",
		"feeBps()",
		"feeRecipient()",
		"owner()",
		"paused()",
		"permit2()",
		"poolFee()",
		"poolTickSpacing()",
		"universalRouter()",
	}
	canonicalSwapEvents = []string{
		"WizPayMainnetSwapExecuted(address,address,address,uint256,uint256,uint256,uint256,uint256)",
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
