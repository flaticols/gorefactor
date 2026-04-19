package loader_test

import (
	"path/filepath"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/loader"
)

func TestWalkRefs_FuncCall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

func DoWork() {}
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Run() { alpha.DoWork() }
`)

	result, err := loader.WalkRefs("example.com/test/alpha", []string{"example.com/test/beta"}, loader.Config{
		Dir:      dir,
		Patterns: []string{"./..."},
	}, 100)
	if err != nil {
		t.Fatalf("WalkRefs() error = %v", err)
	}

	var foundSym bool
	for _, s := range result.Symbols {
		if s.Name == "DoWork" && s.Kind == "func" {
			foundSym = true
		}
	}
	if !foundSym {
		t.Fatalf("expected DoWork symbol, got %v", result.Symbols)
	}

	var foundEdge bool
	for _, e := range result.Edges {
		if e.Kind == graph.EdgeCall {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("expected a call edge, got %v", result.Edges)
	}
}

func TestWalkRefs_VarRead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

import "errors"

var ErrNotFound = errors.New("not found")
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Check(err error) bool { return err == alpha.ErrNotFound }
`)

	result, err := loader.WalkRefs("example.com/test/alpha", []string{"example.com/test/beta"}, loader.Config{
		Dir:      dir,
		Patterns: []string{"./..."},
	}, 100)
	if err != nil {
		t.Fatalf("WalkRefs() error = %v", err)
	}

	var foundVar bool
	for _, s := range result.Symbols {
		if s.Name == "ErrNotFound" && s.Kind == "var" {
			foundVar = true
		}
	}
	if !foundVar {
		t.Fatalf("expected ErrNotFound symbol, got %v", result.Symbols)
	}

	var foundEdge bool
	for _, e := range result.Edges {
		if e.Kind == graph.EdgeRead {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("expected a read edge, got %v", result.Edges)
	}
}

func TestWalkRefs_TypeRef(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

type Config struct{ Timeout int }
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func New(cfg alpha.Config) {}
`)

	result, err := loader.WalkRefs("example.com/test/alpha", []string{"example.com/test/beta"}, loader.Config{
		Dir:      dir,
		Patterns: []string{"./..."},
	}, 100)
	if err != nil {
		t.Fatalf("WalkRefs() error = %v", err)
	}

	var foundType bool
	for _, s := range result.Symbols {
		if s.Name == "Config" && s.Kind == "type" {
			foundType = true
		}
	}
	if !foundType {
		t.Fatalf("expected Config type symbol, got %v", result.Symbols)
	}

	var foundEdge bool
	for _, e := range result.Edges {
		if e.Kind == graph.EdgeTypeRef {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("expected a typeref edge, got %v", result.Edges)
	}
}
