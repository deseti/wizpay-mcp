package browser

import (
	"context"
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"github.com/deseti/wizpay-mcp/internal/siwe"
	"time"
)

// NewWalletService is an explicit assembly seam. Production bootstrap deliberately
// continues using NewService with no verifier: this work does not activate login.
func NewWalletService(repo Repository, catalog Catalog, flow OAuth, wallet *siwe.Service, now func() time.Time) (*Service, error) {
	s, e := NewService(repo, catalog, flow, nil, now)
	if e != nil {
		return nil, e
	}
	if wallet == nil {
		return nil, ErrUnavailable
	}
	s.wallet = wallet
	return s, nil
}
func (s *Service) Challenge(ctx context.Context, raw, proof, address string) (siwe.Challenge, error) {
	v, e := s.proof(ctx, raw, proof)
	if e != nil {
		return siwe.Challenge{}, e
	}
	if s.wallet == nil {
		return siwe.Challenge{}, ErrUnavailable
	}
	if v.State != "PENDING" || v.Decision != "OPEN" {
		return siwe.Challenge{}, ErrDenied
	}
	return s.wallet.Issue(ctx, v.Digest, v.CSRFHash, address)
}
func (s *Service) VerifyWallet(ctx context.Context, raw, proof, id, signature string) (string, error) {
	v, e := s.proof(ctx, raw, proof)
	if e != nil {
		return "", e
	}
	if s.wallet == nil {
		return "", ErrUnavailable
	}
	if v.State != "PENDING" || v.Decision != "OPEN" {
		return "", ErrDenied
	}
	next, e := random("wb_")
	if e != nil {
		return "", e
	}
	ref, e := random("browser_")
	if e != nil {
		return "", e
	}
	e = s.wallet.Finalize(ctx, v.Digest, v.CSRFHash, id, signature, siwe.Rotation{Digest: oauth.Digest(next), CSRFHash: oauth.Digest(csrf(next)), Reference: ref})
	if e != nil {
		return "", e
	}
	return next, nil
}
