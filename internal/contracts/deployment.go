// Package contracts provides typed, allowlisted contract deployment metadata
// and encoding primitives for the verified Arc Testnet Payroll and Swap
// contracts. It never submits transactions, never signs, and never exposes a
// generic arbitrary-call executor.
//
// RegistryVersion is MCP-side artifact metadata. It is NOT a Solidity contract
// semantic version unless a deployment explicitly exposes one (these do not).
package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ContractID identifies a verified on-chain contract role in the MCP registry.
type ContractID string

const (
	// ContractWizPayPayroll is the WizPay payroll routing contract.
	ContractWizPayPayroll ContractID = "WIZPAY_PAYROLL"
	// ContractWizPaySwapExecutor is the WizPay swap executor contract.
	ContractWizPaySwapExecutor ContractID = "WIZPAY_SWAP_EXECUTOR"
)

func (id ContractID) Valid() bool {
	switch id {
	case ContractWizPayPayroll, ContractWizPaySwapExecutor:
		return true
	default:
		return false
	}
}

// RegistryVersion is the MCP-side deployment descriptor version.
//
// RegistryVersion != Solidity contract semantic version. The verified Arc
// Testnet deployments do not expose an on-chain semantic version, so this
// value versions only the MCP artifact metadata (address, allowlists, chain).
const RegistryVersion uint = 1

// Arc Testnet constants shared by registered deployments.
const (
	ChainIDArcTestnet = "5042002"
	NetworkArcTestnet = "TESTNET"
	NetworkNameArc    = "Arc Testnet"
)

// Known Arc Testnet deployment addresses (authoritative for this phase).
const (
	AddressWizPayPayroll      = "0x87ACE45582f45cC81AC1E627E875AE84cbd75946"
	AddressWizPaySwapExecutor = "0x17685466759f9Cde06f0DCbB5464164ABe541eFA"
)

// Status is the enablement status of a deployment descriptor.
//
// ENABLED means the descriptor is registered for encoding/decoding support.
// It does not enable Phase 10 capability availability, does not authorize
// money movement, and does not imply a live transaction may be performed.
type Status string

const (
	StatusEnabled  Status = "ENABLED"
	StatusDisabled Status = "DISABLED"
)

func (status Status) Valid() bool {
	switch status {
	case StatusEnabled, StatusDisabled:
		return true
	default:
		return false
	}
}

// Deployment is an immutable, versioned contract deployment descriptor.
//
// The target contract address is owned exclusively by this descriptor. Callers
// cannot supply an alternate destination address for encoding.
type Deployment struct {
	ID                 ContractID `json:"id"`
	RegistryVersion    uint       `json:"registry_version"`
	Name               string     `json:"name"`
	ChainID            string     `json:"chain_id"`
	Network            string     `json:"network"`
	Address            string     `json:"address"`
	ExecutionFunctions []string   `json:"execution_functions"`
	ReadFunctions      []string   `json:"read_functions"`
	VerificationEvents []string   `json:"verification_events"`
	Status             Status     `json:"status"`
	ABISource          string     `json:"abi_source"`
	SourceContract     string     `json:"source_contract,omitempty"`
	Notes              string     `json:"notes,omitempty"`
}

// Validate checks structural invariants of a deployment descriptor.
func (d Deployment) Validate() error {
	if !d.ID.Valid() {
		return fmt.Errorf("invalid contract ID %q", d.ID)
	}
	if d.RegistryVersion == 0 {
		return fmt.Errorf("registry version must be positive")
	}
	if err := validateSafeText("contract name", d.Name); err != nil {
		return err
	}
	if d.ChainID != ChainIDArcTestnet {
		return fmt.Errorf("chain ID %q is not the supported Arc Testnet chain", d.ChainID)
	}
	if d.Network != NetworkArcTestnet {
		return fmt.Errorf("network %q is not the supported Arc Testnet network label", d.Network)
	}
	if !ValidAddress(d.Address) {
		return fmt.Errorf("deployment address is malformed")
	}
	if !d.Status.Valid() {
		return fmt.Errorf("invalid deployment status %q", d.Status)
	}
	if err := validateSafeText("ABI source", d.ABISource); err != nil {
		return err
	}
	if len(d.ExecutionFunctions) == 0 {
		return fmt.Errorf("deployment must declare at least one execution function")
	}
	if err := validateUniqueStrings(d.ExecutionFunctions, "execution function"); err != nil {
		return err
	}
	if err := validateUniqueStrings(d.ReadFunctions, "read function"); err != nil {
		return err
	}
	if err := validateUniqueStrings(d.VerificationEvents, "verification event"); err != nil {
		return err
	}
	if err := d.validateAddressMatchesID(); err != nil {
		return err
	}
	if err := d.validateCanonicalAllowlists(); err != nil {
		return err
	}
	return nil
}

func (d Deployment) validateAddressMatchesID() error {
	normalized := NormalizeAddress(d.Address)
	switch d.ID {
	case ContractWizPayPayroll:
		if normalized != NormalizeAddress(AddressWizPayPayroll) {
			return fmt.Errorf("WIZPAY_PAYROLL address does not match the verified Arc Testnet deployment")
		}
		if d.Name != "WizPay" {
			return fmt.Errorf("WIZPAY_PAYROLL name must be WizPay")
		}
	case ContractWizPaySwapExecutor:
		if normalized != NormalizeAddress(AddressWizPaySwapExecutor) {
			return fmt.Errorf("WIZPAY_SWAP_EXECUTOR address does not match the verified Arc Testnet deployment")
		}
		if d.Name != "WizPaySwapExecutor" {
			return fmt.Errorf("WIZPAY_SWAP_EXECUTOR name must be WizPaySwapExecutor")
		}
	}
	return nil
}

// validateCanonicalAllowlists requires exact set equality with the hard-coded
// allowlists for each known ContractID. Order does not matter; adding,
// removing, or substituting any signature fails closed.
func (d Deployment) validateCanonicalAllowlists() error {
	switch d.ID {
	case ContractWizPayPayroll:
		if !sameStringSet(d.ExecutionFunctions, canonicalPayrollExecution) {
			return fmt.Errorf("WIZPAY_PAYROLL execution function allowlist does not match the canonical set")
		}
		if !sameStringSet(d.ReadFunctions, canonicalPayrollReads) {
			return fmt.Errorf("WIZPAY_PAYROLL read function allowlist does not match the canonical set")
		}
		if !sameStringSet(d.VerificationEvents, canonicalPayrollEvents) {
			return fmt.Errorf("WIZPAY_PAYROLL verification event allowlist does not match the canonical set")
		}
	case ContractWizPaySwapExecutor:
		if !sameStringSet(d.ExecutionFunctions, canonicalSwapExecution) {
			return fmt.Errorf("WIZPAY_SWAP_EXECUTOR execution function allowlist does not match the canonical set")
		}
		if !sameStringSet(d.ReadFunctions, canonicalSwapReads) {
			return fmt.Errorf("WIZPAY_SWAP_EXECUTOR read function allowlist does not match the canonical set")
		}
		if !sameStringSet(d.VerificationEvents, canonicalSwapEvents) {
			return fmt.Errorf("WIZPAY_SWAP_EXECUTOR verification event allowlist does not match the canonical set")
		}
	default:
		return fmt.Errorf("unknown contract ID %q", d.ID)
	}
	return nil
}

// Normalized returns a defensive copy with sorted allowlists and a checksum
// address. Mutations of the returned value never affect the registry.
func (d Deployment) Normalized() Deployment {
	out := d
	out.Address = ChecksumAddress(d.Address)
	out.ExecutionFunctions = append([]string(nil), d.ExecutionFunctions...)
	out.ReadFunctions = append([]string(nil), d.ReadFunctions...)
	out.VerificationEvents = append([]string(nil), d.VerificationEvents...)
	sort.Strings(out.ExecutionFunctions)
	sort.Strings(out.ReadFunctions)
	sort.Strings(out.VerificationEvents)
	return out
}

// Digest is a deterministic identity over the normalized descriptor metadata.
func (d Deployment) Digest() string {
	canonical, _ := json.Marshal(d.Normalized())
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// AllowsExecution reports whether the canonical function signature is on the
// execution allowlist. Admin and unlisted functions return false.
func (d Deployment) AllowsExecution(signature string) bool {
	for _, candidate := range d.ExecutionFunctions {
		if candidate == signature {
			return true
		}
	}
	return false
}

// AllowsRead reports whether the canonical function signature is on the
// read-only allowlist.
func (d Deployment) AllowsRead(signature string) bool {
	for _, candidate := range d.ReadFunctions {
		if candidate == signature {
			return true
		}
	}
	return false
}

// AllowsEvent reports whether the event signature is on the verification allowlist.
func (d Deployment) AllowsEvent(signature string) bool {
	for _, candidate := range d.VerificationEvents {
		if candidate == signature {
			return true
		}
	}
	return false
}

// EncodedCall is a typed contract call primitive produced only through
// NewEncodedCall after deployment and allowlist validation. Material fields are
// unexported so external packages cannot construct arbitrary targets,
// functions, selectors, or calldata via a public struct literal.
//
// EncodedCall is calldata only. Producing one never submits a transaction and
// never implies financial success.
type EncodedCall struct {
	contractID      ContractID
	registryVersion uint
	chainID         string
	network         string
	to              string
	function        string
	selector        [4]byte
	callData        []byte
}

// NewEncodedCall constructs an EncodedCall from a validated deployment and an
// allowlisted execution function. The destination address is taken exclusively
// from the deployment; callers cannot supply an alternate target address.
//
// Construction fails closed unless:
//   - the deployment validates (including canonical allowlists)
//   - function is on the deployment execution allowlist
//   - selector matches the canonical function signature
//   - callData starts with that selector
func NewEncodedCall(deployment Deployment, function string, selector [4]byte, callData []byte) (EncodedCall, error) {
	if err := deployment.Validate(); err != nil {
		return EncodedCall{}, fmt.Errorf("encoded call deployment is invalid: %w", err)
	}
	if !deployment.AllowsExecution(function) {
		return EncodedCall{}, fmt.Errorf("function %q is not an allowlisted execution function", function)
	}
	want := Selector4(function)
	if selector != want {
		return EncodedCall{}, fmt.Errorf("selector does not match function signature")
	}
	if len(callData) < 4 {
		return EncodedCall{}, fmt.Errorf("call data is too short")
	}
	if callData[0] != selector[0] || callData[1] != selector[1] || callData[2] != selector[2] || callData[3] != selector[3] {
		return EncodedCall{}, fmt.Errorf("call data does not begin with the function selector")
	}
	return newEncodedCall(deployment, function, selector, callData), nil
}

// newEncodedCall is the unexported constructor used after validation. It always
// copies callData and always sets To from the deployment address.
func newEncodedCall(deployment Deployment, function string, selector [4]byte, callData []byte) EncodedCall {
	return EncodedCall{
		contractID:      deployment.ID,
		registryVersion: deployment.RegistryVersion,
		chainID:         deployment.ChainID,
		network:         deployment.Network,
		to:              deployment.Address,
		function:        function,
		selector:        selector,
		callData:        append([]byte(nil), callData...),
	}
}

// ContractID returns the registered contract identity for this call.
func (c EncodedCall) ContractID() ContractID { return c.contractID }

// RegistryVersion returns the MCP artifact registry version for this call.
func (c EncodedCall) RegistryVersion() uint { return c.registryVersion }

// ChainID returns the chain ID for this call.
func (c EncodedCall) ChainID() string { return c.chainID }

// Network returns the network label for this call.
func (c EncodedCall) Network() string { return c.network }

// To returns the destination contract address from the deployment registry.
func (c EncodedCall) To() string { return c.to }

// Function returns the canonical allowlisted function signature.
func (c EncodedCall) Function() string { return c.function }

// Selector returns the 4-byte function selector.
func (c EncodedCall) Selector() [4]byte { return c.selector }

// CallData returns a defensive copy of the ABI-encoded call data.
func (c EncodedCall) CallData() []byte {
	if c.callData == nil {
		return nil
	}
	return append([]byte(nil), c.callData...)
}

// Clone returns a deep copy of the EncodedCall, including CallData.
func (c EncodedCall) Clone() EncodedCall {
	return EncodedCall{
		contractID:      c.contractID,
		registryVersion: c.registryVersion,
		chainID:         c.chainID,
		network:         c.network,
		to:              c.to,
		function:        c.function,
		selector:        c.selector,
		callData:        c.CallData(),
	}
}

// Log is a minimal receipt log shape used by event decoders. It carries no
// provider secrets and does not assert financial success by itself.
type Log struct {
	Address string
	Topics  [][]byte
	Data    []byte
	ChainID string
}

func validateUniqueStrings(values []string, name string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("invalid %s %q", name, value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate %s %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateSafeText(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must be non-empty trimmed text", name)
	}
	if len(value) > 256 {
		return fmt.Errorf("%s exceeds 256 characters", name)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}
