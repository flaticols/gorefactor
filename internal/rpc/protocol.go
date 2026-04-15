package rpc

import (
	"encoding/json"

	"go.flaticols.dev/gorefactor/internal/rules"
	"go.flaticols.dev/gorefactor/internal/treeview"
)

// Request models a JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response models a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is returned on protocol or method failures.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// SearchRequest asks for fuzzy matches over graph symbols.
type SearchRequest struct {
	Query string `json:"query"`
}

// SearchResult describes a function or receiver cluster.
type SearchResult struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Pkg         string `json:"pkg"`
	Kind        string `json:"kind"`
	MethodCount int    `json:"methodCount,omitempty"`
	CallerCount int    `json:"callerCount,omitempty"`
}

// SearchResponse returns matching symbols.
type SearchResponse []SearchResult

// TreeRequest asks for callers around a function.
type TreeRequest struct {
	ID             int    `json:"id"`
	Group          string `json:"group,omitempty"`
	ViolationsOnly bool   `json:"violationsOnly,omitempty"`
}

// TreeResponse returns the tree nodes.
type TreeResponse struct {
	Nodes []treeview.TreeNode `json:"nodes"`
}

// DetailRequest asks for the full detail of a tree node.
type DetailRequest struct {
	NodeID int `json:"nodeID"`
}

// DetailViolation mirrors a rule hit in the detail view.
type DetailViolation struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// DetailResponse returns the detailed metadata for a function.
type DetailResponse struct {
	Pkg        string            `json:"pkg"`
	Func       string            `json:"func"`
	File       string            `json:"file"`
	Line       int               `json:"line"`
	Signature  string            `json:"signature"`
	CallCount  int               `json:"callCount"`
	CallLines  []int             `json:"callLines"`
	Violations []DetailViolation `json:"violations,omitempty"`
}

// FuncAtPosRequest resolves the function at a file:line location.
type FuncAtPosRequest struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// FuncAtPosResponse returns the function descriptor or null.
type FuncAtPosResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Pkg  string `json:"pkg"`
	Kind string `json:"kind"`
}

// CheckRequest asks the server to re-run rules over the in-memory graph.
type CheckRequest struct{}

// CheckResponse returns the same shape used by the batch formatter.
type CheckResponse struct {
	Violations []rules.Violation `json:"violations"`
	Summary    struct {
		TotalViolations int `json:"total_violations"`
		RulesViolated   int `json:"rules_violated"`
		RulesTotal      int `json:"rules_total"`
	} `json:"summary"`
}

// Notification is emitted by the server without an associated request ID.
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}
