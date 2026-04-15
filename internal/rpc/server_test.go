package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/rules"
)

func TestServerDispatchesRequests(t *testing.T) {
	g := &graph.Graph{
		Funcs: []graph.Func{
			{ID: 1, Name: "Execute", Package: "tasks", File: "tasks/process.go", Line: 123, Col: 5},
			{ID: 2, Name: "Calculate", Package: "adapters", File: "adapters/engine.go", Line: 45, Col: 3},
		},
		Edges: []graph.Edge{
			{Caller: 1, Callee: 2, File: "tasks/process.go", Line: 123, Col: 5},
		},
	}
	rs := []rules.Rule{{From: "tasks", To: "adapters", Reason: "tasks must not depend on adapters"}}

	var in bytes.Buffer
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"gorefact.search","params":{"query":"Exec"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"gorefact.tree","params":{"id":2,"group":"method"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"gorefact.detail","params":{"nodeID":2}}`,
		`{"jsonrpc":"2.0","id":4,"method":"gorefact.funcAtPos","params":{"file":"tasks/process.go","line":123}}`,
		`{"jsonrpc":"2.0","id":5,"method":"gorefact.check","params":{}}`,
	}
	in.WriteString(strings.Join(requests, "\n"))
	in.WriteByte('\n')

	var out bytes.Buffer
	srv := NewServer(g, rs, &out)
	if err := srv.Serve(context.Background(), &in); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("got %d responses, want 5: %s", len(lines), out.String())
	}

	assertJSONContains(t, lines[0], `"name":"tasks.Execute"`)
	assertJSONContains(t, lines[1], `"violation":{"from":"tasks","to":"adapters","reason":"tasks must not depend on adapters"}`)
	assertJSONContains(t, lines[2], `"callCount":1`)
	assertJSONContains(t, lines[2], `"callLines":[123]`)
	assertJSONContains(t, lines[3], `"id":1`)
	assertJSONContains(t, lines[4], `"total_violations":1`)
}

func assertJSONContains(t *testing.T, raw, substr string) {
	t.Helper()
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !strings.Contains(raw, substr) {
		t.Fatalf("response %s does not contain %s", raw, substr)
	}
}
