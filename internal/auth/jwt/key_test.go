package jwt_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strconv"
	"strings"
	"testing"

	authjwt "github.com/deseti/wizpay-mcp/internal/auth/jwt"
)

func publicKeyPEM(t *testing.T, key *rsa.PublicKey, pkcs1 bool) []byte {
	t.Helper()
	var der []byte
	if pkcs1 {
		der = x509.MarshalPKCS1PublicKey(key)
	} else {
		value, err := x509.MarshalPKIXPublicKey(key)
		if err != nil {
			t.Fatal(err)
		}
		der = value
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func TestParseRSAPublicKeyRejectsRSAKeysBelow2048Bits(t *testing.T) {
	for _, bits := range []int{1024, 1536} {
		for _, pkcs1 := range []bool{false, true} {
			t.Run(strconv.Itoa(bits)+"/PKCS1="+strconv.FormatBool(pkcs1), func(t *testing.T) {
				key, err := rsa.GenerateKey(rand.Reader, bits)
				if err != nil {
					t.Fatal(err)
				}
				_, err = authjwt.ParseRSAPublicKey(publicKeyPEM(t, &key.PublicKey, pkcs1))
				if err == nil || err.Error() != "invalid authentication public key" {
					t.Fatalf("error = %v", err)
				}
			})
		}
	}
}

func TestParseRSAPublicKeyAccepts2048BitPKIXAndPKCS1Keys(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkcs1 := range []bool{false, true} {
		parsed, err := authjwt.ParseRSAPublicKey(publicKeyPEM(t, &key.PublicKey, pkcs1))
		if err != nil {
			t.Fatalf("PKCS1=%v error = %v", pkcs1, err)
		}
		if parsed.Size() != key.PublicKey.Size() {
			t.Fatalf("PKCS1=%v key size = %d, want %d", pkcs1, parsed.Size(), key.PublicKey.Size())
		}
	}
}

func TestParseRSAPublicKeyContinuesRejectingMalformedAndNonRSAKeys(t *testing.T) {
	malformed := []byte("not-a-pem-key")
	if _, err := authjwt.ParseRSAPublicKey(malformed); err == nil || !strings.Contains(err.Error(), "invalid authentication public key") {
		t.Fatalf("malformed error = %v", err)
	}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	nonRSA := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	if _, err := authjwt.ParseRSAPublicKey(nonRSA); err == nil || err.Error() != "invalid authentication public key" {
		t.Fatalf("non-RSA error = %v", err)
	}
}
