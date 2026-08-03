package postgres

import (
	"context"
	stderrors "errors"

	"github.com/jackc/pgx/v5"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/storage"
	"github.com/deseti/wizpay-mcp/internal/storage/postgres/dbsqlc"
	"github.com/deseti/wizpay-mcp/internal/wallet"
)

func walletFromRow(row dbsqlc.WalletBinding) (wallet.Binding, error) {
	version, err := domainVersion(row.Version)
	if err != nil {
		return wallet.Binding{}, err
	}
	return wallet.NewBinding(wallet.BindingParams{BindingID: row.BindingID, Version: version, UserID: row.UserID, Provider: row.Provider, ProviderUserReference: row.ProviderUserReference, WalletID: row.WalletID, Address: row.Address, ChainID: row.ChainID, Network: row.Network, Status: wallet.BindingStatus(row.Status), VerificationReference: row.VerificationReference, CreatedAt: domainTime(row.CreatedAt), VerifiedAt: domainTime(row.VerifiedAt), RevokedAt: domainTime(row.RevokedAt)})
}

func walletCreateParams(scope storage.Scope, value wallet.Binding) (dbsqlc.CreateWalletBindingParams, error) {
	version, err := dbVersion(value.Version())
	if err != nil {
		return dbsqlc.CreateWalletBindingParams{}, err
	}
	return dbsqlc.CreateWalletBindingParams{TenantID: scope.TenantID(), BindingID: value.BindingID(), Version: version, UserID: value.OwnerUserID(), Provider: value.Provider(), ProviderUserReference: value.ProviderUserReference(), WalletID: value.WalletID(), Address: value.Address(), ChainID: value.ChainID(), Network: value.Network(), Status: string(value.Status()), VerificationReference: value.VerificationReference(), CreatedAt: dbTime(value.CreatedAt()), VerifiedAt: dbTime(value.VerifiedAt()), RevokedAt: dbTime(value.RevokedAt())}, nil
}

func (s *Store) CreateBinding(ctx context.Context, scope storage.Scope, value wallet.Binding) (storage.CreateBindingResult, error) {
	if err := scope.Validate(); err != nil {
		return storage.CreateBindingResult{}, err
	}
	if err := value.Validate(); err != nil {
		return storage.CreateBindingResult{}, err
	}
	if value.OwnerUserID() != scope.ActorID() {
		return storage.CreateBindingResult{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Wallet binding owner does not match the trusted request scope.", false, true, true)
	}
	params, err := walletCreateParams(scope, value)
	if err != nil {
		return storage.CreateBindingResult{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return storage.CreateBindingResult{}, err
	}
	defer cancel()
	row, err := s.queries.CreateWalletBinding(bounded, params)
	if err == nil {
		restored, err := walletFromRow(row)
		return storage.CreateBindingResult{Binding: restored, Created: true}, err
	}
	mapped := mapDatabaseError(err)
	var appErr *apperrors.Error
	if !stderrors.As(mapped, &appErr) || appErr.Code != apperrors.CodeExecutionConflict {
		return storage.CreateBindingResult{}, mapped
	}
	existing, findErr := s.FindBindingByWallet(ctx, scope, value.ChainID(), value.Network(), value.Address())
	if findErr != nil {
		return storage.CreateBindingResult{}, mapped
	}
	if !equalBinding(existing, value) {
		return storage.CreateBindingResult{}, mapped
	}
	return storage.CreateBindingResult{Binding: existing, Created: false}, nil
}
func equalBinding(a, b wallet.Binding) bool {
	return a.BindingID() == b.BindingID() && a.Version() == b.Version() && a.OwnerUserID() == b.OwnerUserID() && a.Provider() == b.Provider() && a.ProviderUserReference() == b.ProviderUserReference() && a.WalletID() == b.WalletID() && a.Address() == b.Address() && a.ChainID() == b.ChainID() && a.Network() == b.Network() && a.Status() == b.Status() && a.VerificationReference() == b.VerificationReference() && a.CreatedAt().Equal(b.CreatedAt()) && a.VerifiedAt().Equal(b.VerifiedAt()) && a.RevokedAt().Equal(b.RevokedAt())
}
func (s *Store) FindBindingByID(ctx context.Context, scope storage.Scope, id string) (wallet.Binding, error) {
	if err := scope.Validate(); err != nil {
		return wallet.Binding{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return wallet.Binding{}, err
	}
	defer cancel()
	row, err := s.queries.FindWalletBindingByID(bounded, dbsqlc.FindWalletBindingByIDParams{TenantID: scope.TenantID(), BindingID: id, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return wallet.Binding{}, notFound(apperrors.CodeWalletNotBound, "Wallet binding is not accessible.")
	}
	if err != nil {
		return wallet.Binding{}, mapDatabaseError(err)
	}
	return walletFromRow(row)
}
func (s *Store) FindBindingByWallet(ctx context.Context, scope storage.Scope, chainID, network, address string) (wallet.Binding, error) {
	if err := scope.Validate(); err != nil {
		return wallet.Binding{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return wallet.Binding{}, err
	}
	defer cancel()
	row, err := s.queries.FindWalletBindingByWallet(bounded, dbsqlc.FindWalletBindingByWalletParams{TenantID: scope.TenantID(), ChainID: chainID, Network: network, Address: address, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return wallet.Binding{}, notFound(apperrors.CodeWalletNotBound, "Wallet binding is not accessible.")
	}
	if err != nil {
		return wallet.Binding{}, mapDatabaseError(err)
	}
	return walletFromRow(row)
}
func (s *Store) UpdateBinding(ctx context.Context, scope storage.Scope, value wallet.Binding, expectedVersion uint64) (wallet.Binding, error) {
	if err := scope.Validate(); err != nil {
		return wallet.Binding{}, err
	}
	if err := value.Validate(); err != nil {
		return wallet.Binding{}, err
	}
	if value.OwnerUserID() != scope.ActorID() {
		return wallet.Binding{}, apperrors.New(apperrors.CodeAuthorizationRequired, "Wallet binding owner does not match the trusted request scope.", false, true, true)
	}
	if expectedVersion == ^uint64(0) || value.Version() != expectedVersion+1 {
		return wallet.Binding{}, apperrors.New(apperrors.CodeExecutionConflict, "Wallet binding version must advance exactly once.", false, true, true)
	}
	version, err := dbVersion(value.Version())
	if err != nil {
		return wallet.Binding{}, err
	}
	expected, err := dbVersion(expectedVersion)
	if err != nil {
		return wallet.Binding{}, err
	}
	bounded, cancel, err := s.queryContext(ctx)
	if err != nil {
		return wallet.Binding{}, err
	}
	defer cancel()
	row, err := s.queries.UpdateWalletBinding(bounded, dbsqlc.UpdateWalletBindingParams{TenantID: scope.TenantID(), BindingID: value.BindingID(), Version: version, Status: string(value.Status()), VerificationReference: value.VerificationReference(), VerifiedAt: dbTime(value.VerifiedAt()), RevokedAt: dbTime(value.RevokedAt()), Version_2: expected, ActorID: scope.ActorID()})
	if stderrors.Is(err, pgx.ErrNoRows) {
		return wallet.Binding{}, apperrors.New(apperrors.CodeExecutionConflict, "Wallet binding changed concurrently.", false, true, true)
	}
	if err != nil {
		return wallet.Binding{}, mapDatabaseError(err)
	}
	return walletFromRow(row)
}

var _ storage.WalletBindingRepository = (*Store)(nil)
