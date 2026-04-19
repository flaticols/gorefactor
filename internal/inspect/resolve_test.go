package inspect_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
	"go.flaticols.dev/gorefactor/internal/rules"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		input  string
		pkg    string
		sym    string
		method string
	}{
		{"github.com/acme/tasks", "github.com/acme/tasks", "", ""},
		{"github.com/acme/tasks.Run", "github.com/acme/tasks", "Run", ""},
		{"github.com/acme/tasks.Engine.Calculate", "github.com/acme/tasks", "Engine", "Calculate"},
		{"tasks", "tasks", "", ""},
		{"tasks.Run", "tasks", "Run", ""},
	}
	for _, tc := range tests {
		pkg, sym, meth := inspect.ParseTarget(tc.input)
		if pkg != tc.pkg || sym != tc.sym || meth != tc.method {
			t.Errorf("ParseTarget(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tc.input, pkg, sym, meth, tc.pkg, tc.sym, tc.method)
		}
	}
}

func TestResolveTarget_Package(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

type Engine struct{}

func NewEngine() Engine { return Engine{} }

const Version = "1.0"
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Run() alpha.Engine { return alpha.NewEngine() }
`)

	cfg := inspect.Config{
		Loader: loader.Config{Dir: dir, Patterns: []string{"./..."}},
	}
	res, err := inspect.ResolveTarget("example.com/test/alpha", cfg)
	if err != nil {
		t.Fatalf("ResolveTarget error = %v", err)
	}
	if res.PkgPath != "example.com/test/alpha" {
		t.Errorf("PkgPath = %q", res.PkgPath)
	}
	if len(res.Symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}
	if len(res.Edges) == 0 {
		t.Fatal("expected edges from beta→alpha, got none")
	}
	for _, e := range res.Edges {
		if e.CallerPkg == "" {
			t.Error("edge missing CallerPkg")
		}
	}
}

func TestResolveTarget_Symbol(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

type Engine struct{}

func NewEngine() Engine { return Engine{} }
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Run() alpha.Engine { return alpha.NewEngine() }
`)

	cfg := inspect.Config{
		Loader: loader.Config{Dir: dir, Patterns: []string{"./..."}},
	}
	res, err := inspect.ResolveTarget("example.com/test/alpha.Engine", cfg)
	if err != nil {
		t.Fatalf("ResolveTarget error = %v", err)
	}
	if res.SymbolName != "Engine" {
		t.Errorf("SymbolName = %q, want Engine", res.SymbolName)
	}
	for _, s := range res.Symbols {
		if s.Name != "Engine" {
			t.Errorf("unexpected symbol %q in filtered result", s.Name)
		}
	}
}

func TestResolveTarget_Violations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

type Engine struct{}
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Run() alpha.Engine { return alpha.Engine{} }
`)

	cfg := inspect.Config{
		Loader: loader.Config{Dir: dir, Patterns: []string{"./..."}},
		Rules:  []rules.Rule{{From: "beta", To: "alpha", Reason: "test rule"}},
	}
	res, err := inspect.ResolveTarget("example.com/test/alpha", cfg)
	if err != nil {
		t.Fatalf("ResolveTarget error = %v", err)
	}
	if len(res.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(res.Violations))
	}
	if res.Violations[0].FromPkg != "example.com/test/beta" {
		t.Errorf("violation FromPkg = %q", res.Violations[0].FromPkg)
	}
}
