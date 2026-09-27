package swap

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	contractswap "github.com/deseti/wizpay-mcp/internal/contracts/swap"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/providers"
)

type DomainStatus string

const (
	DomainVerified   DomainStatus = "DOMAIN_VERIFIED"
	DomainUnverified DomainStatus = "DOMAIN_UNVERIFIED"
	DomainFailed     DomainStatus = "DOMAIN_FAILED"
)

type DomainResult struct {
	Status                     DomainStatus
	ReasonCode, EventSignature string
	Provable                   []string
	TransactionHash            string
	definitiveFailure          bool
}

func (r DomainResult) DefinitiveFailure() bool {
	return r.Status == DomainFailed && r.definitiveFailure
}
func (r DomainResult) FinancialComplete() bool { return r.Status == DomainVerified }

type Verifier struct{ registry *contracts.Registry }

func NewVerifier(registry *contracts.Registry) Verifier { return Verifier{registry: registry} }

func (v Verifier) Verify(intent intents.Intent, plan Plan, receipt providers.Receipt) (DomainResult, error) {
	if err := intent.Validate(); err != nil {
		return DomainResult{}, fmt.Errorf("swap intent is invalid: %w", err)
	}
	if intent.Type() != intents.TypeSwap || intent.Digest() == "" || intent.Status() == intents.StatusDraft {
		return DomainResult{}, fmt.Errorf("swap verifier requires a frozen SWAP intent")
	}
	if err := v.validatePlan(intent, plan); err != nil {
		return DomainResult{}, fmt.Errorf("swap plan binding is invalid: %w", err)
	}
	result := DomainResult{Status: DomainUnverified, ReasonCode: "RECEIPT_NOT_SUCCESS", TransactionHash: strings.ToLower(strings.TrimSpace(receipt.TransactionHash))}
	fail := func(code string) (DomainResult, error) {
		result.Status, result.ReasonCode, result.definitiveFailure = DomainFailed, code, true
		return result, nil
	}
	if receipt.Status == providers.ReceiptReverted {
		return fail("ONCHAIN_EXECUTION_REVERTED")
	}
	if receipt.Status != providers.ReceiptSuccess {
		return result, nil
	}
	if receipt.ChainID != contracts.ChainIDArcMainnet {
		return fail("RECEIPT_CHAIN_MISMATCH")
	}
	if !providers.ValidTransactionHash(result.TransactionHash) {
		return fail("RECEIPT_HASH_INVALID")
	}
	if !receipt.HasTransaction {
		return result, nil
	}
	if !contracts.AddressesEqual(receipt.From, plan.WalletAddress()) {
		return fail("TRANSACTION_SENDER_MISMATCH")
	}
	if !contracts.AddressesEqual(receipt.To, contracts.AddressWizPaySwapExecutor) {
		return fail("TRANSACTION_TARGET_MISMATCH")
	}
	if !bytes.Equal(receipt.Input, plan.EncodedCall().CallData()) {
		return fail("TRANSACTION_CALLDATA_PLAN_MISMATCH")
	}
	decoded, err := contractswap.DecodeExecuteSwapCall(receipt.Input)
	if err != nil {
		return fail("TRANSACTION_CALLDATA_INVALID")
	}
	financial := intent.Financial().Swap
	amountIn, _ := financial.InputAmount.BaseInt()
	minAmountOut, _ := financial.MinimumOutput.BaseInt()
	minHopPrice, _ := new(big.Int).SetString(financial.MinHopPriceX36, 10)
	deadline := big.NewInt(financial.Deadline.UTC().Unix())
	if !contracts.AddressesEqual(decoded.TokenIn, financial.InputToken.Address) || !contracts.AddressesEqual(decoded.TokenOut, financial.OutputToken.Address) || decoded.AmountIn.Cmp(amountIn) != 0 || decoded.MinAmountOut.Cmp(minAmountOut) != 0 || decoded.MinHopPriceX36.Cmp(minHopPrice) != 0 || decoded.Deadline.Cmp(deadline) != 0 {
		return fail("TRANSACTION_FINANCIAL_MISMATCH")
	}
	value, err := hexutil.DecodeBig(receipt.Value)
	if err != nil {
		return fail("TRANSACTION_VALUE_INVALID")
	}
	expectedValue := new(big.Int)
	if contracts.AddressesEqual(financial.InputToken.Address, contracts.AddressUSDCMainnet) {
		expectedValue.Mul(amountIn, big.NewInt(1_000_000_000_000))
	}
	if value.Cmp(expectedValue) != 0 {
		return fail("TRANSACTION_VALUE_MISMATCH")
	}
	if err := providers.ValidateLogs(receipt.Logs); err != nil {
		return fail("RECEIPT_LOGS_MALFORMED")
	}
	eventTopic, _ := contractswap.EventTopic0()
	var executed []contractswap.WizPayMainnetSwapExecuted
	for _, raw := range receipt.Logs {
		if !contracts.AddressesEqual(raw.Address, contracts.AddressWizPaySwapExecutor) || len(raw.Topics) == 0 || !bytes.Equal(raw.Topics[0], eventTopic) {
			continue
		}
		event, decodeErr := contractswap.DecodeWizPayMainnetSwapExecuted(v.registry, raw.ContractLog(receipt.ChainID))
		if decodeErr != nil {
			return fail("SWAP_EVENT_MALFORMED")
		}
		executed = append(executed, event)
	}
	if len(executed) != 1 {
		return fail("SWAP_EVENT_COUNT_MISMATCH")
	}
	event := executed[0]
	if !contracts.AddressesEqual(event.Caller, plan.WalletAddress()) {
		return fail("SWAP_EVENT_CALLER_MISMATCH")
	}
	if !contracts.AddressesEqual(event.TokenIn, financial.InputToken.Address) || !contracts.AddressesEqual(event.TokenOut, financial.OutputToken.Address) {
		return fail("SWAP_EVENT_TOKEN_MISMATCH")
	}
	if event.AmountIn.Cmp(amountIn) != 0 || event.MinAmountOut.Cmp(minAmountOut) != 0 {
		return fail("SWAP_EVENT_FINANCIAL_MISMATCH")
	}
	if event.AmountOut.Cmp(minAmountOut) < 0 {
		return fail("SWAP_OUTPUT_BELOW_MINIMUM")
	}
	if new(big.Int).Add(new(big.Int).Set(event.NetAmountIn), event.FeeAmount).Cmp(amountIn) != 0 {
		return fail("SWAP_FEE_ARITHMETIC_MISMATCH")
	}
	feeScaled := new(big.Int).Mul(new(big.Int).Set(event.FeeAmount), big.NewInt(10_000))
	maxScaled := new(big.Int).Mul(new(big.Int).Set(amountIn), new(big.Int).SetUint64(contracts.ContractMaxFeeBPS))
	if event.FeeAmount.Sign() < 0 || event.NetAmountIn.Sign() < 0 || feeScaled.Cmp(maxScaled) > 0 {
		return fail("SWAP_FEE_EXCEEDS_MAXIMUM")
	}
	if !hasOutputTransfer(receipt.Logs, financial.OutputToken.Address, plan.WalletAddress(), event.AmountOut) {
		return fail("SWAP_OUTPUT_TRANSFER_MISSING")
	}
	result.Status, result.ReasonCode, result.EventSignature = DomainVerified, "SWAP_VERIFIED", contractswap.SigWizPayMainnetSwapExecuted
	result.Provable = []string{"sender", "target", "calldata", "funding", "swap_event", "fee_arithmetic", "minimum_output", "output_transfer"}
	return result, nil
}

var transferEventID = crypto.Keccak256([]byte("Transfer(address,address,uint256)"))

func hasOutputTransfer(logs []providers.ReceiptLog, token, recipient string, amount *big.Int) bool {
	for _, log := range logs {
		if !contracts.AddressesEqual(log.Address, token) || len(log.Topics) != 3 || !bytes.Equal(log.Topics[0], transferEventID) || len(log.Data) != 32 {
			continue
		}
		if topicAddress(log.Topics[2], recipient) && new(big.Int).SetBytes(log.Data).Cmp(amount) == 0 {
			return true
		}
	}
	return false
}

func topicAddress(topic []byte, address string) bool {
	if len(topic) != 32 {
		return false
	}
	for _, b := range topic[:12] {
		if b != 0 {
			return false
		}
	}
	return contracts.AddressesEqual("0x"+fmt.Sprintf("%x", topic[12:]), address)
}

func (v Verifier) validatePlan(intent intents.Intent, plan Plan) error {
	if plan.IntentID() != intent.IntentID() || plan.IntentDigest() != intent.Digest() || plan.Capability() != intents.TypeSwap {
		return fmt.Errorf("plan does not bind the frozen swap intent")
	}
	if plan.ContractID() != contracts.ContractWizPaySwapExecutor || plan.RegistryVersion() != contracts.RegistryVersion || plan.ChainID() != contracts.ChainIDArcMainnet {
		return fmt.Errorf("plan does not bind the canonical swap deployment")
	}
	if plan.ChainID() != intent.Ownership().ChainID || !contracts.AddressesEqual(plan.WalletAddress(), intent.Ownership().WalletAddress) {
		return fmt.Errorf("plan sender or chain does not match intent ownership")
	}
	call := plan.EncodedCall()
	if !contracts.AddressesEqual(call.To(), contracts.AddressWizPaySwapExecutor) || call.ContractID() != contracts.ContractWizPaySwapExecutor {
		return fmt.Errorf("plan target is not the canonical swap contract")
	}
	deployment, err := contractswap.ExpectedDeployment(v.registry)
	if err != nil || !contracts.AddressesEqual(deployment.Address, call.To()) {
		return fmt.Errorf("plan target does not match registry")
	}
	financial := intent.Financial().Swap
	if financial == nil || !financial.Phase12Executable() {
		return fmt.Errorf("intent is not executable swap")
	}
	decoded, err := contractswap.DecodeExecuteSwapCall(call.CallData())
	if err != nil {
		return err
	}
	amount, _ := financial.InputAmount.BaseInt()
	minimum, _ := financial.MinimumOutput.BaseInt()
	price, _ := new(big.Int).SetString(financial.MinHopPriceX36, 10)
	if !contracts.AddressesEqual(decoded.TokenIn, financial.InputToken.Address) || !contracts.AddressesEqual(decoded.TokenOut, financial.OutputToken.Address) || decoded.AmountIn.Cmp(amount) != 0 || decoded.MinAmountOut.Cmp(minimum) != 0 || decoded.MinHopPriceX36.Cmp(price) != 0 || decoded.Deadline.Cmp(big.NewInt(financial.Deadline.UTC().Unix())) != 0 {
		return fmt.Errorf("plan calldata does not bind intent")
	}
	return nil
}
