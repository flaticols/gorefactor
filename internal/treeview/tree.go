package treeview

import (
	"fmt"
	"sort"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/rules"
)

// GroupMode controls how callers are grouped in the tree view.
type GroupMode string

const (
	GroupModeMethod GroupMode = "method"
	GroupModePkg    GroupMode = "pkg"
	GroupModeCaller GroupMode = "caller"
)

// TreeNode is the serialized node representation consumed by the RPC layer
// and the Neovim UI.
type TreeNode struct {
	ID          int            `json:"id"`
	Label       string         `json:"label"`
	Children    []TreeNode     `json:"children,omitempty"`
	File        string         `json:"file,omitempty"`
	Line        int            `json:"line,omitempty"`
	Violation   *NodeViolation `json:"violation,omitempty"`
	CallerCount int            `json:"callerCount,omitempty"`
}

// NodeViolation annotates a tree node with the matching deny rule.
type NodeViolation struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// BuildTree builds a tree for the requested function.
func BuildTree(g *graph.Graph, funcID int, mode GroupMode, violationsOnly bool, rs []rules.Rule) []TreeNode {
	if g == nil {
		return nil
	}
	g.Index()
	root, ok := g.FuncByID(funcID)
	if !ok {
		return nil
	}

	callers := g.CallersOf(root)
	switch mode {
	case GroupModePkg:
		return buildGroupedByPkg(g, root, callers, violationsOnly, rs)
	case GroupModeCaller:
		return buildGroupedByCaller(g, root, callers, violationsOnly, rs)
	default:
		return buildGroupedByMethod(g, root, callers, violationsOnly, rs)
	}
}

func buildGroupedByMethod(g *graph.Graph, root graph.Func, callers []graph.Func, violationsOnly bool, rs []rules.Rule) []TreeNode {
	sort.SliceStable(callers, func(i, j int) bool {
		if callers[i].Name != callers[j].Name {
			return callers[i].Name < callers[j].Name
		}
		return callers[i].File < callers[j].File
	})

	node := TreeNode{
		ID:          root.ID,
		Label:       root.Name,
		CallerCount: len(callers),
	}
	for _, caller := range callers {
		edge := edgeBetween(g, caller, root)
		viol := violationForEdge(g, edge, rs)
		if violationsOnly && viol == nil {
			continue
		}
		node.Children = append(node.Children, leafNode(caller, edge, viol))
	}
	return []TreeNode{node}
}

func buildGroupedByPkg(g *graph.Graph, root graph.Func, callers []graph.Func, violationsOnly bool, rs []rules.Rule) []TreeNode {
	groups := make(map[string][]graph.Func)
	for _, caller := range callers {
		groups[caller.Package] = append(groups[caller.Package], caller)
	}
	keys := sortedKeys(groups)
	var out []TreeNode
	for _, pkg := range keys {
		children := groups[pkg]
		sort.SliceStable(children, func(i, j int) bool {
			return children[i].Name < children[j].Name
		})
		groupNode := TreeNode{
			ID:          root.ID,
			Label:       fmt.Sprintf("%s (%d callers)", pkg, len(children)),
			CallerCount: len(children),
		}
		for _, caller := range children {
			edge := edgeBetween(g, caller, root)
			viol := violationForEdge(g, edge, rs)
			if violationsOnly && viol == nil {
				continue
			}
			groupNode.Children = append(groupNode.Children, leafNode(caller, edge, viol))
		}
		if len(groupNode.Children) > 0 || !violationsOnly {
			out = append(out, groupNode)
		}
	}
	return out
}

func buildGroupedByCaller(g *graph.Graph, root graph.Func, callers []graph.Func, violationsOnly bool, rs []rules.Rule) []TreeNode {
	groups := make(map[string][]graph.Func)
	for _, caller := range callers {
		groups[caller.Package+"."+caller.Name] = append(groups[caller.Package+"."+caller.Name], caller)
	}
	keys := sortedKeys(groups)
	var out []TreeNode
	for _, key := range keys {
		children := groups[key]
		sort.SliceStable(children, func(i, j int) bool {
			return children[i].File < children[j].File
		})
		groupNode := TreeNode{
			ID:          root.ID,
			Label:       fmt.Sprintf("%s (%d calls)", key, len(children)),
			CallerCount: len(children),
		}
		for _, caller := range children {
			edge := edgeBetween(g, caller, root)
			viol := violationForEdge(g, edge, rs)
			if violationsOnly && viol == nil {
				continue
			}
			groupNode.Children = append(groupNode.Children, leafNode(caller, edge, viol))
		}
		if len(groupNode.Children) > 0 || !violationsOnly {
			out = append(out, groupNode)
		}
	}
	return out
}

func leafNode(caller graph.Func, edge graph.Edge, viol *NodeViolation) TreeNode {
	label := caller.Package + "." + caller.Name
	if caller.Receiver != "" {
		label = caller.Package + ".(" + caller.Receiver + ")." + caller.Name
	}
	return TreeNode{
		ID:        caller.ID,
		Label:     label,
		File:      edge.File,
		Line:      edge.Line,
		Violation: viol,
	}
}

func violationForEdge(g *graph.Graph, edge graph.Edge, rs []rules.Rule) *NodeViolation {
	if edge.Caller == 0 || edge.Callee == 0 {
		return nil
	}
	if rule := rules.CheckEdge(g, edge, rs); rule != nil {
		return &NodeViolation{From: rule.From, To: rule.To, Reason: rule.Reason}
	}
	return nil
}

func edgeBetween(g *graph.Graph, caller, callee graph.Func) graph.Edge {
	for _, edge := range g.Edges {
		if edge.Caller == caller.ID && edge.Callee == callee.ID {
			return edge
		}
	}
	return graph.Edge{Caller: caller.ID, Callee: callee.ID}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
