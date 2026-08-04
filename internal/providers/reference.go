package providers

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const (
	// maxReferenceLength bounds the durable adapter reference including optional
	// receipt-observation fields used for restart-safe reorg detection.
	maxReferenceLength = 512
	maxSafeTextLength  = 256
	maxChainIDLength   = 20
	// referencePrefix versions the encoding so a future reference shape can be
	// distinguished from this one without ambiguity.
	referencePrefix = "wzp1"
)

// Reference is the safe, non-secret provider reference persisted as the Phase 9
// execution.Result adapter reference. It carries reconciliation identifiers
// only: never an API key, user token, request body, or response body.
//
// Optional receipt-observation fields (Obs*) carry the last known inclusion
// identity for reorg detection across worker restarts. They are observations,
// not verified financial success.
type Reference struct {
	Provider ProviderID
	ChainID  string
	// WalletID is the user's non-secret provider wallet identifier. It is
	// persisted because reconciliation cannot locate the transaction without
	// it after a restart. It confers no authority over the wallet.
	WalletID string
	// ChallengeID is a non-secret provider challenge identifier. A created
	// challenge is not a financial submission.
	ChallengeID string
	// ProviderTransactionID is the provider's transaction identifier.
	ProviderTransactionID string
	// TransactionHash is the on-chain transaction hash, once the provider
	// reports one. Only this field can lead to on-chain verification.
	TransactionHash string

	// Receipt observation metadata (optional, durable reorg baseline).
	ObsPresent       bool
	ObsKnown         bool
	ObsBlockHash     string
	ObsBlockNumber   uint64
	ObsConfirmations uint64
}

func (r Reference) Validate() error {
	if !r.Provider.Valid() {
		return fmt.Errorf("provider reference has an invalid provider")
	}
	if !validChainID(r.ChainID) {
		return fmt.Errorf("provider reference has an invalid chain ID")
	}
	if r.ChallengeID == "" && r.ProviderTransactionID == "" && r.TransactionHash == "" {
		return fmt.Errorf("provider reference requires at least one provider identifier")
	}
	for name, value := range map[string]string{"challenge ID": r.ChallengeID, "provider transaction ID": r.ProviderTransactionID, "wallet ID": r.WalletID} {
		if value != "" && !validProviderIdentifier(value) {
			return fmt.Errorf("provider reference has an invalid %s", name)
		}
	}
	if r.TransactionHash != "" && !ValidTransactionHash(r.TransactionHash) {
		return fmt.Errorf("provider reference has an invalid transaction hash")
	}
	if r.ObsBlockHash != "" && !ValidTransactionHash(r.ObsBlockHash) {
		// Block hashes use the same 32-byte 0x-hex shape as transaction hashes.
		return fmt.Errorf("provider reference has an invalid observation block hash")
	}
	if err := r.validateObservationMetadata(); err != nil {
		return err
	}
	return nil
}

// hasObservationMetadata reports whether any durable receipt-observation field
// is set on the reference.
func (r Reference) hasObservationMetadata() bool {
	return r.ObsKnown || r.ObsPresent || r.ObsBlockHash != "" || r.ObsBlockNumber != 0 || r.ObsConfirmations != 0
}

func (r Reference) validateObservationMetadata() error {
	if !r.hasObservationMetadata() {
		return nil
	}
	// Observation metadata is always about a specific transaction inclusion.
	if r.TransactionHash == "" || !ValidTransactionHash(r.TransactionHash) {
		return fmt.Errorf("provider reference observation metadata requires a valid transaction hash")
	}
	// Present implies known inclusion history.
	if r.ObsPresent && !r.ObsKnown {
		return fmt.Errorf("provider reference observation present without known flag")
	}
	// Confirmations and block identity require a known observation baseline.
	if !r.ObsKnown {
		return fmt.Errorf("provider reference observation fields require a known observation")
	}
	return nil
}

// normalizeObservationMetadata applies deterministic observation consistency
// after parse/encode helpers construct a reference.
func (r *Reference) normalizeObservationMetadata() {
	if r.ObsPresent {
		r.ObsKnown = true
	}
	if r.ObsBlockHash != "" || r.ObsBlockNumber != 0 {
		r.ObsKnown = true
		r.ObsBlockHash = strings.ToLower(strings.TrimSpace(r.ObsBlockHash))
	}
}

// Encode returns the canonical single-line encoding stored as the adapter
// reference. Empty optional fields are omitted so the encoding stays stable.
func (r Reference) Encode() (string, error) {
	r.normalizeObservationMetadata()
	if err := r.Validate(); err != nil {
		return "", err
	}
	parts := []string{referencePrefix, "p=" + string(r.Provider), "chain=" + r.ChainID}
	if r.WalletID != "" {
		parts = append(parts, "w="+r.WalletID)
	}
	if r.ChallengeID != "" {
		parts = append(parts, "ch="+r.ChallengeID)
	}
	if r.ProviderTransactionID != "" {
		parts = append(parts, "tx="+r.ProviderTransactionID)
	}
	if r.TransactionHash != "" {
		parts = append(parts, "hash="+r.TransactionHash)
	}
	if r.ObsKnown || r.ObsPresent || r.ObsBlockHash != "" || r.ObsBlockNumber != 0 {
		if r.ObsPresent {
			parts = append(parts, "rp=1")
		} else {
			parts = append(parts, "rp=0")
		}
		if r.ObsKnown || r.ObsPresent {
			parts = append(parts, "rk=1")
		}
		if r.ObsBlockHash != "" {
			parts = append(parts, "bh="+strings.ToLower(r.ObsBlockHash))
		}
		if r.ObsBlockNumber != 0 {
			parts = append(parts, "bn="+strconv.FormatUint(r.ObsBlockNumber, 10))
		}
		if r.ObsConfirmations != 0 {
			parts = append(parts, "cf="+strconv.FormatUint(r.ObsConfirmations, 10))
		}
	}
	encoded := strings.Join(parts, ";")
	if len(encoded) > maxReferenceLength {
		return "", fmt.Errorf("provider reference exceeds %d characters", maxReferenceLength)
	}
	return encoded, nil
}

// ReceiptObservation reconstructs durable reorg baseline metadata from this
// reference. Empty when no observation fields were persisted.
func (r Reference) ReceiptObservation() ReceiptObservation {
	if !r.ObsKnown && !r.ObsPresent && r.ObsBlockHash == "" && r.ObsBlockNumber == 0 {
		return ReceiptObservation{}
	}
	return ReceiptObservation{
		ChainID:         r.ChainID,
		TransactionHash: r.TransactionHash,
		Present:         r.ObsPresent,
		Known:           r.ObsKnown || r.ObsPresent || r.ObsBlockHash != "" || r.ObsBlockNumber != 0,
		BlockHash:       strings.ToLower(strings.TrimSpace(r.ObsBlockHash)),
		BlockNumber:     r.ObsBlockNumber,
		Confirmations:   r.ObsConfirmations,
	}
}

// WithReceiptObservation returns a copy carrying durable receipt observation
// metadata for reorg detection after restart.
func (r Reference) WithReceiptObservation(observation ReceiptObservation) Reference {
	next := r
	next.ObsPresent = observation.Present
	next.ObsKnown = observation.Known || observation.Present || observation.BlockHash != "" || observation.BlockNumber != 0
	next.ObsBlockHash = strings.ToLower(strings.TrimSpace(observation.BlockHash))
	next.ObsBlockNumber = observation.BlockNumber
	next.ObsConfirmations = observation.Confirmations
	if next.TransactionHash == "" && observation.TransactionHash != "" {
		next.TransactionHash = strings.ToLower(strings.TrimSpace(observation.TransactionHash))
	}
	next.normalizeObservationMetadata()
	return next
}

// WithTransaction returns a copy carrying provider transaction identifiers. It
// never clears an identifier that is already known.
func (r Reference) WithTransaction(providerTransactionID, transactionHash string) Reference {
	next := r
	if providerTransactionID != "" {
		next.ProviderTransactionID = providerTransactionID
	}
	if transactionHash != "" {
		next.TransactionHash = strings.ToLower(transactionHash)
	}
	return next
}

// ParseReference decodes a persisted adapter reference. It fails closed on any
// unknown prefix, unknown field, duplicate field, or malformed identifier.
func ParseReference(value string) (Reference, error) {
	if value == "" || len(value) > maxReferenceLength {
		return Reference{}, fmt.Errorf("provider reference is malformed")
	}
	segments := strings.Split(value, ";")
	if segments[0] != referencePrefix {
		return Reference{}, fmt.Errorf("provider reference has an unsupported encoding")
	}
	var reference Reference
	seen := make(map[string]struct{}, len(segments))
	for _, segment := range segments[1:] {
		key, fieldValue, found := strings.Cut(segment, "=")
		if !found || fieldValue == "" {
			return Reference{}, fmt.Errorf("provider reference is malformed")
		}
		if _, duplicate := seen[key]; duplicate {
			return Reference{}, fmt.Errorf("provider reference repeats field %q", key)
		}
		seen[key] = struct{}{}
		switch key {
		case "p":
			reference.Provider = ProviderID(fieldValue)
		case "chain":
			reference.ChainID = fieldValue
		case "w":
			reference.WalletID = fieldValue
		case "ch":
			reference.ChallengeID = fieldValue
		case "tx":
			reference.ProviderTransactionID = fieldValue
		case "hash":
			reference.TransactionHash = fieldValue
		case "rp":
			switch fieldValue {
			case "0":
				reference.ObsPresent = false
			case "1":
				reference.ObsPresent = true
			default:
				return Reference{}, fmt.Errorf("provider reference has an invalid observation present flag")
			}
		case "rk":
			if fieldValue != "1" {
				return Reference{}, fmt.Errorf("provider reference has an invalid observation known flag")
			}
			reference.ObsKnown = true
		case "bh":
			reference.ObsBlockHash = fieldValue
		case "bn":
			parsed, err := parseUintField(fieldValue)
			if err != nil {
				return Reference{}, fmt.Errorf("provider reference has an invalid observation block number")
			}
			reference.ObsBlockNumber = parsed
		case "cf":
			parsed, err := parseUintField(fieldValue)
			if err != nil {
				return Reference{}, fmt.Errorf("provider reference has an invalid observation confirmations")
			}
			reference.ObsConfirmations = parsed
		default:
			return Reference{}, fmt.Errorf("provider reference has an unknown field %q", key)
		}
	}
	reference.normalizeObservationMetadata()
	if err := reference.Validate(); err != nil {
		return Reference{}, err
	}
	return reference, nil
}

// ValidTransactionHash reports whether the value is a lowercase 32-byte
// 0x-prefixed EVM transaction hash.
func ValidTransactionHash(value string) bool {
	if len(value) != 66 || !strings.HasPrefix(value, "0x") {
		return false
	}
	return lowerHex(value[2:])
}

// ValidAddress reports whether the value is a 20-byte 0x-prefixed EVM address.
// Comparison elsewhere must be case-insensitive because checksum casing is
// address metadata, not identity.
func ValidAddress(value string) bool {
	if len(value) != 42 || !strings.HasPrefix(value, "0x") {
		return false
	}
	for _, character := range value[2:] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}

func lowerHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

// validProviderIdentifier accepts the conservative character set shared by
// provider-assigned UUID identifiers. It rejects separators used by the
// reference encoding so a hostile identifier cannot forge extra fields.
func validProviderIdentifier(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') && character != '-' {
			return false
		}
	}
	return true
}

func validChainID(value string) bool {
	if value == "" || len(value) > maxChainIDLength || value[0] == '0' {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validSafeText(value string) bool {
	if value == "" || len(value) > maxSafeTextLength || strings.TrimSpace(value) != value {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

// parseUintField parses a canonical decimal uint64. It fails closed on empty
// input, non-digits, non-canonical leading zeros, and values above MaxUint64.
func parseUintField(value string) (uint64, error) {
	if value == "" {
		return 0, fmt.Errorf("invalid unsigned integer")
	}
	// Canonical form: "0" or a non-zero digit followed only by digits.
	if value[0] == '0' {
		if len(value) != 1 {
			return 0, fmt.Errorf("invalid unsigned integer")
		}
	} else {
		for _, character := range value {
			if character < '0' || character > '9' {
				return 0, fmt.Errorf("invalid unsigned integer")
			}
		}
	}
	// strconv.ParseUint fails closed on overflow beyond MaxUint64.
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid unsigned integer")
	}
	return parsed, nil
}
