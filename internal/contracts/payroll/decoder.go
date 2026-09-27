package payroll

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type DecodedSameTokenPayroll struct {
	Token       string
	Recipients  []string
	Amounts     []*big.Int
	ReferenceID string
}

func DecodeSameTokenPayrollCall(data []byte) (DecodedSameTokenPayroll, error) {
	method, err := MethodBySignature(SigExecuteSameTokenPayroll)
	if err != nil {
		return DecodedSameTokenPayroll{}, err
	}
	if len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
		return DecodedSameTokenPayroll{}, fmt.Errorf("calldata selector is not executeSameTokenPayroll")
	}
	values, err := method.Inputs.Unpack(data[4:])
	if err != nil || len(values) != 4 {
		return DecodedSameTokenPayroll{}, fmt.Errorf("decode same-token payroll calldata: %w", err)
	}
	token, ok1 := values[0].(common.Address)
	recipients, ok2 := values[1].([]common.Address)
	amounts, ok3 := values[2].([]*big.Int)
	referenceID, ok4 := values[3].(string)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return DecodedSameTokenPayroll{}, fmt.Errorf("same-token payroll calldata has unexpected ABI types")
	}
	strings := make([]string, len(recipients))
	for i := range recipients {
		strings[i] = recipients[i].Hex()
	}
	out := DecodedSameTokenPayroll{Token: token.Hex(), Recipients: strings, Amounts: copyBigInts(amounts), ReferenceID: referenceID}
	if err := validateSameToken(SameTokenPayrollInput{Token: out.Token, Recipients: out.Recipients, Amounts: out.Amounts, ReferenceID: out.ReferenceID}); err != nil {
		return DecodedSameTokenPayroll{}, err
	}
	repacked, err := method.Inputs.Pack(token, recipients, amounts, referenceID)
	if err != nil || !bytes.Equal(data, append(append([]byte(nil), method.ID...), repacked...)) {
		return DecodedSameTokenPayroll{}, fmt.Errorf("same-token payroll calldata is not canonical")
	}
	return out, nil
}
