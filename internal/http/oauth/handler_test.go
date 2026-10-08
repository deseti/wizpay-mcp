package oauth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	domain "github.com/deseti/wizpay-mcp/internal/oauth"
)

func TestMetadataAndOrigins(t *testing.T) {
	h := NewHandler(nil)
	for _, tc := range []struct{ host, path, key, want string }{{"mcp.wizpay.xyz", "/.well-known/oauth-protected-resource/mcp", "resource", domain.Resource}, {"mcp.wizpay.xyz", "/.well-known/oauth-protected-resource", "resource", domain.Resource}, {"connect.wizpay.xyz", "/.well-known/oauth-authorization-server", "issuer", domain.Issuer}} {
		w := httptest.NewRecorder()
		r := originRequest("GET", "https://"+tc.host+tc.path, nil)
		h.ServeHTTP(deadlineRecorder{w}, r)
		var v map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		if w.Code != 200 || v[tc.key] != tc.want {
			t.Fatal("incorrect metadata")
		}
		for _, unsupported := range []string{"registration_endpoint", "jwks_uri", "revocation_endpoint"} {
			if _, ok := v[unsupported]; ok {
				t.Fatal("unsupported endpoint advertised")
			}
		}
		if tc.key == "issuer" {
			if v["client_id_metadata_document_supported"] != false || v["code_challenge_methods_supported"].([]any)[0] != "S256" || len(v["grant_types_supported"].([]any)) != 1 {
				t.Fatal("unsupported features advertised")
			}
		}
	}
	for _, raw := range []string{"https://attacker.example/.well-known/oauth-authorization-server", "https://mcp.wizpay.xyz/.well-known/oauth-protected-resource/other", "https://mcp.wizpay.xyz/oauth/token", "https://connect.wizpay.xyz/mcp"} {
		w := httptest.NewRecorder()
		Routes(h, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, originRequest("GET", raw, nil))
		if w.Code < 400 {
			t.Fatal("cross-origin route exposed")
		}
	}
}

// All persistence methods panic if touched; invalid requests must reject first.
type untouchedRepository struct{ domain.Repository }

func TestHTTPInputRejectionAndNoTestLogin(t *testing.T) {
	s, _ := domain.NewService(untouchedRepository{}, time.Now)
	h := NewHandler(s)
	for _, body := range []string{"grant_type=password", "grant_type=refresh_token", "grant_type=authorization_code&resource=wrong", "grant_type=authorization_code&grant_type=authorization_code", "grant_type=authorization_code&actor_id=arbitrary-user"} {
		w := httptest.NewRecorder()
		r := originRequest("POST", domain.Issuer+"/oauth/token", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.ServeHTTP(deadlineRecorder{w}, r)
		if w.Code != 400 {
			t.Fatal("unsafe token request accepted")
		}
	}
	for _, path := range []string{"/oauth/login", "/oauth/complete", "/oauth/register"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, originRequest("POST", domain.Issuer+path, strings.NewReader(`{"user_id":"user"}`)))
		if w.Code != 404 {
			t.Fatal("login or registration bypass exposed")
		}
	}
	q := url.Values{"response_type": {"code"}, "resource": {domain.Resource}, "scope": {"approval:decide:human"}}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, originRequest("GET", domain.Issuer+"/oauth/authorize?"+q.Encode(), nil))
	if w.Code != 400 {
		t.Fatal("scope escalation accepted")
	}
	for _, header := range []string{"Authorization", "Cookie", "Origin"} {
		r := originRequest("POST", domain.Issuer+"/oauth/token", strings.NewReader("grant_type=authorization_code"))
		r.Header.Set(header, "untrusted")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.ServeHTTP(deadlineRecorder{w}, r)
		if w.Code != 400 {
			t.Fatal("browser/client credential substitution accepted")
		}
	}
}

func originRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.URL.Scheme = ""
	r.URL.Host = ""
	r.RequestURI = r.URL.RequestURI()
	return r
}

// Only for protocol-input unit tests. Real connection deadlines are tested below.
type deadlineRecorder struct{ *httptest.ResponseRecorder }

func (deadlineRecorder) SetReadDeadline(time.Time) error { return nil }

func TestTokenUnsupportedDeadlineFailsClosed(t *testing.T) {
	s, _ := domain.NewService(untouchedRepository{}, time.Now)
	r := originRequest("POST", domain.Issuer+"/oauth/token", strings.NewReader("grant_type=authorization_code"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	NewHandler(s).ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("unsupported deadline status=%d", w.Code)
	}
}

func TestTokenBodyDeadlineOnConnection(t *testing.T) {
	s, _ := domain.NewService(untouchedRepository{}, time.Now)
	server := httptest.NewServer(NewHandler(s))
	defer server.Close()
	for _, partial := range []bool{false, true} {
		conn, err := net.Dial("tcp", server.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		_, err = fmt.Fprintf(conn, "POST /oauth/token HTTP/1.1\r\nHost: connect.wizpay.xyz\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 100\r\n\r\n")
		if err != nil {
			t.Fatal(err)
		}
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			if !partial {
				return
			}
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					if _, e := conn.Write([]byte("a")); e != nil {
						return
					}
				}
			}
		}()
		_ = conn.SetReadDeadline(start.Add(TokenBodyReadTimeout + 3*time.Second))
		response, err := http.ReadResponse(bufio.NewReader(conn), nil)
		close(stop)
		<-done
		conn.Close()
		if err != nil {
			t.Fatalf("timeout response: %v", err)
		}
		response.Body.Close()
		if response.StatusCode != 400 || time.Since(start) > TokenBodyReadTimeout+2*time.Second || time.Since(start) < TokenBodyReadTimeout-200*time.Millisecond {
			t.Fatalf("status=%d elapsed=%s", response.StatusCode, time.Since(start))
		}
	}
}

func TestTokenMalformedBodiesOnConnection(t *testing.T) {
	s, _ := domain.NewService(untouchedRepository{}, time.Now)
	server := httptest.NewServer(NewHandler(s))
	defer server.Close()
	for _, body := range []string{strings.Repeat("a", 8193), "grant_type=%zz", "grant_type=password"} {
		r, _ := http.NewRequest("POST", server.URL+"/oauth/token", strings.NewReader(body))
		r.Host = "connect.wizpay.xyz"
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatalf("body rejected with %d", response.StatusCode)
		}
	}
}
