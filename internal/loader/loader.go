package loader

import (
	"path/filepath"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
)

// Config configures any loader tier.
type Config struct {
	Dir       string
	Tests     bool
	FilterPkg string
	Patterns  []string
	Progress  func(string)
}

func (c Config) patterns() []string {
	if len(c.Patterns) == 0 {
		return []string{"./..."}
	}
	return c.Patterns
}

func (c Config) progress(stage string) {
	if c.Progress != nil {
		c.Progress(stage)
	}
}

func (c Config) filterPkg() string {
	return strings.TrimSpace(c.FilterPkg)
}

// cleanPath makes file paths relative to base for stable output.
func cleanPath(base, file string) string {
	if file == "" {
		return ""
	}
	file = filepath.Clean(file)
	if base == "" {
		return filepath.ToSlash(file)
	}
	if !filepath.IsAbs(base) {
		if abs, err := filepath.Abs(base); err == nil {
			base = abs
		}
	}
	if rel, err := filepath.Rel(base, file); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}

// Depth controls which tiers are activated.
type Depth int

const (
	// DepthFull runs T1 + T3 (SSA). Used by check and serve commands.
	DepthFull Depth = iota
	// DepthFast runs T1 only. T2 is invoked separately via WalkRefs.
	DepthFast
)

// Result is the output of Load.
type Result struct {
	// Packages is the T1 package import graph. Always populated.
	Packages *PackageGraph
	// Graph is the T3 SSA call graph. Populated when Depth == DepthFull.
	Graph *graph.Graph
}

// Load runs T1 always. When depth == DepthFull, also runs T3 (SSA/CHA).
// Note: DepthFull triggers a second packages.Load internally (SSA). The T1
// result is still populated for callers that need the package import graph
// (e.g. the inspect command). The check/serve commands only use res.Graph.
func Load(cfg Config, depth Depth) (*Result, error) {
	cfg.progress("loading package graph")
	pg, err := BuildPackageGraph(cfg)
	if err != nil {
		return nil, err
	}

	result := &Result{Packages: pg}

	if depth == DepthFull {
		g, err := buildSSA(cfg)
		if err != nil {
			return nil, err
		}
		result.Graph = g
	}

	return result, nil
}
