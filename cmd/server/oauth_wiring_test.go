package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// This source-contract regression complements the transport test: that test
// supplies its own registry, so alone it cannot detect loss of bootstrap selection.
// Parsing does not run bootstrap, connect to a database or open a listener.
func TestProductionOAuthBootstrapSelectsReadOnlyRegistry(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	ast.Inspect(file, func(node ast.Node) bool {
		branch, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		cond, ok := branch.Cond.(*ast.SelectorExpr)
		if !ok || cond.Sel.Name != "OAuthEnabled" {
			return true
		}
		cfg, ok := cond.X.(*ast.Ident)
		if !ok || cfg.Name != "cfg" {
			return true
		}
		var constructor, selection, application token.Pos
		var readRegistry string
		for _, stmt := range branch.Body.List {
			assignment, ok := stmt.(*ast.AssignStmt)
			if !ok {
				continue
			}
			for _, rhs := range assignment.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				receiver, ok := selector.X.(*ast.Ident)
				if !ok {
					continue
				}
				if receiver.Name == "tools" && selector.Sel.Name == "NewOAuthReadOnlyRegistry" {
					id, ok := assignment.Lhs[0].(*ast.Ident)
					if !ok {
						t.Fatal("unrecognized read registry assignment")
					}
					readRegistry = id.Name
					constructor = stmt.Pos()
				}
				if receiver.Name == readRegistry && readRegistry != "" && selector.Sel.Name == "Tools" && assignment.Tok == token.ASSIGN && len(assignment.Lhs) == 1 {
					id, ok := assignment.Lhs[0].(*ast.Ident)
					if ok && id.Name == "registrations" {
						selection = stmt.Pos()
					}
				}
				if receiver.Name == "app" && selector.Sel.Name == "NewOAuthServerWithBrowserFoundation" {
					application = stmt.Pos()
					if len(call.Args) == 0 || !call.Ellipsis.IsValid() {
						t.Fatal("OAuth constructor missing registrations")
					}
					id, ok := call.Args[len(call.Args)-1].(*ast.Ident)
					if !ok || id.Name != "registrations" {
						t.Fatal("OAuth constructor bypasses selected registrations")
					}
				}
			}
		}
		if application.IsValid() {
			if !constructor.IsValid() || !selection.IsValid() || constructor >= selection || selection >= application {
				t.Fatal("production OAuth branch does not select restricted registry before construction")
			}
			matched = true
		}
		return true
	})
	if !matched {
		t.Fatal("production OAuth application construction not found")
	}
}
