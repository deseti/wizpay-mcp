// Package siwe implements a restricted server-issued EIP-4361 EOA profile.
// It never parses caller-provided messages or invokes a blockchain provider.
package siwe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var ErrDenied = errors.New("wallet authentication denied")

const Domain = "connect.wizpay.xyz"
const URI = "https://connect.wizpay.xyz"
const Lifetime = 2 * time.Minute

type Challenge struct {
	ID, Nonce, Message, Address, SessionReference, TransactionID, ClientID, TenantID string
	IssuedAt, ExpiresAt                                                              time.Time
}
type Request struct{ SessionDigest, CSRFDigest, TenantID, Address string }
type Rotation struct{ Digest, CSRFHash, Reference string }
type Repository interface {
	IssueSIWE(context.Context, Request) (Challenge, error)
	FinalizeSIWE(context.Context, Request, string, string, Rotation) error
}

// Tenant is trusted process configuration, never browser input. Construction is
// deliberately not wired into production bootstrap in this work package.
type Service struct {
	repo   Repository
	tenant string
}

func NewService(repo Repository, tenant, origin string) (*Service, error) {
	if origin != URI || repo == nil || strings.TrimSpace(tenant) != tenant || tenant == "" || len(tenant) > 256 {
		return nil, ErrDenied
	}
	return &Service{repo, tenant}, nil
}
func (s *Service) Issue(ctx context.Context, digest, proof, address string) (Challenge, error) {
	a, e := Address(address)
	if e != nil {
		return Challenge{}, e
	}
	return s.repo.IssueSIWE(ctx, Request{digest, proof, s.tenant, a})
}
func (s *Service) Finalize(ctx context.Context, digest, proof, id, signature string, r Rotation) error {
	if len(id) != 64 || len(signature) != 132 {
		return ErrDenied
	}
	return s.repo.FinalizeSIWE(ctx, Request{SessionDigest: digest, CSRFDigest: proof, TenantID: s.tenant}, id, signature, r)
}
func Random() (string, error) {
	var b [32]byte
	_, e := rand.Read(b[:])
	return hex.EncodeToString(b[:]), e
}

// Lower/upper-case input is accepted; mixed case must carry a valid EIP-55 checksum.
func Address(raw string) (string, error) {
	if len(raw) != 42 || !strings.HasPrefix(raw, "0x") || !common.IsHexAddress(raw) {
		return "", ErrDenied
	}
	a := common.HexToAddress(raw).Hex()
	text := raw[2:]
	if text != strings.ToLower(text) && text != strings.ToUpper(text) && raw != a {
		return "", ErrDenied
	}
	return a, nil
}
func Message(c Challenge) (string, error) {
	a, e := Address(c.Address)
	if e != nil || a != c.Address || len(c.Nonce) != 64 || c.IssuedAt.IsZero() || !c.ExpiresAt.After(c.IssuedAt) || c.ExpiresAt.Sub(c.IssuedAt) > Lifetime {
		return "", ErrDenied
	}
	if _, e = hex.DecodeString(c.Nonce); e != nil {
		return "", ErrDenied
	}
	return fmt.Sprintf("%s wants you to sign in with your Ethereum account:\n%s\n\nAuthenticate to WizPay MCP. This does not authorize transactions.\n\nURI: %s\nVersion: 1\nChain ID: 5042\nNonce: %s\nIssued At: %s\nExpiration Time: %s", Domain, a, URI, c.Nonce, c.IssuedAt.UTC().Format(time.RFC3339Nano), c.ExpiresAt.UTC().Format(time.RFC3339Nano)), nil
}
func Verify(message, address, signature string) error {
	if len(signature) != 132 || !strings.HasPrefix(signature, "0x") || len(message) == 0 || len(message) > 2048 {
		return ErrDenied
	}
	sig, e := hex.DecodeString(signature[2:])
	if e != nil || len(sig) != 65 {
		return ErrDenied
	}
	switch sig[64] {
	case 0, 1:
	case 27, 28:
		sig[64] -= 27
	default:
		return ErrDenied
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	if !crypto.ValidateSignatureValues(sig[64], r, s, true) {
		return ErrDenied
	}
	hash := crypto.Keccak256([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len([]byte(message)))), []byte(message))
	pub, e := crypto.SigToPub(hash, sig)
	if e != nil {
		return ErrDenied
	}
	expected, e := Address(address)
	if e != nil || crypto.PubkeyToAddress(*pub).Hex() != expected {
		return ErrDenied
	}
	return nil
}

// ValidTime uses a trusted clock sampled after database locks, not a request timestamp.
func ValidTime(c Challenge, now time.Time) bool {
	return !c.IssuedAt.IsZero() && !now.Before(c.IssuedAt) && now.Before(c.ExpiresAt) && c.ExpiresAt.After(c.IssuedAt) && c.ExpiresAt.Sub(c.IssuedAt) <= Lifetime
}
