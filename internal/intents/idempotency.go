package intents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

const operationDomain = "WIZPAY_MCP_OPERATION_V1\n"

// OperationIdentityVersion versions the logical-operation key derivation.
const OperationIdentityVersion uint64 = 1

// OperationIdentity is the stable logical-operation key for retries of one
// frozen intent. It is not a transaction, provider request, or submission ID.
type OperationIdentity struct {
	intentID      string
	intentVersion uint64
	intentDigest  string
	operationKey  string
	version       uint64
}

func NewOperationIdentity(intent Intent) (OperationIdentity, error) {
	if err := intent.Validate(); err != nil {
		return OperationIdentity{}, err
	}
	if intent.Status() == StatusDraft || intent.Digest() == "" {
		return OperationIdentity{}, apperrors.New(apperrors.CodeValidationError, "Intent must be frozen before assigning an operation identity.", false, true, true)
	}
	material := fmt.Sprintf("%s\n%d\n%s", intent.IntentID(), intent.Version(), intent.Digest())
	sum := sha256.Sum256([]byte(operationDomain + material))
	return OperationIdentity{intent.IntentID(), intent.Version(), intent.Digest(), hex.EncodeToString(sum[:]), OperationIdentityVersion}, nil
}

func (o OperationIdentity) EnsureMatches(intent Intent) error {
	other, err := NewOperationIdentity(intent)
	if err != nil {
		return err
	}
	if o.intentID != other.intentID || o.intentVersion != other.intentVersion || o.intentDigest != other.intentDigest || o.operationKey != other.operationKey || o.version != other.version {
		return apperrors.New(apperrors.CodeIntentMutated, "Operation identity does not match the frozen intent.", false, true, true)
	}
	return nil
}

func (o OperationIdentity) IntentID() string      { return o.intentID }
func (o OperationIdentity) IntentVersion() uint64 { return o.intentVersion }
func (o OperationIdentity) IntentDigest() string  { return o.intentDigest }
func (o OperationIdentity) OperationKey() string  { return o.operationKey }
func (o OperationIdentity) Version() uint64       { return o.version }
