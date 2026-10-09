package browser

import (
	"github.com/deseti/wizpay-mcp/internal/oauth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSIWEJSONStrictness(t *testing.T) {
	for _, body := range []string{`{}`, `{"address":"a","address":"b"}`, `{"address":"a","user_id":"u"}`, `{"address":true}`, `{"address":""}`, `{"address":"a"} {}`, `[]`, `{"address":null}`} {
		if _, e := exactObject(strings.NewReader(body), "address"); e == nil {
			t.Fatal(body)
		}
	}
	if _, e := exactObject(strings.NewReader(`{"address":"a"}`), "address"); e != nil {
		t.Fatal(e)
	}
}
func TestSIWEHTTPProductionUnavailable(t *testing.T) {
	h, raw, csrf := setup(t)
	for _, path := range []string{"/browser/siwe/challenge", "/browser/siwe/verify"} {
		body := `{"address":"0x0000000000000000000000000000000000000001"}`
		if strings.HasSuffix(path, "verify") {
			body = `{"challenge_id":"x","signature":"x"}`
		}
		r := request("POST", path, body)
		r.Header.Set("Origin", oauth.Issuer)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: CookieName, Value: raw})
		w := deadlineRecorder{httptest.NewRecorder()}
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(path, w.Code)
		}
		if len(w.Result().Cookies()) != 0 {
			t.Fatal("rotation without verifier")
		}
	}
}
