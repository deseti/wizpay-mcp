package services

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
	"github.com/deseti/wizpay-mcp/internal/contracts"
	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/execution"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/requestauth"
	"github.com/deseti/wizpay-mcp/internal/storage"
)

type PayrollRecipientDraft struct {
	Address string
	Amount  intents.Amount
}
type PayrollDraft struct {
	ClientRequestID, Nonce, WalletBindingID, TokenSymbol string
	OutputTokenSymbol                                    string
	Recipients                                           []PayrollRecipientDraft
	ReferenceID                                          string
	Deadline                                             time.Time
	GrossInput, MinTotalOut                              intents.Amount
	MinHopPriceX36                                       string
	SwapDeadline                                         time.Time
	PolicyReference                                      string
}
type PayrollPreview struct {
	Token                         intents.Token
	OutputToken                   intents.Token
	Recipients                    []PayrollRecipientDraft
	Total                         intents.Amount
	GrossInput, MinTotalOut       intents.Amount
	MinHopPriceX36                string
	SwapDeadline                  time.Time
	ReferenceID, ChainID, Network string
}

type PayrollService interface {
	PreviewPayroll(context.Context, PayrollDraft) (PayrollPreview, error)
	CreatePayrollIntent(context.Context, PayrollDraft) (intents.Intent, error)
	ExecutePayroll(context.Context, string, string, string, uint64) (execution.Request, error)
	PayrollStatus(context.Context, string) (execution.Execution, error)
}

type PayrollExecutionAuthority interface {
	AuthorizePayroll(context.Context, intents.Intent) error
}

type PersistedPayrollService struct {
	Intents     *PersistedIntentService
	Executions  *PersistedExecutionService
	ExecutionDB storage.ExecutionRepository
	Wallets     storage.WalletBindingRepository
	Authorizer  auth.Authorizer
	Authority   PayrollExecutionAuthority
}

func payrollFinancial(draft PayrollDraft) (intents.PayrollParameters, error) {
	resource, err := contracts.CanonicalToken(draft.TokenSymbol)
	if err != nil {
		return intents.PayrollParameters{}, err
	}
	token := tokenFromResource(resource)
	if draft.OutputTokenSymbol != "" {
		outputResource, err := contracts.CanonicalToken(draft.OutputTokenSymbol)
		if err != nil {
			return intents.PayrollParameters{}, err
		}
		output := tokenFromResource(outputResource)
		lines := make([]intents.Recipient, len(draft.Recipients))
		total := new(big.Int)
		for i, recipient := range draft.Recipients {
			value, err := recipient.Amount.BaseInt()
			if err != nil || recipient.Amount.Decimals != output.Decimals {
				return intents.PayrollParameters{}, fmt.Errorf("recipient %d output amount must use canonical token decimals", i)
			}
			total.Add(total, value)
			lines[i] = intents.Recipient{Address: recipient.Address, TokenOut: output, MinAmountOut: recipient.Amount}
		}
		variant := intents.PayrollVariantBatchSingleTokenOut
		if len(lines) == 1 {
			variant = intents.PayrollVariantSingle
		}
		params := intents.PayrollParameters{SchemaVersion: intents.FinancialSchemaPhase12, Variant: variant, TokenIn: token, Recipients: lines, Total: amountFromBaseUnits(total, output.Decimals), ReferenceID: draft.ReferenceID, CrossToken: &intents.CrossTokenPayrollParameters{GrossInput: draft.GrossInput, MinTotalOut: draft.MinTotalOut, MinHopPriceX36: draft.MinHopPriceX36, Deadline: draft.SwapDeadline}}
		if err := params.ValidateCrossToken(); err != nil {
			return intents.PayrollParameters{}, err
		}
		return params, nil
	}
	lines := make([]intents.Recipient, len(draft.Recipients))
	total := new(big.Int)
	for i, recipient := range draft.Recipients {
		value, err := recipient.Amount.BaseInt()
		if err != nil || recipient.Amount.Decimals != resource.Decimals {
			return intents.PayrollParameters{}, fmt.Errorf("recipient %d amount must use canonical token decimals", i)
		}
		total.Add(total, value)
		lines[i] = intents.Recipient{Address: recipient.Address, TokenOut: token, AmountIn: recipient.Amount, MinAmountOut: recipient.Amount}
	}
	variant := intents.PayrollVariantBatchSingleTokenOut
	if len(lines) == 1 {
		variant = intents.PayrollVariantSingle
	}
	params := intents.PayrollParameters{SchemaVersion: intents.FinancialSchemaPhase12, Variant: variant, TokenIn: token, Recipients: lines, Total: amountFromBaseUnits(total, resource.Decimals), ReferenceID: draft.ReferenceID}
	if err := params.ValidateSameToken(); err != nil {
		return intents.PayrollParameters{}, err
	}
	return params, nil
}

func amountFromBaseUnits(value *big.Int, decimals uint8) intents.Amount {
	digits := value.String()
	width := int(decimals)
	if width == 0 {
		return intents.Amount{Decimal: digits, BaseUnits: digits, Decimals: decimals}
	}
	if len(digits) <= width {
		digits = strings.Repeat("0", width-len(digits)+1) + digits
	}
	whole, fraction := digits[:len(digits)-width], strings.TrimRight(digits[len(digits)-width:], "0")
	decimal := whole
	if fraction != "" {
		decimal += "." + fraction
	}
	return intents.Amount{Decimal: decimal, BaseUnits: value.String(), Decimals: decimals}
}

func (s *PersistedPayrollService) PreviewPayroll(ctx context.Context, draft PayrollDraft) (PayrollPreview, error) {
	if s == nil || s.Wallets == nil || s.Authorizer == nil {
		return PayrollPreview{}, fmt.Errorf("payroll service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return PayrollPreview{}, err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: auth.PermissionCreateIntent}); err != nil {
		return PayrollPreview{}, err
	}
	scope, err := requestauth.StorageScopeFromContext(ctx)
	if err != nil {
		return PayrollPreview{}, err
	}
	binding, err := s.Wallets.FindBindingByID(ctx, scope, draft.WalletBindingID)
	if err != nil {
		return PayrollPreview{}, err
	}
	if err := binding.EnsureAuthorizable(scope.ActorID()); err != nil {
		return PayrollPreview{}, err
	}
	params, err := payrollFinancial(draft)
	if err != nil {
		return PayrollPreview{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll is invalid.", false, true, true, err)
	}
	now := time.Now().UTC()
	if s.Intents != nil && s.Intents.Now != nil {
		now = s.Intents.Now().UTC()
	}
	_, err = intents.NewDraft(intents.Params{IntentID: "preview", Version: 1, ClientRequestID: draft.ClientRequestID, Nonce: draft.Nonce, Type: intents.TypePayroll, Ownership: intents.Ownership{UserID: scope.ActorID(), IdentityProvider: request.Principal().IdentityProvider(), ProviderUserReference: binding.ProviderUserReference(), WalletBindingID: binding.BindingID(), WalletBindingVersion: binding.Version(), WalletID: binding.WalletID(), WalletAddress: binding.Address(), ChainID: binding.ChainID(), Network: binding.Network()}, Financial: intents.FinancialParameters{Payroll: &params}, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferencePayroll, Version: intents.RouteVersionPayroll}, Constraints: intents.Constraints{Deadline: draft.Deadline, PolicyReference: draft.PolicyReference}, CreatedAt: now, ExpiresAt: draft.Deadline})
	if err != nil {
		return PayrollPreview{}, err
	}
	resource, _ := contracts.CanonicalToken(draft.TokenSymbol)
	preview := PayrollPreview{Token: params.TokenIn, Recipients: append([]PayrollRecipientDraft(nil), draft.Recipients...), Total: params.Total, ReferenceID: params.ReferenceID, ChainID: resource.ChainID, Network: resource.Network}
	if params.CrossTokenExecutable() {
		preview.OutputToken = params.Recipients[0].TokenOut
		preview.GrossInput = params.CrossToken.GrossInput
		preview.MinTotalOut = params.CrossToken.MinTotalOut
		preview.MinHopPriceX36 = params.CrossToken.MinHopPriceX36
		preview.SwapDeadline = params.CrossToken.Deadline
	}
	return preview, nil
}

func (s *PersistedPayrollService) CreatePayrollIntent(ctx context.Context, draft PayrollDraft) (intents.Intent, error) {
	if s == nil || s.Intents == nil {
		return intents.Intent{}, fmt.Errorf("payroll service is not configured")
	}
	params, err := payrollFinancial(draft)
	if err != nil {
		return intents.Intent{}, apperrors.Wrap(apperrors.CodeValidationError, "Payroll is invalid.", false, true, true, err)
	}
	return s.Intents.CreateIntent(ctx, CreateIntentCommand{ClientRequestID: draft.ClientRequestID, Nonce: draft.Nonce, WalletBindingID: draft.WalletBindingID, Type: intents.TypePayroll, Financial: intents.FinancialParameters{Payroll: &params}, Route: intents.Route{Type: intents.RouteAllowlistedContract, Reference: intents.RouteReferencePayroll, Version: intents.RouteVersionPayroll}, Deadline: draft.Deadline, PolicyReference: draft.PolicyReference})
}

func (s *PersistedPayrollService) ExecutePayroll(ctx context.Context, intentID, approvalID, policyID string, policyVersion uint64) (execution.Request, error) {
	if s == nil || s.Intents == nil || s.Executions == nil || s.Authority == nil {
		return execution.Request{}, apperrors.New(apperrors.CodeCapabilityUnavailable, "Arc Mainnet Payroll authorization provider is unavailable.", false, true, true)
	}
	intent, err := s.Intents.GetIntent(ctx, intentID)
	if err != nil {
		return execution.Request{}, err
	}
	if intent.Type() != intents.TypePayroll || intent.Financial().Payroll == nil || (!intent.Financial().Payroll.SameTokenExecutable() && !intent.Financial().Payroll.CrossTokenExecutable()) {
		return execution.Request{}, apperrors.New(apperrors.CodeValidationError, "Executable PAYROLL intent is required.", false, true, true)
	}
	if err := s.Authority.AuthorizePayroll(ctx, intent); err != nil {
		return execution.Request{}, err
	}
	return s.Executions.PrepareExecution(ctx, intentID, approvalID, policyID, policyVersion)
}

func (s *PersistedPayrollService) PayrollStatus(ctx context.Context, executionID string) (execution.Execution, error) {
	if s == nil || s.ExecutionDB == nil || s.Intents == nil || s.Intents.Intents == nil || s.Authorizer == nil {
		return execution.Execution{}, fmt.Errorf("payroll service is not configured")
	}
	request, err := auth.TrustedRequestFromContext(ctx)
	if err != nil {
		return execution.Execution{}, err
	}
	if err := s.Authorizer.Authorize(ctx, auth.AuthorizationInput{Request: request, Permission: auth.PermissionPrepareExecution}); err != nil {
		return execution.Execution{}, err
	}
	scope, err := requestauth.StorageScopeFromContext(ctx)
	if err != nil {
		return execution.Execution{}, err
	}
	value, err := s.ExecutionDB.FindExecutionByID(ctx, scope, executionID)
	if err != nil {
		return execution.Execution{}, err
	}
	intent, err := s.Intents.Intents.FindIntentByID(ctx, scope, value.Request().IntentID())
	if err != nil {
		return execution.Execution{}, err
	}
	if intent.Type() != intents.TypePayroll || intent.Digest() != value.Request().IntentDigest() {
		return execution.Execution{}, apperrors.New(apperrors.CodeValidationError, "Execution is not a PAYROLL execution.", false, true, true)
	}
	return value, nil
}

var _ PayrollService = (*PersistedPayrollService)(nil)
