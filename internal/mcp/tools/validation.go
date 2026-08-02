package tools

import (
	"fmt"
	"strings"
	"unicode"

	apperrors "github.com/deseti/wizpay-mcp/internal/errors"
	"github.com/deseti/wizpay-mcp/internal/intents"
	"github.com/deseti/wizpay-mcp/internal/policies"
)

func validateReference(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s must be a non-empty safe reference", name)
	}
	return nil
}

func validateRequestID(value string) error { return validateReference("request_id", value) }

func (in CreateIntentInput) Validate() error {
	for name, value := range map[string]string{"request_id": in.RequestID, "client_request_id": in.ClientRequestID, "nonce": in.Nonce, "wallet_binding_id": in.WalletBindingID, "policy_reference": in.PolicyReference} {
		if err := validateReference(name, value); err != nil {
			return err
		}
	}
	if !in.IntentType.Valid() {
		return fmt.Errorf("intent_type is invalid")
	}
	if in.Deadline.IsZero() {
		return fmt.Errorf("deadline is required")
	}
	if !in.Route.Type.Valid() || in.Route.Version == 0 {
		return fmt.Errorf("route is invalid")
	}
	if err := validateReference("route.reference", in.Route.Reference); err != nil {
		return err
	}

	count := 0
	if in.Financial.Payroll != nil {
		count++
	}
	if in.Financial.Swap != nil {
		count++
	}
	if in.Financial.Bridge != nil {
		count++
	}
	if in.Financial.ANS != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("financial must contain exactly one typed payload")
	}
	if (in.IntentType == intents.TypePayroll) != (in.Financial.Payroll != nil) ||
		(in.IntentType == intents.TypeSwap) != (in.Financial.Swap != nil) ||
		(in.IntentType == intents.TypeBridge) != (in.Financial.Bridge != nil) ||
		(in.IntentType == intents.TypeANSRegistration) != (in.Financial.ANS != nil) {
		return fmt.Errorf("financial payload does not match intent_type")
	}
	return validateFinancial(in.Financial)
}

func validateFinancial(financial intents.FinancialParameters) error {
	if payroll := financial.Payroll; payroll != nil {
		if err := payroll.Token.Validate(); err != nil {
			return err
		}
		if len(payroll.Recipients) == 0 || len(payroll.Recipients) > 500 {
			return fmt.Errorf("payroll requires 1 to 500 recipients")
		}
		for index, recipient := range payroll.Recipients {
			if err := validateReference("recipient address", recipient.Address); err != nil {
				return fmt.Errorf("recipient %d: %w", index, err)
			}
			if err := recipient.Amount.Validate(); err != nil {
				return fmt.Errorf("recipient %d: %w", index, err)
			}
		}
		if err := payroll.Total.Validate(); err != nil {
			return err
		}
	}
	if swap := financial.Swap; swap != nil {
		if err := swap.InputToken.Validate(); err != nil {
			return err
		}
		if err := swap.OutputToken.Validate(); err != nil {
			return err
		}
		for _, item := range []struct {
			name   string
			amount intents.Amount
		}{{"input", swap.InputAmount}, {"expected output", swap.ExpectedOutput}, {"minimum output", swap.MinimumOutput}} {
			if err := item.amount.Validate(); err != nil {
				return fmt.Errorf("%s amount: %w", item.name, err)
			}
		}
		if err := validateReference("quote_reference", swap.QuoteReference); err != nil {
			return err
		}
	}
	if bridge := financial.Bridge; bridge != nil {
		if err := validateChainID(bridge.SourceChainID); err != nil {
			return err
		}
		if err := validateChainID(bridge.DestinationChainID); err != nil {
			return err
		}
		if err := bridge.SourceToken.Validate(); err != nil {
			return err
		}
		if err := bridge.SourceAmount.Validate(); err != nil {
			return err
		}
		if err := bridge.DestinationAmount.Validate(); err != nil {
			return err
		}
		if err := validateReference("destination_address", bridge.DestinationAddress); err != nil {
			return err
		}
		if err := validateReference("plan_reference", bridge.PlanReference); err != nil {
			return err
		}
	}
	if ans := financial.ANS; ans != nil {
		if err := validateReference("normalized_name", ans.NormalizedName); err != nil {
			return err
		}
		if err := validateReference("name_version", ans.NameVersion); err != nil {
			return err
		}
		if err := validateReference("controller", ans.Controller); err != nil {
			return err
		}
		if err := ans.CostToken.Validate(); err != nil {
			return err
		}
		if err := ans.Cost.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateChainID(value string) error {
	if value == "" || len(value) > 20 || value[0] == '0' {
		return fmt.Errorf("chain ID must be a canonical positive decimal string")
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return fmt.Errorf("chain ID must be a canonical positive decimal string")
		}
	}
	return nil
}

func (in GetIntentInput) Validate() error {
	if err := validateRequestID(in.RequestID); err != nil {
		return err
	}
	return validateReference("intent_id", in.IntentID)
}
func (in RequestApprovalInput) Validate() error {
	if err := validateRequestID(in.RequestID); err != nil {
		return err
	}
	return validateReference("intent_id", in.IntentID)
}
func (in GetApprovalInput) Validate() error {
	if err := validateRequestID(in.RequestID); err != nil {
		return err
	}
	return validateReference("approval_id", in.ApprovalID)
}
func (in EvaluatePolicyInput) Validate() error {
	for name, value := range map[string]string{"request_id": in.RequestID, "intent_id": in.IntentID, "policy_id": in.PolicyID} {
		if err := validateReference(name, value); err != nil {
			return err
		}
	}
	if in.PolicyVersion == 0 {
		return fmt.Errorf("policy_version must be at least 1")
	}
	stage := policies.EvaluationStage(in.Stage)
	if stage != policies.EvaluationStageBeforeApproval && stage != policies.EvaluationStageBeforeExecution {
		return fmt.Errorf("stage is invalid")
	}
	return nil
}
func (in PrepareExecutionInput) Validate() error {
	for name, value := range map[string]string{"request_id": in.RequestID, "intent_id": in.IntentID, "approval_id": in.ApprovalID, "policy_id": in.PolicyID} {
		if err := validateReference(name, value); err != nil {
			return err
		}
	}
	if in.PolicyVersion == 0 {
		return fmt.Errorf("policy_version must be at least 1")
	}
	return nil
}

func publicToolError(requestID string, err error) *ToolError {
	public := apperrors.ToPublic(err)
	return &ToolError{Code: string(public.Code), Message: public.Message, RequestID: requestID, Retryable: public.Retryable, UserActionRequired: public.UserActionRequired, Terminal: public.Terminal}
}

func validationToolError(requestID string, err error) *ToolError {
	return publicToolError(requestID, apperrors.Wrap(apperrors.CodeValidationError, "Tool input is invalid.", false, true, true, err))
}
