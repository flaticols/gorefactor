package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildLoadsPackagesAndEdges(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "task", "task.go"), `package task

type Runner struct{}

func (Runner) Execute() {}

func legacyLookup() {}

func init() {
	var r Runner
	r.Execute()
	legacyLookup()
}
`)

	g, err := Build(BuildConfig{
		Dir:      dir,
		Patterns: []string{"./..."},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	execFn, ok := findFunc(g, "Execute", "Runner")
	if !ok {
		t.Fatalf("missing Execute method: %#v", g.Funcs)
	}
	legacyFn, ok := findFunc(g, "legacyLookup", "")
	if !ok {
		t.Fatalf("missing legacyLookup function: %#v", g.Funcs)
	}
	initFn, ok := findFunc(g, "init", "")
	if !ok {
		t.Fatalf("missing init function: %#v", g.Funcs)
	}

	count := g.CallCount(initFn, execFn)
	if count.Count == 0 {
		t.Fatalf("expected init -> Execute call, got %#v", count)
	}
	count = g.CallCount(initFn, legacyFn)
	if count.Count == 0 {
		t.Fatalf("expected init -> legacyLookup call, got %#v", count)
	}

	if got, ok := g.FuncAtPos("task/task.go", legacyFn.Line); !ok || got.ID != legacyFn.ID {
		t.Fatalf("FuncAtPos = %#v, %v", got, ok)
	}
}

func findFunc(g *Graph, name, receiver string) (Func, bool) {
	for _, fn := range g.Funcs {
		if fn.Name == name && fn.Receiver == receiver {
			return fn, true
		}
	}
	return Func{}, false
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
