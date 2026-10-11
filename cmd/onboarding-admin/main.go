// Owner-operated, bounded administration only. Never included in runtime images.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/deseti/wizpay-mcp/internal/onboarding"
	"github.com/jackc/pgx/v5"
)

func decode(input io.Reader) (onboarding.Request, error) {
	var r onboarding.Request
	body, err := io.ReadAll(io.LimitReader(input, 8193))
	if err != nil || len(body) > 8192 {
		return r, onboarding.ErrDenied
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, onboarding.ErrDenied
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return r, onboarding.ErrDenied
	}
	return r, r.Validate(time.Now().UTC())
}

func connectionConfig(raw string) (*pgx.ConnConfig, error) {
	cfg, err := pgx.ParseConfig(strings.TrimSpace(raw))
	if err != nil || cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify || len(cfg.Fallbacks) != 0 {
		return nil, onboarding.ErrDenied
	}
	return cfg, nil
}

func run() error {
	if len(os.Args) != 1 || os.Getenv("ONBOARDING_ADMIN_APPROVED") != "yes" {
		return onboarding.ErrDenied
	}
	r, err := decode(os.Stdin)
	if err != nil {
		return err
	}
	path := os.Getenv("ONBOARDING_DATABASE_URL_FILE")
	if path == "" {
		return onboarding.ErrDenied
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return onboarding.ErrDenied
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return onboarding.ErrDenied
	}
	cfg, err := connectionConfig(string(raw))
	if err != nil {
		return onboarding.ErrDenied
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return onboarding.ErrDenied
	}
	defer conn.Close(context.Background())
	_, err = conn.Exec(ctx, `SELECT public.admin_onboarding($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, r.Action, r.RequestID, r.TenantID, r.UserID, r.BindingID, r.Issuer, r.Subject, r.Address, r.ReviewReference, r.ExpiresAt)
	if err != nil {
		return onboarding.ErrDenied
	}
	return nil
}

func main() {
	if run() != nil {
		os.Stderr.WriteString("FAIL: onboarding administration denied.\n")
		os.Exit(1)
	}
	os.Stdout.WriteString("PASS: onboarding administration applied; no authentication activated.\n")
}
