package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.flaticols.dev/gorefactor/internal/loader"
)

func TestBuildPackageGraph(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

import "fmt"

var ErrNotFound = fmt.Errorf("not found")
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Run() error { return alpha.ErrNotFound }
`)

	pg, err := loader.BuildPackageGraph(loader.Config{
		Dir:      dir,
		Patterns: []string{"./..."},
	})
	if err != nil {
		t.Fatalf("BuildPackageGraph() error = %v", err)
	}

	importers := pg.ImportersOf("example.com/test/alpha")
	if len(importers) != 1 || importers[0].Path != "example.com/test/beta" {
		t.Fatalf("ImportersOf = %v", importers)
	}

	paths := pg.AllPaths()
	if len(paths) < 2 {
		t.Fatalf("AllPaths len = %d, want >= 2", len(paths))
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}
