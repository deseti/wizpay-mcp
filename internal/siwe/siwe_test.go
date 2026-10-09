package siwe

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func signFixture(t *testing.T, message string) (string, string) {
	t.Helper()
	k, e := crypto.HexToECDSA(strings.Repeat("0", 63) + "1")
	if e != nil {
		t.Fatal(e)
	}
	h := crypto.Keccak256([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len([]byte(message)))), []byte(message))
	sig, e := crypto.Sign(h, k)
	if e != nil {
		t.Fatal(e)
	}
	return crypto.PubkeyToAddress(k.PublicKey).Hex(), "0x" + hex.EncodeToString(sig)
}
func TestEOAVerification(t *testing.T) {
	for _, message := range []string{"example message", "UTF-8: é 🔒", "\x19Ethereum Signed Message:\n42literal"} {
		address, sig := signFixture(t, message)
		if e := Verify(message, address, sig); e != nil {
			t.Fatal(e)
		}
		b, _ := hex.DecodeString(sig[2:])
		b[64] += 27
		if e := Verify(message, address, "0x"+hex.EncodeToString(b)); e != nil {
			t.Fatal(e)
		}
		if Verify(message+"!", address, sig) == nil {
			t.Fatal("altered message accepted")
		}
		if Verify(message, "0x0000000000000000000000000000000000000002", sig) == nil {
			t.Fatal("wrong signer accepted")
		}
	}
}
func TestMalformedSignaturesFailClosed(t *testing.T) {
	a, sig := signFixture(t, "m")
	for n := 0; n < 150; n++ {
		if n == 132 {
			continue
		}
		if Verify("m", a, "0x"+strings.Repeat("0", n)) == nil {
			t.Fatalf("length %d", n)
		}
	}
	for _, v := range []byte{2, 26, 29, 54, 255} {
		b, _ := hex.DecodeString(sig[2:])
		b[64] = v
		if Verify("m", a, "0x"+hex.EncodeToString(b)) == nil {
			t.Fatal("recovery value")
		}
	}
	for _, which := range []string{"zero-r", "zero-s", "high-s", "overflow-r", "hex"} {
		b, _ := hex.DecodeString(sig[2:])
		switch which {
		case "zero-r":
			clear(b[:32])
		case "zero-s":
			clear(b[32:64])
		case "high-s":
			high := new(big.Int).Sub(crypto.S256().Params().N, new(big.Int).SetBytes(b[32:64]))
			high.FillBytes(b[32:64])
			b[64] ^= 1
		case "overflow-r":
			crypto.S256().Params().N.FillBytes(b[:32])
		case "hex":
			if Verify("m", a, "0x"+strings.Repeat("z", 130)) == nil {
				t.Fatal("hex")
			}
			continue
		}
		if Verify("m", a, "0x"+hex.EncodeToString(b)) == nil {
			t.Fatal(which)
		}
	}
	if Verify("m", a, sig+strings.Repeat("00", 32)) == nil {
		t.Fatal("wrapped signature")
	}
}
func TestMessageProfile(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	address, _ := signFixture(t, "m")
	c := Challenge{Address: address, Nonce: strings.Repeat("a", 64), IssuedAt: at, ExpiresAt: at.Add(time.Minute)}
	m, e := Message(c)
	if e != nil {
		t.Fatal(e)
	}
	expected := "connect.wizpay.xyz wants you to sign in with your Ethereum account:\n" + address + "\n\nAuthenticate to WizPay MCP. This does not authorize transactions.\n\nURI: https://connect.wizpay.xyz\nVersion: 1\nChain ID: 5042\nNonce: " + c.Nonce + "\nIssued At: 2026-01-02T03:04:05Z\nExpiration Time: 2026-01-02T03:05:05Z"
	if m != expected {
		t.Fatalf("format mismatch %q", m)
	}
	for _, bad := range []Challenge{{Address: "invalid", Nonce: c.Nonce, IssuedAt: at, ExpiresAt: c.ExpiresAt}, {Address: c.Address, Nonce: "bad", IssuedAt: at, ExpiresAt: c.ExpiresAt}, {Address: c.Address, Nonce: c.Nonce, IssuedAt: at, ExpiresAt: at}, {Address: c.Address, Nonce: c.Nonce, IssuedAt: at, ExpiresAt: at.Add(3 * time.Minute)}} {
		if _, e := Message(bad); e == nil {
			t.Fatal("invalid challenge")
		}
	}
	if _, e := Address("0x52908400098527886E0F7030069857D2e4169EE7"); e == nil {
		t.Fatal("bad checksum")
	}
}

func TestMessageAuthorityAlteration(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	address, _ := signFixture(t, "m")
	c := Challenge{Address: address, Nonce: strings.Repeat("a", 64), IssuedAt: at, ExpiresAt: at.Add(time.Minute)}
	message, e := Message(c)
	if e != nil {
		t.Fatal(e)
	}
	_, sig := signFixture(t, message)
	for _, change := range [][2]string{{Domain, "evil.example"}, {URI, "https://evil.example"}, {"5042", "1"}, {c.Nonce, strings.Repeat("b", 64)}, {"03:04:05", "03:04:06"}} {
		altered := strings.Replace(message, change[0], change[1], 1)
		if Verify(altered, address, sig) == nil {
			t.Fatal("changed signed authority")
		}
	}
}
func TestChallengeTimePolicy(t *testing.T) {
	now := time.Now().UTC()
	c := Challenge{IssuedAt: now, ExpiresAt: now.Add(time.Minute)}
	if !ValidTime(c, now) {
		t.Fatal("valid challenge")
	}
	for _, at := range []time.Time{now.Add(-time.Nanosecond), c.ExpiresAt, c.ExpiresAt.Add(time.Second)} {
		if ValidTime(c, at) {
			t.Fatal("invalid challenge time")
		}
	}
	c.ExpiresAt = now.Add(3 * time.Minute)
	if ValidTime(c, now) {
		t.Fatal("unbounded lifetime")
	}
}

func TestTypedDataSignatureIsNotPersonalSign(t *testing.T) {
	key, e := crypto.HexToECDSA(strings.Repeat("0", 63) + "1")
	if e != nil {
		t.Fatal(e)
	}
	hash := crypto.Keccak256([]byte{0x19, 0x01}, make([]byte, 64))
	sig, e := crypto.Sign(hash, key)
	if e != nil {
		t.Fatal(e)
	}
	if Verify("authentication message", crypto.PubkeyToAddress(key.PublicKey).Hex(), "0x"+hex.EncodeToString(sig)) == nil {
		t.Fatal("typed data signature accepted")
	}
}

func TestProfileConfigurationRejectsUntrustedOrigins(t *testing.T) {
	repo := &struct{ Repository }{}
	for _, origin := range []string{"", "http://connect.wizpay.xyz", "https://evil.example", "https://connect.wizpay.xyz/", "https://connect.wizpay.xyz@evil.example"} {
		if _, e := NewService(repo, "reviewed-tenant", origin); e == nil {
			t.Fatal("origin accepted", origin)
		}
	}
	for _, tenant := range []string{"", " tenant", "tenant "} {
		if _, e := NewService(repo, tenant, URI); e == nil {
			t.Fatal("tenant accepted")
		}
	}
	if _, e := NewService(repo, "reviewed-tenant", URI); e != nil {
		t.Fatal(e)
	}
}
