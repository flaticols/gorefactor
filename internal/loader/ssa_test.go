package loader_test

import (
	"path/filepath"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/loader"
)

func TestSSALoadsPackagesAndEdges(t *testing.T) {
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

	res, err := loader.Load(loader.Config{
		Dir:      dir,
		Patterns: []string{"./..."},
	}, loader.DepthFull)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	g := res.Graph

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

func findFunc(g *graph.Graph, name, receiver string) (graph.Func, bool) {
	for _, fn := range g.Funcs {
		if fn.Name == name && fn.Receiver == receiver {
			return fn, true
		}
	}
	return graph.Func{}, false
}
