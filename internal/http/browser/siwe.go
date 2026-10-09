package browser

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"
)

// exactObject rejects duplicate and unknown keys, non-string fields, trailing
// data and missing fields. The handler has already bounded the transport read.
func exactObject(body io.Reader, fields ...string) (map[string]string, error) {
	d := json.NewDecoder(body)
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return nil, io.ErrUnexpectedEOF
	}
	out := map[string]string{}
	for d.More() {
		t, e = d.Token()
		if e != nil {
			return nil, e
		}
		k, ok := t.(string)
		if !ok {
			return nil, io.ErrUnexpectedEOF
		}
		allowed := false
		for _, f := range fields {
			if k == f {
				allowed = true
			}
		}
		if !allowed {
			return nil, io.ErrUnexpectedEOF
		}
		if _, ok = out[k]; ok {
			return nil, io.ErrUnexpectedEOF
		}
		var v string
		if e = d.Decode(&v); e != nil || v == "" {
			return nil, io.ErrUnexpectedEOF
		}
		out[k] = v
	}
	if _, e = d.Token(); e != nil {
		return nil, e
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	if len(out) != len(fields) {
		return nil, io.ErrUnexpectedEOF
	}
	return out, nil
}
func (h *Handler) walletAction(w http.ResponseWriter, r *http.Request, raw, proof string, ctl *http.ResponseController) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "application/json" {
		w.Header().Set("Connection", "close")
		reply(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	fields := []string{"address"}
	if r.URL.Path == "/browser/siwe/verify" {
		fields = []string{"challenge_id", "signature"}
	}
	input, e := exactObject(r.Body, fields...)
	if e != nil {
		w.Header().Set("Connection", "close")
		reply(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	_ = ctl.SetReadDeadline(time.Time{})
	if r.URL.Path == "/browser/siwe/challenge" {
		c, e := h.service.Challenge(r.Context(), raw, proof, input["address"])
		if e != nil {
			reply(w, 403, map[string]string{"error": "authentication_unavailable"})
			return
		}
		reply(w, 200, map[string]string{"challenge_id": c.ID, "message": c.Message, "address": c.Address, "expires_at": c.ExpiresAt.Format(time.RFC3339Nano)})
		return
	}
	next, e := h.service.VerifyWallet(r.Context(), raw, proof, input["challenge_id"], input["signature"])
	if e != nil {
		reply(w, 403, map[string]string{"error": "authentication_unavailable"})
		return
	}
	cookie(w, next, 1800)
	reply(w, 200, map[string]string{"state": "AUTHENTICATED"})
}
