package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/deseti/wizpay-mcp/internal/contracts"
	"github.com/deseti/wizpay-mcp/internal/execution"
)

const idempotencyDomain = "WIZPAY_MCP_PROVIDER_IDEMPOTENCY_V1\n"

// PlanKind selects the closed submission surface for a Plan.
//
// External callers never supply raw contractAddress/callData/function/selector
// fields. Contract execution is only expressible through a sealed
// contracts.EncodedCall constructed by typed domain encoders.
type PlanKind string

const (
	// PlanKindTokenTransfer is the existing Circle token-transfer surface.
	// Empty Kind is treated as this value for backward compatibility.
	PlanKindTokenTransfer PlanKind = "TOKEN_TRANSFER"
	// PlanKindContractExecution is the sealed allowlisted contract-call surface
	// for WIZPAY_PAYROLL and WIZPAY_SWAP_EXECUTOR only.
	PlanKindContractExecution PlanKind = "CONTRACT_EXECUTION"
)

// Plan is the provider-neutral description of what a single execution submits.
//
// Producing a Plan from an intent is capability/orchestration logic. The plan
// carries no credentials, no signatures, and no raw provider payload.
//
// CONTRACT_EXECUTION plans seal the target and calldata inside
// contracts.EncodedCall. There is no generic contract executor surface: callers
// cannot set contractAddress, callData, function, or selector directly.
type Plan struct {
	// Kind selects the submission surface. Empty means TOKEN_TRANSFER.
	Kind PlanKind

	WalletBindingID string
	WalletID        string
	WalletAddress   string
	ChainID         string
	Network         string

	// DestinationAddress, TokenID, and Amount are TOKEN_TRANSFER fields only.
	// TokenID is the provider's non-secret token identifier. Amount is a
	// decimal string denominated in the token's own units.
	DestinationAddress string
	TokenID            string
	Amount             string

	// encodedCall is CONTRACT_EXECUTION only. It is unexported so external
	// packages cannot inject arbitrary target/calldata without going through
	// NewContractExecutionPlan and a sealed contracts.EncodedCall.
	encodedCall    contracts.EncodedCall
	hasEncodedCall bool

	// submitNotAfter is the exclusive upper bound for first submission when
	// set. Checked only before the first provider submission attempt using an
	// injected clock; once submission may have started, reconciliation must
	// continue even if this bound is in the past.
	submitNotAfter    time.Time
	hasSubmitNotAfter bool
}

// ContractExecutionParams constructs a sealed CONTRACT_EXECUTION plan.
//
// Call must already be a sealed EncodedCall from a typed payroll/swap encoder.
// SubmitNotAfter is required and must be derived from immutable financial
// material (intent expiry, constraint deadline, and for Swap also frozen swap
// deadline and quote expiry) using an explicit orchestration clock — never
// time.Now() inside planners or encoders.
type ContractExecutionParams struct {
	WalletBindingID string
	WalletID        string
	WalletAddress   string
	ChainID         string
	Network         string
	Call            contracts.EncodedCall
	SubmitNotAfter  time.Time
}

// NewContractExecutionPlan builds a closed CONTRACT_EXECUTION plan.
//
// It rejects zero/missing EncodedCall material, unsupported contract IDs, and a
// missing freshness bound. Target address and calldata come only from Call.
func NewContractExecutionPlan(params ContractExecutionParams) (Plan, error) {
	if params.Call.To() == "" || len(params.Call.CallData()) < 4 {
		return Plan{}, fmt.Errorf("contract execution plan requires a sealed encoded call")
	}
	if !allowedContractExecutionID(params.Call.ContractID()) {
		return Plan{}, fmt.Errorf("contract execution plan contract %q is not supported", params.Call.ContractID())
	}
	if params.SubmitNotAfter.IsZero() {
		return Plan{}, fmt.Errorf("contract execution plan requires a submit-not-after freshness bound")
	}
	plan := Plan{
		Kind:              PlanKindContractExecution,
		WalletBindingID:   params.WalletBindingID,
		WalletID:          params.WalletID,
		WalletAddress:     params.WalletAddress,
		ChainID:           params.ChainID,
		Network:           params.Network,
		encodedCall:       params.Call.Clone(),
		hasEncodedCall:    true,
		submitNotAfter:    params.SubmitNotAfter.UTC(),
		hasSubmitNotAfter: true,
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// EffectiveKind returns the closed plan kind. Empty Kind is TOKEN_TRANSFER.
func (p Plan) EffectiveKind() PlanKind {
	switch p.Kind {
	case PlanKindContractExecution:
		return PlanKindContractExecution
	case PlanKindTokenTransfer, "":
		return PlanKindTokenTransfer
	default:
		return p.Kind
	}
}

// EncodedCall returns the sealed contract call for CONTRACT_EXECUTION plans.
func (p Plan) EncodedCall() (contracts.EncodedCall, bool) {
	if !p.hasEncodedCall {
		return contracts.EncodedCall{}, false
	}
	return p.encodedCall.Clone(), true
}

// SubmitNotAfter returns the exclusive first-submission freshness bound when set.
func (p Plan) SubmitNotAfter() (time.Time, bool) {
	if !p.hasSubmitNotAfter || p.submitNotAfter.IsZero() {
		return time.Time{}, false
	}
	return p.submitNotAfter.UTC(), true
}

// FreshnessExpired reports whether first submission is forbidden at at.
//
// Reconciliation paths must not call this to decide resubmission: once
// submission may have started, expiry never authorizes a replacement submit.
func (p Plan) FreshnessExpired(at time.Time) bool {
	bound, ok := p.SubmitNotAfter()
	if !ok {
		return false
	}
	if at.IsZero() {
		return true
	}
	return !at.UTC().Before(bound)
}

// EarliestDeadline returns the earliest non-zero UTC candidate, or zero if none
// are set. Orchestration uses this to combine intent expiry, constraint
// deadline, swap deadline, and quote expiry into SubmitNotAfter.
func EarliestDeadline(candidates ...time.Time) time.Time {
	var earliest time.Time
	for _, candidate := range candidates {
		if candidate.IsZero() {
			continue
		}
		value := candidate.UTC()
		if earliest.IsZero() || value.Before(earliest) {
			earliest = value
		}
	}
	return earliest
}

func (p Plan) Validate() error {
	for _, field := range []struct{ name, value string }{
		{"wallet binding ID", p.WalletBindingID}, {"wallet ID", p.WalletID},
		{"network", p.Network},
	} {
		if !validSafeText(field.value) {
			return fmt.Errorf("submission plan %s is invalid", field.name)
		}
	}
	if !validChainID(p.ChainID) {
		return fmt.Errorf("submission plan chain ID is invalid")
	}
	if !ValidAddress(p.WalletAddress) {
		return fmt.Errorf("submission plan wallet address is invalid")
	}

	switch p.EffectiveKind() {
	case PlanKindTokenTransfer:
		return p.validateTokenTransfer()
	case PlanKindContractExecution:
		return p.validateContractExecution()
	default:
		return fmt.Errorf("submission plan kind is invalid")
	}
}

func (p Plan) validateTokenTransfer() error {
	if p.hasEncodedCall {
		return fmt.Errorf("submission plan token transfer cannot carry an encoded call")
	}
	if p.hasSubmitNotAfter {
		// Token transfer keeps the historical shape; freshness bounds belong to
		// contract-execution financial material for this phase.
		return fmt.Errorf("submission plan token transfer cannot carry a contract freshness bound")
	}
	if !validSafeText(p.TokenID) {
		return fmt.Errorf("submission plan token ID is invalid")
	}
	if !ValidAddress(p.DestinationAddress) {
		return fmt.Errorf("submission plan destination address is invalid")
	}
	if strings.EqualFold(p.WalletAddress, p.DestinationAddress) {
		return fmt.Errorf("submission plan destination cannot equal the source wallet")
	}
	if err := validateAmount(p.Amount); err != nil {
		return err
	}
	return nil
}

func (p Plan) validateContractExecution() error {
	if p.DestinationAddress != "" || p.TokenID != "" || p.Amount != "" {
		return fmt.Errorf("submission plan contract execution cannot carry token-transfer fields")
	}
	if !p.hasEncodedCall {
		return fmt.Errorf("submission plan contract execution requires a sealed encoded call")
	}
	if !p.hasSubmitNotAfter || p.submitNotAfter.IsZero() {
		return fmt.Errorf("submission plan contract execution requires a submit-not-after freshness bound")
	}
	call := p.encodedCall
	if !allowedContractExecutionID(call.ContractID()) {
		return fmt.Errorf("submission plan contract %q is not supported", call.ContractID())
	}
	if call.RegistryVersion() == 0 {
		return fmt.Errorf("submission plan registry version is invalid")
	}
	if call.ChainID() == "" || call.To() == "" || call.Function() == "" || len(call.CallData()) < 4 {
		return fmt.Errorf("submission plan encoded call is incomplete")
	}
	if !ValidAddress(call.To()) {
		return fmt.Errorf("submission plan contract address is invalid")
	}
	// Plan chain identity must agree with the sealed call (no caller override).
	if p.ChainID != call.ChainID() {
		return fmt.Errorf("submission plan chain ID does not match encoded call")
	}
	if p.Network != "" && call.Network() != "" && p.Network != call.Network() {
		return fmt.Errorf("submission plan network does not match encoded call")
	}
	return nil
}

// allowedContractExecutionID is the Phase 12 Step 3 closed allowlist.
func allowedContractExecutionID(id contracts.ContractID) bool {
	switch id {
	case contracts.ContractWizPayPayroll, contracts.ContractWizPaySwapExecutor:
		return true
	default:
		return false
	}
}

// validateAmount enforces a plain positive decimal string. Unit interpretation
// is the token's, never this layer's: no scaling is applied here.
func validateAmount(amount string) error {
	if amount == "" || len(amount) > 80 || amount != strings.TrimSpace(amount) {
		return fmt.Errorf("submission plan amount is invalid")
	}
	whole, fraction, hasFraction := strings.Cut(amount, ".")
	if whole == "" || (hasFraction && fraction == "") {
		return fmt.Errorf("submission plan amount is invalid")
	}
	nonZero := false
	for _, part := range []string{whole, fraction} {
		for _, character := range part {
			if character < '0' || character > '9' {
				return fmt.Errorf("submission plan amount is invalid")
			}
			if character != '0' {
				nonZero = true
			}
		}
	}
	if !nonZero {
		return fmt.Errorf("submission plan amount must be positive")
	}
	return nil
}

// Planner resolves the authorized execution into a concrete submission plan.
// Implementations must derive every field from already-approved intent state
// and must never accept caller-supplied wallet identifiers or arbitrary
// contract target/calldata.
type Planner interface {
	Plan(ctx context.Context, request execution.Request) (Plan, error)
}

// IdempotencyKey derives a stable provider idempotency key from immutable
// execution identity. It is deterministic across process restarts and lease
// handoffs, so a retried submission is recognized by the provider as the same
// financial operation rather than a second one.
//
// The result is formatted as an RFC 4122 version 4 UUID because that is the
// shape providers accept; the value is derived, not random.
func IdempotencyKey(request execution.Request) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(idempotencyDomain))
	for _, part := range []string{
		request.ExecutionID(),
		request.RequestKey(),
		fmt.Sprint(request.Version()),
		request.OperationKey(),
		fmt.Sprint(request.OperationVersion()),
	} {
		_, _ = fmt.Fprintf(digest, "%d:", len(part))
		_, _ = digest.Write([]byte(part))
	}
	sum := digest.Sum(nil)
	sum[6] = (sum[6] & 0x0f) | 0x40
	sum[8] = (sum[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(sum[:16])
	return strings.Join([]string{encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]}, "-"), nil
}
