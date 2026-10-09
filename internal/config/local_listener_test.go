package config

import "testing"

func TestLocalListenerIsolation(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1", ""} {
		cfg, err := LoadWithLookup(func(k string) (string, bool) {
			if k == "SERVER_HOST" {
				return host, true
			}
			return "", false
		})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"127.0.0.1": "127.0.0.1:8080", "::1": "[::1]:8080", "": ":8080"}[host]
		if cfg.Address() != want {
			t.Fatalf("got %s, want %s", cfg.Address(), want)
		}
	}
	for _, host := range []string{"0.0.0.0", "192.0.2.1", "localhost", "127.0.0.1:8080"} {
		_, err := LoadWithLookup(func(k string) (string, bool) {
			if k == "SERVER_HOST" {
				return host, true
			}
			return "", false
		})
		if err == nil {
			t.Fatalf("accepted non-loopback host %s", host)
		}
	}
}
