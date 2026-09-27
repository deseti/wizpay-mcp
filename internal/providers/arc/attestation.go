package arc

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

// DeploymentAttestation is a read-only observation of runtime bytecode at a
// canonical deployment address. Code presence is not proof of contract state,
// ownership, liquidity, or executable readiness.
type DeploymentAttestation struct {
	ContractID      contracts.ContractID
	RegistryVersion uint
	ChainID         string
	Network         string
	Address         string
	CodeSize        int
	CodeHash        string
}

// AttestDeploymentCode reads runtime bytecode only for an exact deployment in
// the static Mainnet registry. Callers cannot supply an arbitrary address.
func (c *Client) AttestDeploymentCode(ctx context.Context, id contracts.ContractID, version uint) (DeploymentAttestation, error) {
	deployment, err := contracts.DefaultRegistry().Require(id, version, contracts.ChainIDArcMainnet, contracts.NetworkArcMainnet)
	if err != nil {
		return DeploymentAttestation{}, fmt.Errorf("Arc deployment attestation target is not canonical: %w", err)
	}
	if err := c.ensureChainIdentity(ctx); err != nil {
		return DeploymentAttestation{}, err
	}
	var encoded string
	if err := c.call(ctx, "eth_getCode", []any{deployment.Address, "latest"}, &encoded); err != nil {
		return DeploymentAttestation{}, err
	}
	trimmed := strings.TrimPrefix(strings.TrimSpace(encoded), "0x")
	if trimmed == "" || len(trimmed)%2 != 0 {
		return DeploymentAttestation{}, fmt.Errorf("Arc deployment bytecode is absent or malformed")
	}
	code, err := hex.DecodeString(trimmed)
	if err != nil || len(code) == 0 {
		return DeploymentAttestation{}, fmt.Errorf("Arc deployment bytecode is absent or malformed")
	}
	return DeploymentAttestation{
		ContractID: id, RegistryVersion: version,
		ChainID: deployment.ChainID, Network: deployment.Network, Address: deployment.Address,
		CodeSize: len(code), CodeHash: crypto.Keccak256Hash(code).Hex(),
	}, nil
}
