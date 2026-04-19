package graph

import "testing"

func TestGraphIndexesAndLookups(t *testing.T) {
	g := &Graph{
		Funcs: []Func{
			{ID: 1, Name: "(*TaskRunner).Execute", Receiver: "*TaskRunner", Package: "tasks", File: "tasks/process.go", Line: 123, Exported: true},
			{ID: 2, Name: "(*TaxEngine).Calculate", Receiver: "*TaxEngine", Package: "adapters", File: "adapters/engine.go", Line: 45, Exported: true},
			{ID: 3, Name: "legacyLookup", Package: "tasks", File: "tasks/legacy.go", Line: 234},
		},
		Edges: []Edge{
			{Caller: 1, Callee: 2, File: "tasks/process.go", Line: 123},
			{Caller: 3, Callee: 2, File: "tasks/legacy.go", Line: 234},
			{Caller: 3, Callee: 2, File: "tasks/legacy.go", Line: 301},
		},
	}

	g.Index()

	if got, ok := g.FuncByID(2); !ok || got.Name != "(*TaxEngine).Calculate" {
		t.Fatalf("FuncByID = %#v, %v", got, ok)
	}

	callers := g.CallersOf(g.Funcs[1])
	if len(callers) != 2 {
		t.Fatalf("CallersOf len = %d, want 2", len(callers))
	}

	callees := g.CalleesOf(g.Funcs[2])
	if len(callees) != 1 || callees[0].ID != 2 {
		t.Fatalf("CalleesOf = %#v", callees)
	}

	methods := g.ExportedMethods("tasks", "*TaskRunner")
	if len(methods) != 1 || methods[0].ID != 1 {
		t.Fatalf("ExportedMethods = %#v", methods)
	}

	items := g.AllSearchItems()
	if len(items) != 5 {
		t.Fatalf("AllSearchItems len = %d, want 5 (%#v)", len(items), items)
	}

	count := g.CallCount(g.Funcs[0], g.Funcs[1])
	if count.Count != 1 || len(count.Lines) != 1 || count.Lines[0] != 123 {
		t.Fatalf("CallCount = %#v", count)
	}

	fn, ok := g.FuncAtPos("tasks/legacy.go", 300)
	if !ok || fn.ID != 3 {
		t.Fatalf("FuncAtPos fallback = %#v, %v", fn, ok)
	}
}

func TestEdgeKindOnGraph(t *testing.T) {
	g := &Graph{
		Funcs: []Func{
			{ID: 1, Name: "Caller", Package: "a", File: "a/a.go", Line: 1},
			{ID: 2, Name: "Callee", Package: "b", File: "b/b.go", Line: 1},
		},
		Edges: []Edge{
			{Caller: 1, Callee: 2, Kind: EdgeCall, File: "a/a.go", Line: 5},
		},
	}
	g.Index()
	callers := g.CallersOf(g.Funcs[1])
	if len(callers) != 1 {
		t.Fatalf("CallersOf len = %d, want 1", len(callers))
	}
}

func TestSymbolIndexing(t *testing.T) {
	g := &Graph{
		Symbols: []Symbol{
			{ID: 10, Kind: "var", Name: "ErrNotFound", Package: "tasks", File: "tasks/errors.go", Line: 5, Exported: true},
			{ID: 11, Kind: "const", Name: "MaxRetries", Package: "tasks", File: "tasks/config.go", Line: 3, Exported: true},
		},
	}
	g.Index()

	sym, ok := g.SymbolByID(10)
	if !ok || sym.Name != "ErrNotFound" {
		t.Fatalf("SymbolByID(10) = %v, %v", sym, ok)
	}

	syms := g.SymbolsInPackage("tasks")
	if len(syms) != 2 {
		t.Fatalf("SymbolsInPackage len = %d, want 2", len(syms))
	}
}

func TestEdgeSamePkgFlag(t *testing.T) {
	g := &Graph{
		Funcs: []Func{
			{ID: 1, Name: "A", Package: "pkg", File: "pkg/a.go", Line: 1},
			{ID: 2, Name: "B", Package: "pkg", File: "pkg/b.go", Line: 1},
		},
		Edges: []Edge{
			{Caller: 1, Callee: 2, Kind: EdgeCall, SamePkg: true, File: "pkg/a.go", Line: 5},
		},
	}
	g.Index()
	if !g.Edges[0].SamePkg {
		t.Fatal("expected SamePkg=true")
	}
}
