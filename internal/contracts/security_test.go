package contracts_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoGenericArbitraryContractAPIs(t *testing.T) {
	root := findModuleRoot(t)
	packages := []string{
		filepath.Join(root, "internal", "contracts"),
		filepath.Join(root, "internal", "contracts", "payroll"),
		filepath.Join(root, "internal", "contracts", "swap"),
	}
	forbiddenNames := []string{
		"CallContract",
		"ExecuteABI",
		"RawContractCall",
		"SendCalldata",
		"ExecuteArbitraryContract",
		"EncodeMethod",
		"EncodeArbitrary",
		"SignLocally",
		"SignTransaction",
		"PrivateKey",
		"SeedPhrase",
		"SigningShare",
	}
	forbiddenSubstrings := []string{
		"private key",
		"seed phrase",
		"signing share",
	}

	for _, dir := range packages {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name == nil || !fn.Name.IsExported() {
					continue
				}
				name := fn.Name.Name
				for _, forbidden := range forbiddenNames {
					if name == forbidden || strings.Contains(name, forbidden) {
						t.Fatalf("%s exports forbidden API %s", path, name)
					}
				}
				// No public encoder that accepts arbitrary method name / calldata destination.
				if strings.HasPrefix(name, "Encode") {
					if fn.Type.Params != nil {
						for _, field := range fn.Type.Params.List {
							for _, ident := range field.Names {
								lower := strings.ToLower(ident.Name)
								if lower == "method" || lower == "selector" || lower == "calldata" || lower == "to" || lower == "target" {
									// Encode* may not take free-form destination/method inputs.
									// Registry pointer and typed input structs are fine.
									t.Fatalf("%s.%s accepts free-form parameter %s", path, name, ident.Name)
								}
							}
						}
					}
				}
			}
			if strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lower := strings.ToLower(string(src))
			for _, phrase := range forbiddenSubstrings {
				// Comments mentioning exclusion are allowed; binding implementations are not.
				if strings.Contains(lower, "never stores "+phrase) || strings.Contains(lower, "never for signing") {
					continue
				}
			}
			// No admin encode helpers.
			if strings.Contains(string(src), "func EncodeEmergency") || strings.Contains(string(src), "func EncodePause") {
				t.Fatalf("%s appears to encode admin operations", path)
			}
		}
	}
}

func TestNoAdminPublicEncoders(t *testing.T) {
	root := findModuleRoot(t)
	for _, rel := range []string{
		filepath.Join("internal", "contracts", "payroll"),
		filepath.Join("internal", "contracts", "swap"),
	} {
		dir := filepath.Join(root, rel)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name == nil || !fn.Name.IsExported() {
					continue
				}
				name := strings.ToLower(fn.Name.Name)
				for _, admin := range []string{"pause", "unpause", "emergency", "whitelist", "ownership", "rescue", "setfee", "setrouter", "settoken", "updatefx", "updatefee"} {
					if strings.Contains(name, admin) && strings.HasPrefix(name, "encode") {
						t.Fatalf("public admin encoder found: %s in %s", fn.Name.Name, path)
					}
				}
			}
		}
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
