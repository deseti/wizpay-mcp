// Local browser-test fixture only. Never run against a production database.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	exact, err := siwe.Message(c)
	return e1 == nil && e2 == nil && err == nil && exact == message && siwe.ValidTime(c, now)
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
	tenant, user, client := "wp3d-browser-"+id, "browser-user-"+id, "browser-client-"+id
	// Public fixture metadata only. No key is serialized or persisted.
	sql := fmt.Sprintf(`BEGIN;
DO $$ BEGIN IF current_database() <> 'wizpay_mcp_browser_e2e' THEN RAISE EXCEPTION 'wrong test database'; END IF; END $$;
INSERT INTO tenants(tenant_id,created_at,status) VALUES('%s',now(),'ACTIVE');
INSERT INTO identities(tenant_id,user_id,provider,provider_subject,status,created_at,updated_at) VALUES('%s','%s','browser-e2e','%s','ACTIVE',now(),now());
INSERT INTO wallet_bindings(tenant_id,binding_id,version,user_id,provider,provider_user_reference,wallet_id,address,chain_id,network,status,verification_reference,created_at,verified_at) VALUES('%s','binding-%s',1,'%s','EXTERNAL_EVM','synthetic-owner','wallet-%s','%s','5042','MAINNET','ACTIVE','test-only-owner-review',now(),now());
INSERT INTO oauth_clients(client_id,name,client_type,auth_method,status,redirect_uris,scopes,resources) VALUES('%s','Synthetic Browser Client','public','none','ACTIVE',ARRAY['https://connect.wizpay.xyz/browser-e2e/callback'],ARRAY['mcp:read'],ARRAY['https://mcp.wizpay.xyz/mcp']);
COMMIT;`, tenant, tenant, user, user, tenant, id, user, id, address, client)
	otherSQL := fmt.Sprintf("INSERT INTO identities(tenant_id,user_id,provider,provider_subject,status,created_at,updated_at) VALUES('%s','other-%s','browser-e2e','other-%s','ACTIVE',now(),now());\nINSERT INTO wallet_bindings(tenant_id,binding_id,version,user_id,provider,provider_user_reference,wallet_id,address,chain_id,network,status,verification_reference,created_at,verified_at) VALUES('%s','other-binding-%s',1,'other-%s','EXTERNAL_EVM','synthetic-owner','other-wallet-%s','%s','5042','MAINNET','ACTIVE','test-only-owner-review',now(),now());\n", tenant, user, user, tenant, id, user, id, otherAddress)
	sql = strings.Replace(sql, "COMMIT;", otherSQL+"COMMIT;", 1)
	out := json.NewEncoder(os.Stdout)
	if out.Encode(map[string]string{"address": address, "other_address": otherAddress, "other_user_id": "other-" + user, "tenant_id": tenant, "user_id": user, "client_id": client, "sql": sql}) != nil {
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
