package requestauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/auth"
)

type verifierStub struct {
	principal  auth.AuthenticatedPrincipal
	err        error
	credential string
}

func (v *verifierStub) Verify(_ context.Context, credential string) (auth.AuthenticatedPrincipal, error) {
	v.credential = credential
	return v.principal, v.err
}

type resolverStub struct {
	identity  auth.Identity
	err       error
	principal auth.AuthenticatedPrincipal
}

func (r *resolverStub) ResolveIdentity(_ context.Context, principal auth.AuthenticatedPrincipal) (auth.Identity, error) {
	r.principal = principal
	return r.identity, r.err
}

func middlewareFixture(t *testing.T) (*verifierStub, *resolverStub, Middleware) {
	t.Helper()
	principal, err := auth.NewAuthenticatedPrincipal(auth.PrincipalParams{TenantID: "tenant_1", ActorID: "user_1", IdentityProvider: "issuer.example", ProviderSubject: "legacy-unmapped:user_1", ExpiresAt: time.Now().Add(time.Minute), Permissions: []auth.Permission{auth.PermissionReadIntent}})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := auth.NewIdentity("user_1", "issuer.example", auth.IdentityStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	verifier, resolver := &verifierStub{principal: principal}, &resolverStub{identity: identity}
	middleware, err := NewMiddleware(verifier, resolver)
	if err != nil {
		t.Fatal(err)
	}
	return verifier, resolver, middleware
}

func TestMiddlewareRejectsMissingAndMalformedBearerCredential(t *testing.T) {
	_, _, middleware := middlewareFixture(t)
	for _, header := range []string{"", "Basic abc", "Bearer", "Bearer one two"} {
		request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		request.Header.Set("Authorization", header)
		response := httptest.NewRecorder()
		middleware.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("protected handler reached") })).ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || !strings.HasPrefix(response.Header().Get("WWW-Authenticate"), "Bearer") {
			t.Fatalf("header %q response = %d", header, response.Code)
		}
	}
}

func TestMiddlewareValidCredentialReachesHandlerWithoutRawTokenInContext(t *testing.T) {
	verifier, _, middleware := middlewareFixture(t)
	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer raw-secret-token")
	request.Header.Set("Traceparent", "trace_1")
	response := httptest.NewRecorder()
	reached := false
	middleware.Handler(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		reached = true
		if request.Header.Get("Authorization") != "" {
			t.Fatal("authorization header reached protected handler")
		}
		trusted, err := auth.TrustedRequestFromContext(request.Context())
		if err != nil {
			t.Fatal(err)
		}
		if trusted.Metadata().RequestID == "" || trusted.Metadata().TraceID == "" || trusted.Metadata().TraceID == "trace_1" {
			t.Fatalf("metadata = %+v", trusted.Metadata())
		}
		if strings.Contains(trusted.Principal().TokenID(), "raw-secret-token") {
			t.Fatal("raw token retained")
		}
	})).ServeHTTP(response, request)
	if !reached || verifier.credential != "raw-secret-token" {
		t.Fatalf("reached=%v credential=%q", reached, verifier.credential)
	}
}

func TestMiddlewareInvalidCredentialAndInactiveIdentityAreSafeUnauthorized(t *testing.T) {
	verifier, resolver, middleware := middlewareFixture(t)
	for _, setup := range []func(){
		func() { verifier.err = context.Canceled },
		func() {
			verifier.err = nil
			resolver.identity, _ = auth.NewIdentity("user_1", "issuer.example", auth.IdentityStatusSuspended)
		},
	} {
		setup()
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		request.Header.Set("Authorization", "Bearer raw-secret-token")
		middleware.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler reached") })).ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "secret") {
			t.Fatalf("response = %d %q", response.Code, response.Body.String())
		}
	}
}
