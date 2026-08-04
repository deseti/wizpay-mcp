package errors

import (
	stderrors "errors"
	"testing"
)

func TestToPublicKnownError(t *testing.T) {
	internalCause := stderrors.New("sensitive internal detail")
	err := Wrap(CodeApprovalRequired, "Explicit approval is required.", false, true, false, internalCause)

	public := ToPublic(err)
	if public.Code != CodeApprovalRequired || public.Message != "Explicit approval is required." {
		t.Fatalf("ToPublic() = %+v", public)
	}
	if !public.UserActionRequired || public.Retryable || public.Terminal {
		t.Fatalf("ToPublic() flags = %+v", public)
	}
	if !stderrors.Is(err, internalCause) {
		t.Fatal("wrapped cause is not available internally")
	}
}

func TestToPublicUnknownErrorHidesDetails(t *testing.T) {
	public := ToPublic(stderrors.New("database password was exposed here"))
	if public.Code != CodeInternalError || public.Message != internalMessage {
		t.Fatalf("ToPublic() = %+v, want safe internal error", public)
	}
}

func TestToPublicUnknownCodeHidesDetails(t *testing.T) {
	public := ToPublic(New(Code("unregistered_code"), "unsafe detail", false, false, false))
	if public.Code != CodeInternalError || public.Message != internalMessage {
		t.Fatalf("ToPublic() = %+v, want safe internal error", public)
	}
}

func TestToPublicDomainCodes(t *testing.T) {
	codes := []Code{
		CodeIdentityNotFound,
		CodeIdentitySuspended,
		CodeIdentityRevoked,
		CodeWalletNotBound,
		CodeWalletMismatch,
		CodeWalletRevoked,
		CodeIntentNotFound,
		CodeIntentExpired,
		CodeIntentMutated,
		CodeApprovalNotFound,
		CodeApprovalExpired,
		CodeApprovalRejected,
		CodeApprovalAlreadyConsumed,
		CodePolicyNotFound,
		CodePolicyInvalid,
		CodePolicyDenied,
		CodePolicyExpired,
		CodePolicyDisabled,
		CodeReviewRequired,
		CodeCapabilityNotFound,
		CodeCapabilityUnavailable,
		CodeCapabilityConflict,
		CodeProviderNotFound,
		CodeProviderUnavailable,
		CodeProviderConflict,
		CodeContractNotFound,
		CodeContractUnavailable,
		CodeContractConflict,
		CodeExecutionNotFound,
		CodeExecutionInvalid,
		CodeExecutionNotAuthorized,
		CodeExecutionConflict,
		CodeExecutionFailed,
		CodeExecutionRecoverable,
	}
	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			public := ToPublic(New(code, "Safe domain error.", false, true, false))
			if public.Code != code || public.Message != "Safe domain error." {
				t.Fatalf("ToPublic() = %+v, want code %q", public, code)
			}
		})
	}
}
