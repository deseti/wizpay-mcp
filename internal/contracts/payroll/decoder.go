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

type DecodedCrossTokenPayroll struct {
	TokenIn, TokenOut                                 string
	Recipients                                        []string
	OutputAmounts                                     []*big.Int
	GrossInput, MinTotalOut, MinHopPriceX36, Deadline *big.Int
	ReferenceID                                       string
}

func DecodeCrossTokenPayrollCall(data []byte) (DecodedCrossTokenPayroll, error) {
	method, err := MethodBySignature(SigExecuteCrossTokenPayroll)
	if err != nil {
		return DecodedCrossTokenPayroll{}, err
	}
	if len(data) < 4 || !bytes.Equal(data[:4], method.ID) {
		return DecodedCrossTokenPayroll{}, fmt.Errorf("calldata selector is not executeCrossTokenPayroll")
	}
	v, err := method.Inputs.Unpack(data[4:])
	if err != nil || len(v) != 9 {
		return DecodedCrossTokenPayroll{}, fmt.Errorf("decode cross-token payroll calldata: %w", err)
	}
	tokenIn, a := v[0].(common.Address)
	tokenOut, b := v[1].(common.Address)
	recipients, c := v[2].([]common.Address)
	amounts, d := v[3].([]*big.Int)
	gross, e := v[4].(*big.Int)
	minimum, f := v[5].(*big.Int)
	price, g := v[6].(*big.Int)
	deadline, h := v[7].(*big.Int)
	reference, i := v[8].(string)
	if !a || !b || !c || !d || !e || !f || !g || !h || !i {
		return DecodedCrossTokenPayroll{}, fmt.Errorf("cross-token payroll calldata has unexpected ABI types")
	}
	addresses := make([]string, len(recipients))
	for j := range recipients {
		addresses[j] = recipients[j].Hex()
	}
	out := DecodedCrossTokenPayroll{TokenIn: tokenIn.Hex(), TokenOut: tokenOut.Hex(), Recipients: addresses, OutputAmounts: copyBigInts(amounts), GrossInput: cloneInt(gross), MinTotalOut: cloneInt(minimum), MinHopPriceX36: cloneInt(price), Deadline: cloneInt(deadline), ReferenceID: reference}
	if err := validateCrossToken(CrossTokenPayrollInput{TokenIn: out.TokenIn, TokenOut: out.TokenOut, Recipients: out.Recipients, OutputAmounts: out.OutputAmounts, GrossInput: out.GrossInput, MinTotalOut: out.MinTotalOut, MinHopPriceX36: out.MinHopPriceX36, Deadline: out.Deadline, ReferenceID: out.ReferenceID}); err != nil {
		return DecodedCrossTokenPayroll{}, err
	}
	repacked, err := method.Inputs.Pack(tokenIn, tokenOut, recipients, amounts, gross, minimum, price, deadline, reference)
	if err != nil || !bytes.Equal(data, append(append([]byte(nil), method.ID...), repacked...)) {
		return DecodedCrossTokenPayroll{}, fmt.Errorf("cross-token payroll calldata is not canonical")
	}
	return out, nil
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
