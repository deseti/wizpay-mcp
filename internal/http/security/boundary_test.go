package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBoundedAdmissionAndRecovery(t *testing.T) {
	now := time.Now()
	b := New(func() time.Time { return now })
	calls := 0
	h := b.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		return w
	}
	for i := 0; i < 20; i++ {
		if request("/browser/siwe/verify").Code != 204 {
			t.Fatal("early rejection")
		}
	}
	w := request("/browser/siwe/challenge")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || calls != 20 {
		t.Fatal("unbounded admission")
	}
	if request("/health").Code != 204 || request("/mcp").Code != 204 {
		t.Fatal("unrelated route limited")
	}
	now = now.Add(3 * time.Second)
	if request("/browser/siwe/verify").Code != 204 {
		t.Fatal("no recovery")
	}
	if len(b.buckets) != 2 {
		t.Fatal("unbounded keys")
	}
}
