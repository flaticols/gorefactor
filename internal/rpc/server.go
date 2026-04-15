package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/rules"
	"go.flaticols.dev/gorefactor/internal/treeview"
)

// Server serves JSON-RPC requests over newline-delimited stdin/stdout.
type Server struct {
	graph *graph.Graph
	rules []rules.Rule
	out   io.Writer
}

// NewServer constructs a server around an in-memory graph and rule set.
func NewServer(g *graph.Graph, rs []rules.Rule, out io.Writer) *Server {
	if g != nil {
		g.Index()
	}
	return &Server{graph: g, rules: append([]rules.Rule(nil), rs...), out: out}
}

// Serve reads requests until EOF or context cancellation.
func (s *Server) Serve(ctx context.Context, in io.Reader) error {
	if s.out == nil {
		return errors.New("rpc: missing output writer")
	}
	scanner := bufio.NewScanner(in)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = s.writeError(nil, -32700, "invalid JSON", err)
			continue
		}
		if req.JSONRPC == "" {
			req.JSONRPC = "2.0"
		}
		if req.ID == nil {
			// Notification: execute for side effects only.
			_, _ = s.dispatch(req.Method, req.Params)
			continue
		}

		result, rpcErr := s.dispatch(req.Method, req.Params)
		resp := Response{JSONRPC: "2.0", ID: req.ID}
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}
		if err := s.write(resp); err != nil {
			return err
		}
	}
}

func (s *Server) dispatch(method string, params json.RawMessage) (any, *RPCError) {
	switch method {
	case "gorefact.search":
		var req SearchRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, protocolError(-32602, "invalid params", err)
		}
		return s.search(req.Query), nil
	case "gorefact.tree":
		var req TreeRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, protocolError(-32602, "invalid params", err)
		}
		mode := treeview.GroupMode(req.Group)
		if mode == "" {
			mode = treeview.GroupModeMethod
		}
		return TreeResponse{Nodes: treeview.BuildTree(s.graph, req.ID, mode, req.ViolationsOnly, s.rules)}, nil
	case "gorefact.detail":
		var req DetailRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, protocolError(-32602, "invalid params", err)
		}
		return s.detail(req.NodeID), nil
	case "gorefact.funcAtPos":
		var req FuncAtPosRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, protocolError(-32602, "invalid params", err)
		}
		return s.funcAtPos(req.File, req.Line), nil
	case "gorefact.check":
		return s.check(), nil
	default:
		return nil, protocolError(-32601, "method not found", method)
	}
}

func (s *Server) search(query string) SearchResponse {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" || s.graph == nil {
		return nil
	}

	type scored struct {
		item  SearchResult
		score int
	}
	var results []scored
	seenStruct := make(map[string]struct{})
	for _, fn := range s.graph.Funcs {
		name := fn.Name
		full := fn.Package + "." + name
		recv := fn.Package + "." + fn.Receiver
		if score := searchScore(query, name, full, recv); score > 0 {
			kind := "function"
			if fn.Receiver != "" {
				kind = "method"
			}
			results = append(results, scored{
				item: SearchResult{
					ID:          fn.ID,
					Name:        fullName(fn),
					Pkg:         fn.Package,
					Kind:        kind,
					CallerCount: len(s.graph.CallersOf(fn)),
				},
				score: score,
			})
		}
		if fn.Receiver == "" {
			continue
		}
		key := fn.Package + "\x00" + fn.Receiver
		if _, ok := seenStruct[key]; ok {
			continue
		}
		if score := searchScore(query, fn.Receiver, fn.Package+"."+fn.Receiver, fn.Package+"."+fn.Name); score > 0 {
			seenStruct[key] = struct{}{}
			methods := methodsForReceiver(s.graph, fn.Package, fn.Receiver)
			results = append(results, scored{
				item: SearchResult{
					ID:          fn.ID,
					Name:        fn.Package + "." + fn.Receiver,
					Pkg:         fn.Package,
					Kind:        "struct",
					MethodCount: len(methods),
					CallerCount: uniqueCallerCount(s.graph, methods),
				},
				score: score + len(methods),
			})
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		if results[i].item.Name != results[j].item.Name {
			return results[i].item.Name < results[j].item.Name
		}
		return results[i].item.ID < results[j].item.ID
	})

	out := make(SearchResponse, 0, len(results))
	for _, r := range results {
		out = append(out, r.item)
	}
	return out
}

func (s *Server) detail(nodeID int) DetailResponse {
	if s.graph == nil {
		return DetailResponse{}
	}
	fn, ok := s.graph.FuncByID(nodeID)
	if !ok {
		return DetailResponse{}
	}
	var (
		callLines  []int
		violations []DetailViolation
	)
	for _, edge := range s.graph.Edges {
		if edge.Callee != nodeID {
			continue
		}
		if edge.Line > 0 {
			callLines = append(callLines, edge.Line)
		}
		if rule := rules.CheckEdge(s.graph, edge, s.rules); rule != nil {
			violations = append(violations, DetailViolation{From: rule.From, To: rule.To, Reason: rule.Reason})
		}
	}
	sort.Ints(callLines)
	return DetailResponse{
		Pkg:        fn.Package,
		Func:       fn.Name,
		File:       fn.File,
		Line:       fn.Line,
		Signature:  fn.Signature,
		CallCount:  len(callLines),
		CallLines:  append([]int(nil), callLines...),
		Violations: violations,
	}
}

func (s *Server) funcAtPos(file string, line int) *FuncAtPosResponse {
	if s.graph == nil {
		return nil
	}
	fn, ok := s.graph.FuncAtPos(file, line)
	if !ok {
		return nil
	}
	kind := "function"
	if fn.Receiver != "" {
		kind = "method"
	}
	return &FuncAtPosResponse{
		ID:   fn.ID,
		Name: fn.Name,
		Pkg:  fn.Package,
		Kind: kind,
	}
}

func (s *Server) check() CheckResponse {
	violations := rules.Check(s.graph, s.rules)
	resp := CheckResponse{Violations: make([]rules.Violation, len(violations))}
	copy(resp.Violations, violations)
	resp.Summary.TotalViolations = len(violations)
	resp.Summary.RulesViolated = countViolatedRules(violations)
	resp.Summary.RulesTotal = len(s.rules)
	return resp
}

func (s *Server) write(v any) error {
	enc := json.NewEncoder(s.out)
	return enc.Encode(v)
}

func (s *Server) writeError(id json.RawMessage, code int, msg string, data any) error {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: msg,
			Data:    data,
		},
	}
	return s.write(resp)
}

func protocolError(code int, msg string, data any) *RPCError {
	return &RPCError{Code: code, Message: msg, Data: data}
}

func searchScore(query string, values ...string) int {
	best := 0
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		switch {
		case value == query:
			best = max(best, 100)
		case strings.HasPrefix(value, query):
			best = max(best, 90-len(value)+len(query))
		case strings.Contains(value, query):
			best = max(best, 50-len(value)+len(query))
		}
	}
	return best
}

func fullName(fn graph.Func) string {
	if fn.Receiver == "" {
		return fn.Package + "." + fn.Name
	}
	return fn.Package + ".(" + fn.Receiver + ")." + fn.Name
}

func uniqueCallerCount(g *graph.Graph, funcs []graph.Func) int {
	seen := make(map[int]struct{})
	for _, fn := range funcs {
		for _, caller := range g.CallersOf(fn) {
			seen[caller.ID] = struct{}{}
		}
	}
	return len(seen)
}

func methodsForReceiver(g *graph.Graph, pkg, receiver string) []graph.Func {
	var out []graph.Func
	for _, fn := range g.Funcs {
		if fn.Package == pkg && fn.Receiver == receiver {
			out = append(out, fn)
		}
	}
	return out
}

func countViolatedRules(violations []rules.Violation) int {
	seen := make(map[string]struct{})
	for _, v := range violations {
		key := v.Rule.From + "\x00" + v.Rule.To + "\x00" + v.Rule.Reason
		seen[key] = struct{}{}
	}
	return len(seen)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
