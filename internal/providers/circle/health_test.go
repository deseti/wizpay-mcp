package circle

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/deseti/wizpay-mcp/internal/providers"
)

func TestCircleHealthNotConfigured(t *testing.T) {
	checker, err := NewHealthChecker(Config{}, nil, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	health := checker.Check(context.Background())
	if health.Status != providers.HealthNotConfigured {
		t.Fatalf("status = %s", health.Status)
	}
}

func TestCircleHealthProbeStatuses(t *testing.T) {
	cases := map[int]providers.HealthStatus{
		http.StatusOK:                  providers.HealthHealthy,
		http.StatusNotFound:            providers.HealthHealthy,
		http.StatusUnauthorized:        providers.HealthDegraded,
		http.StatusInternalServerError: providers.HealthUnavailable,
	}
	for code, want := range cases {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "" {
					t.Fatal("API key header missing")
				}
				if r.Header.Get("X-User-Token") != "" {
					t.Fatal("user token must not be sent on health probe")
				}
				w.WriteHeader(code)
			}))
			defer server.Close()

			httpClient := &http.Client{Transport: &rewriteTransport{target: server.URL}, Timeout: 2 * time.Second}
			config := Config{
				Enabled: true, BaseURL: "https://api.circle.com", APIKey: APIKey{value: "test-key"},
				Blockchain: BlockchainArcTestnet, ChainID: "5042002", Network: "TESTNET",
				Timeout: 2 * time.Second,
			}
			checker, err := NewHealthChecker(config, httpClient, nil, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			health := checker.Check(context.Background())
			if health.Status != want {
				t.Fatalf("status = %s want %s detail=%q", health.Status, want, health.Detail)
			}
			if health.Detail == "" {
				t.Fatal("detail required")
			}
		})
	}
}

type rewriteTransport struct {
	target string
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(r.target)
	if err != nil {
		return nil, err
	}
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
	}
	endpoint := target.Scheme + "://" + target.Host + req.URL.RequestURI()
	out, err := http.NewRequestWithContext(req.Context(), req.Method, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	out.Header = req.Header.Clone()
	return http.DefaultTransport.RoundTrip(out)
}
