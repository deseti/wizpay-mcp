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
