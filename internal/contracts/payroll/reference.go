package payroll

import (
	"fmt"
	"math/big"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

// ReferenceHash reproduces keccak256(abi.encode(chainId, payroll, employer,
// referenceId)), the contract's global per-employer replay domain.
func ReferenceHash(employer, referenceID string) ([32]byte, error) {
	if !contracts.ValidAddress(employer) || referenceID == "" {
		return [32]byte{}, fmt.Errorf("reference hash material is invalid")
	}
	uintType, _ := ethabi.NewType("uint256", "", nil)
	addressType, _ := ethabi.NewType("address", "", nil)
	stringType, _ := ethabi.NewType("string", "", nil)
	encoded, err := (ethabi.Arguments{{Type: uintType}, {Type: addressType}, {Type: addressType}, {Type: stringType}}).Pack(big.NewInt(5042), common.HexToAddress(contracts.AddressWizPayPayroll), common.HexToAddress(employer), referenceID)
	if err != nil {
		return [32]byte{}, err
	}
	return [32]byte(crypto.Keccak256Hash(encoded)), nil
}
