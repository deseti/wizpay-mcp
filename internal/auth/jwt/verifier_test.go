package jwt_test

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"

	"github.com/deseti/wizpay-mcp/internal/auth"
	authjwt "github.com/deseti/wizpay-mcp/internal/auth/jwt"
)

var jwtNow = time.Unix(1_800_000_000, 0).UTC()

func newVerifier(t *testing.T, publicKey *rsa.PublicKey) *authjwt.Verifier {
	t.Helper()
	verifier, err := authjwt.NewVerifier(authjwt.Config{
		Issuer: "https://issuer.example", Audience: "wizpay-mcp", PublicKey: publicKey,
		AllowedAlgorithms: []string{"RS256"}, ClockSkew: 30 * time.Second,
	}, func() time.Time { return jwtNow })
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func claims() authjwt.Claims {
	return authjwt.Claims{
		TenantID: "tenant_1", ActorID: "user_1", Permissions: []string{"intent:read", "intent:create", "intent:read"},
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer: "https://issuer.example", Subject: "user_1", Audience: jwtlib.ClaimStrings{"wizpay-mcp"},
			ExpiresAt: jwtlib.NewNumericDate(jwtNow.Add(10 * time.Minute)), IssuedAt: jwtlib.NewNumericDate(jwtNow.Add(-time.Minute)),
			NotBefore: jwtlib.NewNumericDate(jwtNow.Add(-time.Minute)), ID: "jti_1",
		},
	}
}

func signed(t *testing.T, method jwtlib.SigningMethod, key any, value authjwt.Claims) string {
	t.Helper()
	token, err := jwtlib.NewWithClaims(method, value).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestVerifierAcceptsValidCredentialAndNormalizesClaims(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	principal, err := newVerifier(t, &key.PublicKey).Verify(t.Context(), signed(t, jwtlib.SigningMethodRS256, key, claims()))
	if err != nil {
		t.Fatal(err)
	}
	if principal.TenantID() != "tenant_1" || principal.ActorID() != "user_1" || principal.IdentityProvider() != "https://issuer.example" || principal.ProviderSubject() != "user_1" {
		t.Fatalf("principal = %#v", principal)
	}
	if got := principal.Permissions(); len(got) != 2 || got[0] != auth.PermissionCreateIntent || got[1] != auth.PermissionReadIntent {
		t.Fatalf("permissions = %v", got)
	}
}

func TestVerifierRejectsMalformedSignatureAlgorithmAndTiming(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	verifier := newVerifier(t, &key.PublicKey)

	tests := []struct{ name, token string }{
		{"malformed", "not-a-jwt"},
		{"invalid signature", signed(t, jwtlib.SigningMethodRS256, other, claims())},
		{"unsupported algorithm", signed(t, jwtlib.SigningMethodHS256, []byte("not-an-rsa-key"), claims())},
	}
	expired := claims()
	expired.ExpiresAt = jwtlib.NewNumericDate(jwtNow.Add(-time.Minute))
	notYet := claims()
	notYet.NotBefore = jwtlib.NewNumericDate(jwtNow.Add(time.Minute))
	tests = append(tests,
		struct{ name, token string }{"expired", signed(t, jwtlib.SigningMethodRS256, key, expired)},
		struct{ name, token string }{"not before", signed(t, jwtlib.SigningMethodRS256, key, notYet)},
	)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := verifier.Verify(t.Context(), test.token)
			if err == nil {
				t.Fatal("credential accepted")
			}
			if strings.Contains(err.Error(), test.token) || strings.Contains(err.Error(), "signature") {
				t.Fatalf("unsafe verifier error = %q", err)
			}
		})
	}
}

func TestVerifierRejectsWrongIssuerAudienceAndMissingOrMalformedClaims(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	verifier := newVerifier(t, &key.PublicKey)
	tests := []struct {
		name string
		edit func(*authjwt.Claims)
	}{
		{"issuer", func(c *authjwt.Claims) { c.Issuer = "https://wrong.example" }},
		{"audience", func(c *authjwt.Claims) { c.Audience = jwtlib.ClaimStrings{"wrong"} }},
		{"subject", func(c *authjwt.Claims) { c.Subject = "" }},
		{"tenant", func(c *authjwt.Claims) { c.TenantID = "" }},
		{"actor", func(c *authjwt.Claims) { c.ActorID = "bad\nactor" }},

		{"permission", func(c *authjwt.Claims) { c.Permissions = []string{"root:*"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := claims()
			test.edit(&value)
			if _, err := verifier.Verify(t.Context(), signed(t, jwtlib.SigningMethodRS256, key, value)); err == nil {
				t.Fatal("credential accepted")
			}
		})
	}
}
