package payroll

import (
	"fmt"

	"github.com/deseti/wizpay-mcp/internal/contracts"
)

// EventTopic0 returns the exact topic hash for a reviewed Mainnet payroll
// verification event. Unknown and administrative events fail closed.
func EventTopic0(signature string) ([]byte, error) {
	deployment, err := ExpectedDeployment(nil)
	if err != nil {
		return nil, err
	}
	if !deployment.AllowsEvent(signature) {
		return nil, fmt.Errorf("payroll event is not allowlisted")
	}
	event, err := EventBySignature(signature)
	if err != nil {
		return nil, err
	}
	return event.ID.Bytes(), nil
}

// ValidateEventLog validates immutable chain/address/topic context only. Track
// A does not activate payroll settlement verification.
func ValidateEventLog(registry *contracts.Registry, signature string, log contracts.Log) error {
	deployment, err := ExpectedDeployment(registry)
	if err != nil {
		return err
	}
	if !deployment.AllowsEvent(signature) {
		return fmt.Errorf("payroll event is not allowlisted")
	}
	if log.ChainID != "" && log.ChainID != deployment.ChainID {
		return fmt.Errorf("log chain ID does not match payroll deployment")
	}
	if !contracts.AddressesEqual(log.Address, deployment.Address) {
		return fmt.Errorf("log address does not match payroll deployment")
	}
	topic0, err := EventTopic0(signature)
	if err != nil {
		return err
	}
	if len(log.Topics) == 0 || !topicsEqual(log.Topics[0], topic0) {
		return fmt.Errorf("payroll event topic mismatch")
	}
	return nil
}

func topicsEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
