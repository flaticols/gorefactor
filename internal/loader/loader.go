// Package loader builds the package import graph and walks source references.
package loader

import (
	"path/filepath"
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
