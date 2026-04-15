package graph

import (
	"sort"
	"strings"
)

// Func describes a function or method in the call graph.
type Func struct {
	ID        int
	Name      string
	Receiver  string
	Signature string
	Package   string
	File      string
	Line      int
	Col       int
	Exported  bool
}

// Edge describes a call site from one function to another.
type Edge struct {
	Caller  int
	Callee  int
	File    string
	Line    int
	Col     int
	Dynamic bool
}

// CallCount carries the number of calls between two functions plus the
// originating source locations.
type CallCount struct {
	Count int
	Lines []int
}

// Graph stores discovered functions and call edges together with indexes for
// common lookups used by the CLI and RPC server.
type Graph struct {
	Funcs []Func
	Edges []Edge

	byID         map[int]int
	byName       map[string][]int
	byPkgRecv    map[string][]int
	callersIndex map[int][]int
	calleesIndex map[int][]int
}

// Index builds all in-memory indexes. It is safe to call multiple times.
func (g *Graph) Index() {
	g.byID = make(map[int]int, len(g.Funcs))
	g.byName = make(map[string][]int)
	g.byPkgRecv = make(map[string][]int)
	g.callersIndex = make(map[int][]int)
	g.calleesIndex = make(map[int][]int)

	for i := range g.Funcs {
		f := g.Funcs[i]
		g.byID[f.ID] = i
		g.byName[normalizeSearchName(f.Name)] = append(g.byName[normalizeSearchName(f.Name)], f.ID)
		if key := pkgRecvKey(f.Package, f.Receiver); key != "" {
			g.byPkgRecv[key] = append(g.byPkgRecv[key], f.ID)
		}
	}

	for i := range g.Edges {
		e := g.Edges[i]
		g.callersIndex[e.Callee] = append(g.callersIndex[e.Callee], e.Caller)
		g.calleesIndex[e.Caller] = append(g.calleesIndex[e.Caller], e.Callee)
	}

	// Keep lookups stable for tests and downstream consumers.
	for key, ids := range g.byName {
		sort.Ints(ids)
		g.byName[key] = uniqueInts(ids)
	}
	for key, ids := range g.byPkgRecv {
		sort.Ints(ids)
		g.byPkgRecv[key] = uniqueInts(ids)
	}
	for key, ids := range g.callersIndex {
		sort.Ints(ids)
		g.callersIndex[key] = uniqueInts(ids)
	}
	for key, ids := range g.calleesIndex {
		sort.Ints(ids)
		g.calleesIndex[key] = uniqueInts(ids)
	}
}

// FuncByID returns the function for the given identifier.
func (g *Graph) FuncByID(id int) (Func, bool) {
	if g.byID == nil {
		g.Index()
	}
	i, ok := g.byID[id]
	if !ok {
		return Func{}, false
	}
	return g.Funcs[i], true
}

// CallersOf returns all callers for the callee function.
func (g *Graph) CallersOf(f Func) []Func {
	if g.callersIndex == nil {
		g.Index()
	}
	ids := g.callersIndex[f.ID]
	return g.funcsForIDs(ids)
}

// CalleesOf returns all callees for the caller function.
func (g *Graph) CalleesOf(f Func) []Func {
	if g.calleesIndex == nil {
		g.Index()
	}
	ids := g.calleesIndex[f.ID]
	return g.funcsForIDs(ids)
}

// ExportedMethods returns exported methods for a package/receiver pair.
func (g *Graph) ExportedMethods(pkg, receiver string) []Func {
	if g.byPkgRecv == nil {
		g.Index()
	}
	ids := g.byPkgRecv[pkgRecvKey(pkg, receiver)]
	funcs := g.funcsForIDs(ids)
	out := funcs[:0]
	for _, fn := range funcs {
		if fn.Exported && fn.Receiver != "" {
			out = append(out, fn)
		}
	}
	return append([]Func(nil), out...)
}

// AllSearchItems returns a de-duplicated list of names suitable for fuzzy
// search across functions and struct receiver names.
func (g *Graph) AllSearchItems() []string {
	if g.byName == nil {
		g.Index()
	}
	seen := make(map[string]struct{})
	var items []string
	for _, fn := range g.Funcs {
		if fn.Name != "" {
			if _, ok := seen[fn.Name]; !ok {
				seen[fn.Name] = struct{}{}
				items = append(items, fn.Name)
			}
		}
		if fn.Receiver != "" {
			recv := fn.Package + "." + fn.Receiver
			if _, ok := seen[recv]; !ok {
				seen[recv] = struct{}{}
				items = append(items, recv)
			}
		}
	}
	sort.Strings(items)
	return items
}

// CallCount returns the number of call edges between two functions and the
// source lines for those calls.
func (g *Graph) CallCount(callerFunc, calleeFunc Func) CallCount {
	if g.Edges == nil {
		return CallCount{}
	}
	var lines []int
	for _, e := range g.Edges {
		if e.Caller == callerFunc.ID && e.Callee == calleeFunc.ID {
			lines = append(lines, e.Line)
		}
	}
	sort.Ints(lines)
	return CallCount{Count: len(lines), Lines: append([]int(nil), lines...)}
}

// FuncAtPos resolves the function covering a file:line position.
func (g *Graph) FuncAtPos(file string, line int) (Func, bool) {
	if g.byID == nil {
		g.Index()
	}
	for _, fn := range g.Funcs {
		if fn.File == file && fn.Line == line {
			return fn, true
		}
	}
	// Best-effort fallback: prefer the closest function at or before the line.
	var (
		best    Func
		found   bool
		bestGap = int(^uint(0) >> 1)
	)
	for _, fn := range g.Funcs {
		if fn.File != file || fn.Line > line {
			continue
		}
		gap := line - fn.Line
		if !found || gap < bestGap || (gap == bestGap && fn.Line > best.Line) {
			best = fn
			bestGap = gap
			found = true
		}
	}
	return best, found
}

func (g *Graph) funcsForIDs(ids []int) []Func {
	if len(ids) == 0 {
		return nil
	}
	out := make([]Func, 0, len(ids))
	for _, id := range ids {
		if fn, ok := g.FuncByID(id); ok {
			out = append(out, fn)
		}
	}
	return out
}

func normalizeSearchName(name string) string {
	return strings.TrimSpace(strings.ToLower(name))
}

func pkgRecvKey(pkg, receiver string) string {
	pkg = strings.TrimSpace(pkg)
	receiver = strings.TrimSpace(receiver)
	if pkg == "" || receiver == "" {
		return ""
	}
	return pkg + "\x00" + receiver
}

func uniqueInts(ids []int) []int {
	if len(ids) < 2 {
		return append([]int(nil), ids...)
	}
	out := make([]int, 0, len(ids))
	last := ids[0]
	out = append(out, last)
	for _, id := range ids[1:] {
		if id == last {
			continue
		}
		last = id
		out = append(out, id)
	}
	return out
}
