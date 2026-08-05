package intents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
)

const digestDomain = "WIZPAY_MCP_INTENT_V1\n"

type RouteType string

const (
	RouteDirectWallet        RouteType = "DIRECT_WALLET"
	RouteAllowlistedContract RouteType = "ALLOWLISTED_CONTRACT"
	RouteApprovedProvider    RouteType = "APPROVED_PROVIDER"
)

func (r RouteType) Valid() bool {
	switch r {
	case RouteDirectWallet, RouteAllowlistedContract, RouteApprovedProvider:
		return true
	default:
		return false
	}
}

// Ownership freezes the exact application identity and wallet binding used to
// authorize an intent. It contains references only, never credentials or keys.
type Ownership struct {
	UserID                string `json:"user_id"`
	IdentityProvider      string `json:"identity_provider"`
	ProviderUserReference string `json:"provider_user_reference"`
	WalletBindingID       string `json:"wallet_binding_id"`
	WalletBindingVersion  uint64 `json:"wallet_binding_version"`
	WalletID              string `json:"wallet_id"`
	WalletAddress         string `json:"wallet_address"`
	ChainID               string `json:"chain_id"`
	Network               string `json:"network"`
}

func (o Ownership) validate() error {
	for _, field := range []struct{ name, value string }{
		{"user ID", o.UserID}, {"identity provider", o.IdentityProvider},
		{"provider user reference", o.ProviderUserReference}, {"wallet binding ID", o.WalletBindingID},
		{"wallet ID", o.WalletID}, {"wallet address", o.WalletAddress}, {"network", o.Network},
	} {
		if err := validateText(field.name, field.value); err != nil {
			return err
		}
	}
	if o.WalletBindingVersion == 0 {
		return fmt.Errorf("wallet binding version must be at least 1")
	}
	return validateChainID(o.ChainID)
}

type Route struct {
	Type      RouteType `json:"type"`
	Reference string    `json:"reference"`
	Version   uint64    `json:"version"`
}

func (r Route) validate() error {
	if !r.Type.Valid() {
		return fmt.Errorf("invalid route type %q", r.Type)
	}
	if err := validateText("route reference", r.Reference); err != nil {
		return err
	}
	if r.Version == 0 {
		return fmt.Errorf("route version must be at least 1")
	}
	return nil
}

// Constraints are frozen authorization limits, not provider execution data.
type Constraints struct {
	Deadline        time.Time `json:"deadline"`
	PolicyReference string    `json:"policy_reference"`
}

func (c Constraints) validate(createdAt, expiresAt time.Time) error {
	if c.Deadline.IsZero() || c.Deadline.Before(createdAt) || c.Deadline.After(expiresAt) {
		return fmt.Errorf("constraint deadline must be between creation and expiration")
	}
	return validateText("policy reference", c.PolicyReference)
}

// Params contains every material field of an intent. Values are normalized and
// defensively copied when a Draft is constructed.
type Params struct {
	IntentID        string
	Version         uint64
	ClientRequestID string
	Nonce           string
	Type            Type
	Ownership       Ownership
	Financial       FinancialParameters
	Route           Route
	Constraints     Constraints
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// Intent is an immutable value. Draft revisions and lifecycle transitions
// return a new value rather than mutating the receiver.
type Intent struct {
	params            Params
	status            Status
	digest            string
	lifecycleRevision uint64
}

// NewDraft validates a typed intent before its material fields are frozen.
// Representation is normalized; schema versions and economic fields are never
// invented. Phase 12 shapes must carry an explicit schema_version.
func NewDraft(params Params) (Intent, error) {
	params = normalizeParams(params)
	normalizeCreationAddresses(&params)
	if err := validateParams(params); err != nil {
		return Intent{}, apperrors.Wrap(apperrors.CodeValidationError, "Intent is invalid.", false, true, true, err)
	}
	return Intent{params: params, status: StatusDraft, lifecycleRevision: 1}, nil
}

// ReviseDraft replaces material fields only while the intent remains DRAFT.
// An exact replay after creation is an idempotent no-op; a material change after
// creation fails closed and requires a new intent and approval.
func (i Intent) ReviseDraft(params Params) (Intent, error) {
	if err := i.Validate(); err != nil {
		return Intent{}, err
	}
	params = normalizeParams(params)
	normalizeCreationAddresses(&params)
	if err := validateParams(params); err != nil {
		return Intent{}, apperrors.Wrap(apperrors.CodeValidationError, "Intent is invalid.", false, true, true, err)
	}
	if i.params.IntentID != params.IntentID || i.params.Version != params.Version {
		return Intent{}, apperrors.New(apperrors.CodeIntentMutated, "A material intent change requires a new intent and approval.", false, true, true)
	}
	if i.status == StatusDraft {
		return Intent{params: params, status: StatusDraft, lifecycleRevision: i.lifecycleRevision + 1}, nil
	}
	canonical, err := canonicalMaterial(params)
	if err != nil {
		return Intent{}, apperrors.Wrap(apperrors.CodeValidationError, "Intent is invalid.", false, true, true, err)
	}
	if digestBytes(canonical) == i.digest {
		return i, nil
	}
	return Intent{}, apperrors.New(apperrors.CodeIntentMutated, "A material intent change requires a new intent and approval.", false, true, true)
}

func (i Intent) Validate() error {
	if err := validateParams(i.params); err != nil {
		return apperrors.Wrap(apperrors.CodeValidationError, "Intent is invalid.", false, true, true, err)
	}
	if !i.status.Valid() {
		return apperrors.New(apperrors.CodeValidationError, "Intent status is invalid.", false, true, true)
	}
	if i.lifecycleRevision == 0 {
		return apperrors.New(apperrors.CodeValidationError, "Intent lifecycle revision must be at least 1.", false, true, true)
	}
	if i.status == StatusDraft {
		if i.digest != "" {
			return apperrors.New(apperrors.CodeValidationError, "Draft intent cannot have a digest.", false, true, true)
		}
		return nil
	}
	canonical, err := canonicalMaterial(i.params)
	if err != nil {
		return apperrors.Wrap(apperrors.CodeValidationError, "Intent is invalid.", false, true, true, err)
	}
	if i.digest == "" || digestBytes(canonical) != i.digest {
		return apperrors.New(apperrors.CodeIntentMutated, "Intent material does not match its digest.", false, true, true)
	}
	return nil
}

func validateParams(p Params) error {
	for _, field := range []struct{ name, value string }{{"intent ID", p.IntentID}, {"client request ID", p.ClientRequestID}, {"nonce", p.Nonce}} {
		if err := validateText(field.name, field.value); err != nil {
			return err
		}
	}
	if p.Version == 0 {
		return fmt.Errorf("intent version must be at least 1")
	}
	if !p.Type.Valid() {
		return fmt.Errorf("invalid intent type %q", p.Type)
	}
	if err := p.Ownership.validate(); err != nil {
		return err
	}
	if err := p.Financial.validate(p.Type); err != nil {
		return err
	}
	switch p.Type {
	case TypePayroll:
		token := p.Financial.Payroll.SourceToken()
		if token.ChainID != p.Ownership.ChainID {
			return fmt.Errorf("payroll token chain does not match owning wallet chain")
		}
		if p.Financial.Payroll.IsPhase12() {
			if err := validatePayrollRoute(p.Route); err != nil {
				return err
			}
		}
	case TypeSwap:
		if p.Financial.Swap.InputToken.ChainID != p.Ownership.ChainID {
			return fmt.Errorf("swap input token chain does not match owning wallet chain")
		}
		if p.Financial.Swap.IsPhase12() {
			if err := p.Financial.Swap.validateWithTimeline(p.CreatedAt, p.Constraints.Deadline, p.ExpiresAt); err != nil {
				return err
			}
			if err := validateSwapRoute(p.Route); err != nil {
				return err
			}
		}
	case TypeBridge:
		if p.Financial.Bridge.SourceChainID != p.Ownership.ChainID {
			return fmt.Errorf("bridge source chain does not match owning wallet chain")
		}
	case TypeANSRegistration:
		if p.Financial.ANS.CostToken.ChainID != p.Ownership.ChainID {
			return fmt.Errorf("ANS cost token chain does not match owning wallet chain")
		}
	}
	if err := p.Route.validate(); err != nil {
		return err
	}
	if p.CreatedAt.IsZero() {
		return fmt.Errorf("intent creation time is required")
	}
	if p.ExpiresAt.IsZero() || !p.ExpiresAt.After(p.CreatedAt) {
		return fmt.Errorf("intent expiration must follow creation")
	}
	return p.Constraints.validate(p.CreatedAt, p.ExpiresAt)
}

func validatePayrollRoute(route Route) error {
	if route.Type != RouteAllowlistedContract {
		return fmt.Errorf("phase 12 payroll requires ALLOWLISTED_CONTRACT route")
	}
	if route.Reference != RouteReferencePayroll {
		return fmt.Errorf("phase 12 payroll route reference must be %s", RouteReferencePayroll)
	}
	if route.Version != RouteVersionPayroll {
		return fmt.Errorf("phase 12 payroll route version must be %d", RouteVersionPayroll)
	}
	return nil
}

func validateSwapRoute(route Route) error {
	if route.Type != RouteAllowlistedContract {
		return fmt.Errorf("phase 12 swap requires ALLOWLISTED_CONTRACT route")
	}
	if route.Reference != RouteReferenceSwap {
		return fmt.Errorf("phase 12 swap route reference must be %s", RouteReferenceSwap)
	}
	if route.Version != RouteVersionSwap {
		return fmt.Errorf("phase 12 swap route version must be %d", RouteVersionSwap)
	}
	return nil
}

// normalizeParams performs representation-only normalization shared by
// NewDraft, ReviseDraft, and Restore.
//
// It never invents schema_version or other immutable economic metadata.
// It does not lowercase EVM addresses: address case is part of historically
// frozen material, and Restore must not rewrite digests. Creation paths apply
// address lowercasing via normalizeCreationAddresses after this step.
func normalizeParams(p Params) Params {
	p.IntentID = strings.TrimSpace(p.IntentID)
	p.ClientRequestID = strings.TrimSpace(p.ClientRequestID)
	p.Nonce = strings.TrimSpace(p.Nonce)
	p.Ownership.UserID = strings.TrimSpace(p.Ownership.UserID)
	p.Ownership.IdentityProvider = strings.TrimSpace(p.Ownership.IdentityProvider)
	p.Ownership.ProviderUserReference = strings.TrimSpace(p.Ownership.ProviderUserReference)
	p.Ownership.WalletBindingID = strings.TrimSpace(p.Ownership.WalletBindingID)
	p.Ownership.WalletID = strings.TrimSpace(p.Ownership.WalletID)
	p.Ownership.WalletAddress = strings.TrimSpace(p.Ownership.WalletAddress)
	p.Ownership.ChainID = strings.TrimSpace(p.Ownership.ChainID)
	p.Ownership.Network = strings.TrimSpace(p.Ownership.Network)
	p.Route.Reference = strings.TrimSpace(p.Route.Reference)
	p.Constraints.PolicyReference = strings.TrimSpace(p.Constraints.PolicyReference)
	p.CreatedAt = p.CreatedAt.UTC()
	p.ExpiresAt = p.ExpiresAt.UTC()
	p.Constraints.Deadline = p.Constraints.Deadline.UTC()
	p.Financial = cloneFinancial(p.Financial)
	// Time representation only — no economic fields invented. Historical swap
	// payloads have nil Quote and zero Deadline, so these branches are no-ops.
	if p.Financial.Swap != nil {
		if p.Financial.Swap.Quote != nil {
			p.Financial.Swap.Quote.ExpiresAt = p.Financial.Swap.Quote.ExpiresAt.UTC()
		}
		if !p.Financial.Swap.Deadline.IsZero() {
			p.Financial.Swap.Deadline = p.Financial.Swap.Deadline.UTC()
		}
	}
	return p
}

// normalizeCreationAddresses lowercases valid EVM addresses for new drafts and
// draft revisions only. Restore must not call this: historical digests were
// frozen from material that was not address-case-rewritten.
func normalizeCreationAddresses(p *Params) {
	if p == nil {
		return
	}
	normalizeOwnershipWalletAddress(&p.Ownership)
	if p.Financial.Payroll != nil {
		normalizePayrollAddresses(p.Financial.Payroll)
	}
	if p.Financial.Swap != nil {
		normalizeSwapAddresses(p.Financial.Swap)
	}
}

type materialEnvelope struct {
	IntentID        string              `json:"intent_id"`
	Version         uint64              `json:"intent_version"`
	ClientRequestID string              `json:"client_request_id"`
	Nonce           string              `json:"nonce"`
	Type            Type                `json:"intent_type"`
	Ownership       Ownership           `json:"ownership"`
	Financial       FinancialParameters `json:"financial_parameters"`
	Route           Route               `json:"route"`
	Constraints     Constraints         `json:"constraints"`
	CreatedAt       time.Time           `json:"created_at"`
	ExpiresAt       time.Time           `json:"expires_at"`
}

func canonicalMaterial(p Params) ([]byte, error) {
	return canonicalJSON(materialEnvelope{p.IntentID, p.Version, p.ClientRequestID, p.Nonce, p.Type, p.Ownership, p.Financial, p.Route, p.Constraints, p.CreatedAt, p.ExpiresAt})
}

func digestBytes(material []byte) string {
	sum := sha256.Sum256(append([]byte(digestDomain), material...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (i Intent) IntentID() string               { return i.params.IntentID }
func (i Intent) Version() uint64                { return i.params.Version }
func (i Intent) ClientRequestID() string        { return i.params.ClientRequestID }
func (i Intent) Nonce() string                  { return i.params.Nonce }
func (i Intent) Type() Type                     { return i.params.Type }
func (i Intent) Ownership() Ownership           { return i.params.Ownership }
func (i Intent) Financial() FinancialParameters { return cloneFinancial(i.params.Financial) }
func (i Intent) Route() Route                   { return i.params.Route }
func (i Intent) Constraints() Constraints       { return i.params.Constraints }
func (i Intent) CreatedAt() time.Time           { return i.params.CreatedAt }
func (i Intent) ExpiresAt() time.Time           { return i.params.ExpiresAt }
func (i Intent) Status() Status                 { return i.status }
func (i Intent) Digest() string                 { return i.digest }
func (i Intent) LifecycleRevision() uint64      { return i.lifecycleRevision }
