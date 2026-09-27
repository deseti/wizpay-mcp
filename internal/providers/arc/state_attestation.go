package arc

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

// DeploymentStateAttestation contains read-only observations from fixed getter
// calls on a canonical registry deployment. It grants no execution authority
// and does not assert that a contract is ready for financial use.
type DeploymentStateAttestation struct {
	ContractID      contracts.ContractID
	RegistryVersion uint
	ChainID         string
	Network         string
	Address         string
	Owner           string
	FeeRecipient    string
	FeeBPS          uint64
	Paused          bool
	USDC            string
	EURC            string
	PoolManager     string
	UniversalRouter string
	Permit2         string
	PoolFee         uint32
	PoolTickSpacing int32
}

// ValidateCanonicalResources checks only reviewed immutable/configuration
// identities. Owner, fee recipient, fee rate, and paused state are observations
// and are deliberately not treated as activation approval.
func (a DeploymentStateAttestation) ValidateCanonicalResources() error {
	if a.ChainID != contracts.ChainIDArcMainnet || a.Network != contracts.NetworkArcMainnet {
		return fmt.Errorf("attested deployment has the wrong Arc Mainnet identity")
	}
	expectedAddress := map[contracts.ContractID]string{
		contracts.ContractWizPayPayroll:      contracts.AddressWizPayPayroll,
		contracts.ContractWizPaySwapExecutor: contracts.AddressWizPaySwapExecutor,
	}[a.ContractID]
	if expectedAddress == "" || !contracts.AddressesEqual(a.Address, expectedAddress) {
		return fmt.Errorf("attested deployment address is not canonical")
	}
	if !contracts.ValidAddress(a.Owner) || contracts.AddressesEqual(a.Owner, contracts.AddressZero) ||
		!contracts.ValidAddress(a.FeeRecipient) || contracts.AddressesEqual(a.FeeRecipient, contracts.AddressZero) {
		return fmt.Errorf("attested owner or fee recipient is invalid")
	}
	if !contracts.AddressesEqual(a.USDC, contracts.AddressUSDCMainnet) ||
		!contracts.AddressesEqual(a.EURC, contracts.AddressEURCMainnet) ||
		!contracts.AddressesEqual(a.UniversalRouter, contracts.AddressUniswapUniversalRouter) ||
		!contracts.AddressesEqual(a.Permit2, contracts.AddressPermit2) {
		return fmt.Errorf("attested token or router resources are not canonical")
	}
	if a.ContractID == contracts.ContractWizPayPayroll && !contracts.AddressesEqual(a.PoolManager, contracts.AddressUniswapV4PoolManager) {
		return fmt.Errorf("attested PoolManager is not canonical")
	}
	if a.PoolFee != contracts.UniswapV4USDCEURCFee || a.PoolTickSpacing != contracts.UniswapV4TickSpacing {
		return fmt.Errorf("attested pool parameters are not canonical")
	}
	if a.FeeBPS > 10_000 {
		return fmt.Errorf("attested fee basis points exceed the basis-point denominator")
	}
	return nil
}

// AttestDeploymentState performs only a closed set of view calls against a
// canonical registry address. There is no arbitrary eth_call surface.
func (c *Client) AttestDeploymentState(ctx context.Context, id contracts.ContractID, version uint) (DeploymentStateAttestation, error) {
	deployment, err := contracts.DefaultRegistry().Require(id, version, contracts.ChainIDArcMainnet, contracts.NetworkArcMainnet)
	if err != nil {
		return DeploymentStateAttestation{}, fmt.Errorf("Arc deployment state target is not canonical: %w", err)
	}
	if err := c.ensureChainIdentity(ctx); err != nil {
		return DeploymentStateAttestation{}, err
	}
	addressReads := []struct {
		signature string
		dest      *string
	}{
		{signature: "owner()"},
		{signature: "feeRecipient()"},
		{signature: "USDC()"},
		{signature: "EURC()"},
		{signature: "universalRouter()"},
		{signature: "permit2()"},
	}
	attestation := DeploymentStateAttestation{
		ContractID: id, RegistryVersion: version, ChainID: deployment.ChainID,
		Network: deployment.Network, Address: deployment.Address,
	}
	addressReads[0].dest = &attestation.Owner
	addressReads[1].dest = &attestation.FeeRecipient
	addressReads[2].dest = &attestation.USDC
	addressReads[3].dest = &attestation.EURC
	addressReads[4].dest = &attestation.UniversalRouter
	addressReads[5].dest = &attestation.Permit2
	if id == contracts.ContractWizPayPayroll {
		addressReads = append(addressReads, struct {
			signature string
			dest      *string
		}{signature: "poolManager()", dest: &attestation.PoolManager})
	}
	for _, read := range addressReads {
		word, err := c.callDeploymentWord(ctx, deployment, read.signature)
		if err != nil {
			return DeploymentStateAttestation{}, err
		}
		*read.dest = common.BytesToAddress(word[12:]).Hex()
	}
	feeWord, err := c.callDeploymentWord(ctx, deployment, "feeBps()")
	if err != nil {
		return DeploymentStateAttestation{}, err
	}
	fee, err := decodeUint64Word(feeWord)
	if err != nil {
		return DeploymentStateAttestation{}, fmt.Errorf("Arc feeBps result is malformed")
	}
	attestation.FeeBPS = fee
	pausedWord, err := c.callDeploymentWord(ctx, deployment, "paused()")
	if err != nil {
		return DeploymentStateAttestation{}, err
	}
	paused, err := decodeBoolWord(pausedWord)
	if err != nil {
		return DeploymentStateAttestation{}, fmt.Errorf("Arc paused result is malformed")
	}
	attestation.Paused = paused
	poolFeeWord, err := c.callDeploymentWord(ctx, deployment, "poolFee()")
	if err != nil {
		return DeploymentStateAttestation{}, err
	}
	poolFee, err := decodeUint64Word(poolFeeWord)
	if err != nil || poolFee > uint64(^uint32(0)) {
		return DeploymentStateAttestation{}, fmt.Errorf("Arc poolFee result is malformed")
	}
	attestation.PoolFee = uint32(poolFee)
	tickWord, err := c.callDeploymentWord(ctx, deployment, "poolTickSpacing()")
	if err != nil {
		return DeploymentStateAttestation{}, err
	}
	tick, err := decodeInt32Word(tickWord)
	if err != nil {
		return DeploymentStateAttestation{}, fmt.Errorf("Arc poolTickSpacing result is malformed")
	}
	attestation.PoolTickSpacing = tick
	return attestation, nil
}

func (c *Client) callDeploymentWord(ctx context.Context, deployment contracts.Deployment, signature string) ([]byte, error) {
	selector := contracts.Selector4(signature)
	call := map[string]string{"to": deployment.Address, "data": fmt.Sprintf("0x%x", selector)}
	var encoded string
	if err := c.call(ctx, "eth_call", []any{call, "latest"}, &encoded); err != nil {
		return nil, err
	}
	trimmed := strings.TrimPrefix(strings.TrimSpace(encoded), "0x")
	word, err := hex.DecodeString(trimmed)
	if err != nil || len(word) != 32 {
		return nil, fmt.Errorf("Arc %s result is malformed", signature)
	}
	return word, nil
}

func decodeUint64Word(word []byte) (uint64, error) {
	value := new(big.Int).SetBytes(word)
	if !value.IsUint64() {
		return 0, fmt.Errorf("word exceeds uint64")
	}
	return value.Uint64(), nil
}

func decodeBoolWord(word []byte) (bool, error) {
	value, err := decodeUint64Word(word)
	if err != nil || value > 1 {
		return false, fmt.Errorf("word is not bool")
	}
	return value == 1, nil
}

func decodeInt32Word(word []byte) (int32, error) {
	value := new(big.Int).SetBytes(word)
	if word[0]&0x80 != 0 {
		value.Sub(value, new(big.Int).Lsh(big.NewInt(1), 256))
	}
	if !value.IsInt64() || value.Int64() < -1<<31 || value.Int64() > 1<<31-1 {
		return 0, fmt.Errorf("word exceeds int32")
	}
	return int32(value.Int64()), nil
}
