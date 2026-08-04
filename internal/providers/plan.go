package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/deseti/wizpay-mcp/internal/execution"
)

const idempotencyDomain = "WIZPAY_MCP_PROVIDER_IDEMPOTENCY_V1\n"

// Plan is the provider-neutral description of what a single execution submits.
//
// Phase 11 defines this port but implements no domain planning: producing a
// Plan from an intent is capability logic and belongs to a later phase. The
// plan carries no credentials, no signatures, and no raw provider payload.
type Plan struct {
	WalletBindingID    string
	WalletID           string
	WalletAddress      string
	ChainID            string
	Network            string
	DestinationAddress string
	// TokenID is the provider's non-secret token identifier. It is required
	// because token units and native gas units are distinct denominations and
	// must never be inferred from one another.
	TokenID string
	// Amount is a decimal string denominated in the token's own units.
	Amount string
}

func (p Plan) Validate() error {
	for _, field := range []struct{ name, value string }{
		{"wallet binding ID", p.WalletBindingID}, {"wallet ID", p.WalletID},
		{"network", p.Network}, {"token ID", p.TokenID},
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
// and must never accept caller-supplied wallet identifiers.
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
