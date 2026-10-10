package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SecretLookup reads only explicitly configured database secret files, once at
// startup. Errors never include paths, contents or connection strings.
func SecretLookup(lookup LookupEnv, readFile func(string) ([]byte, error)) (LookupEnv, error) {
	if lookup == nil || readFile == nil {
		return nil, fmt.Errorf("database secret loader is unavailable")
	}
	values := map[string]string{}
	for _, name := range []string{"DATABASE_URL", "DATABASE_MIGRATION_URL"} {
		path, exists := lookup(name + "_FILE")
		if !exists {
			continue
		}
		if _, both := lookup(name); both {
			return nil, fmt.Errorf("database secret sources conflict")
		}
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, fmt.Errorf("database secret file is invalid")
		}
		body, err := readFile(path)
		value := strings.TrimSpace(string(body))
		if err != nil || len(body) > 16384 || value == "" || strings.ContainsAny(value, "\r\n\x00") {
			return nil, fmt.Errorf("database secret file is invalid")
		}
		values[name] = value
	}
	return func(key string) (string, bool) {
		if value, ok := values[key]; ok {
			return value, true
		}
		return lookup(key)
	}, nil
}
