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

func TestCircleMainnetHealthRemainsOfflineInTrackA(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	config := Config{
		Enabled: true, BaseURL: "https://api.circle.com", APIKey: APIKey{value: "test-key"},
		Blockchain: Blockchain("ARC"), ChainID: "5042", Network: "MAINNET", Timeout: 2 * time.Second,
	}
	checker, err := NewHealthChecker(config, &http.Client{Transport: &rewriteTransport{target: server.URL}}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	health := checker.Check(context.Background())
	if health.Status != providers.HealthNotConfigured || called {
		t.Fatalf("health=%#v network_called=%v", health, called)
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
