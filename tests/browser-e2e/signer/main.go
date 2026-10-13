// Local browser-test fixture only. Never run against a production database.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/deseti/wizpay-mcp/internal/siwe"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
)

// Only messages exactly matching the server's restricted profile can be signed.
func eligible(message, address string, now time.Time) bool {
	lines := strings.Split(message, "\n")
	if len(lines) != 11 || !strings.HasPrefix(lines[8], "Nonce: ") || !strings.HasPrefix(lines[9], "Issued At: ") || !strings.HasPrefix(lines[10], "Expiration Time: ") {
		return false
	}
	issued, e1 := time.Parse(time.RFC3339Nano, strings.TrimPrefix(lines[9], "Issued At: "))
	expires, e2 := time.Parse(time.RFC3339Nano, strings.TrimPrefix(lines[10], "Expiration Time: "))
	c := siwe.Challenge{Address: address, Nonce: strings.TrimPrefix(lines[8], "Nonce: "), IssuedAt: issued, ExpiresAt: expires}
	if e1 != nil || e2 != nil || !siwe.ValidTime(c, now) {
		return false
	}
	for _, purpose := range []string{"AUTHENTICATE", "ENROLL"} {
		c.Purpose = purpose
		exact, err := siwe.Message(c)
		if err == nil && exact == message {
			return true
		}
	}
	return false
}

func main() {
	key, err := crypto.GenerateKey()
	if err != nil {
		os.Exit(1)
	}
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	otherKey, err := crypto.GenerateKey()
	if err != nil {
		os.Exit(1)
	}
	otherAddress := crypto.PubkeyToAddress(otherKey.PublicKey).Hex()
	id, err := siwe.Random()
	if err != nil {
		os.Exit(1)
	}
	fixture, err := newFixture(id, address, otherAddress)
	if err != nil {
		os.Exit(1)
	}
	out := json.NewEncoder(os.Stdout)
	if out.Encode(fixture) != nil {
		os.Exit(1)
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 8192)
	for scanner.Scan() {
		var r struct {
			ID      int    `json:"id"`
			Message string `json:"message"`
		}
		// Decode before assessing either address; malformed input cannot sign.
		if json.Unmarshal(scanner.Bytes(), &r) != nil {
			_ = out.Encode(map[string]any{"id": r.ID, "error": "signature rejected"})
			continue
		}
		valid := eligible(r.Message, address, time.Now().UTC())
		signingKey := key
		if !valid && eligible(r.Message, otherAddress, time.Now().UTC()) {
			valid = true
			signingKey = otherKey
		}
		if !valid {
			_ = out.Encode(map[string]any{"id": r.ID, "error": "signature rejected"})
			continue
		}
		sig, err := crypto.Sign(accounts.TextHash([]byte(r.Message)), signingKey)
		if err != nil {
			_ = out.Encode(map[string]any{"id": r.ID, "error": "signature rejected"})
			continue
		}
		_ = out.Encode(map[string]any{"id": r.ID, "signature": "0x" + hex.EncodeToString(sig)})
	}
}
