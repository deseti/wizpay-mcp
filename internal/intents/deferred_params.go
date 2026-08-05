package intents

import (
	"fmt"
	"strings"
)

// BridgeParameters and ANSParameters remain in the closed financial union for
// future phases. Phase 12 Step 1 does not implement Bridge/ANS execution.

type BridgeParameters struct {
	SourceChainID      string `json:"source_chain_id"`
	DestinationChainID string `json:"destination_chain_id"`
	SourceToken        Token  `json:"source_token"`
	SourceAmount       Amount `json:"source_amount"`
	DestinationAmount  Amount `json:"destination_amount"`
	DestinationAddress string `json:"destination_address"`
	PlanReference      string `json:"plan_reference"`
}

type ANSParameters struct {
	NormalizedName string `json:"normalized_name"`
	NameVersion    string `json:"name_version"`
	TermSeconds    uint64 `json:"term_seconds"`
	Controller     string `json:"controller"`
	CostToken      Token  `json:"cost_token"`
	Cost           Amount `json:"cost"`
}

func (p BridgeParameters) validate() error {
	if err := validateChainID(p.SourceChainID); err != nil {
		return err
	}
	if err := validateChainID(p.DestinationChainID); err != nil {
		return err
	}
	if p.SourceChainID == p.DestinationChainID {
		return fmt.Errorf("bridge source and destination chains must differ")
	}
	if err := p.SourceToken.Validate(); err != nil {
		return err
	}
	if p.SourceToken.ChainID != p.SourceChainID {
		return fmt.Errorf("bridge source token chain does not match source chain")
	}
	if err := p.SourceAmount.Validate(); err != nil {
		return err
	}
	if err := p.DestinationAmount.Validate(); err != nil {
		return err
	}
	if p.SourceAmount.Decimals != p.SourceToken.Decimals {
		return fmt.Errorf("bridge source amount decimals do not match token")
	}
	if err := validateText("destination address", p.DestinationAddress); err != nil {
		return err
	}
	return validateText("bridge plan reference", p.PlanReference)
}

func (p ANSParameters) validate() error {
	if err := validateText("normalized name", p.NormalizedName); err != nil {
		return err
	}
	if p.NormalizedName != strings.ToLower(p.NormalizedName) || strings.TrimSuffix(p.NormalizedName, ".") != p.NormalizedName {
		return fmt.Errorf("ANS name must be normalized lowercase without a trailing dot")
	}
	if err := validateText("name version", p.NameVersion); err != nil {
		return err
	}
	if p.TermSeconds == 0 {
		return fmt.Errorf("ANS term must be positive")
	}
	if err := validateText("ANS controller", p.Controller); err != nil {
		return err
	}
	if err := p.CostToken.Validate(); err != nil {
		return err
	}
	if err := p.Cost.Validate(); err != nil {
		return err
	}
	if p.Cost.Decimals != p.CostToken.Decimals {
		return fmt.Errorf("ANS cost decimals do not match token")
	}
	return nil
}
