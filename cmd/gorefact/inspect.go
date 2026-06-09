package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
	"go.flaticols.dev/gorefactor/internal/tui"
)

// exploreOpts holds the root-command flags.
type exploreOpts struct {
	dir            string
	format         string
	tests          bool
	filterPkg      string
	moduleOnly     bool
	formatExplicit bool
}

// runExplore is the root command: open the TUI on a terminal, or print a
// package report otherwise. positional is [dir] [target] — the first arg is
// the working directory only when it is an explicit path to an existing
// directory; otherwise it is the target.
func runExplore(opts exploreOpts, positional []string, stdout, stderr io.Writer) error {
	if len(positional) > 0 {
		if d, ok := dirArg(positional[0]); ok {
			opts.dir = d
			positional = positional[1:]
		}
	}
	target := ""
	if len(positional) > 0 {
		target = strings.TrimSpace(positional[0])
	}

	cfg := inspect.Config{
		Loader: loader.Config{
			Dir:       opts.dir,
			Tests:     opts.tests,
			FilterPkg: opts.filterPkg,
		},
	}

	if isTTY() && !opts.formatExplicit {
		if err := tui.Run(target, cfg); err != nil {
			return &exitErr{code: 1, err: fmt.Errorf("tui error: %w", err)}
		}
		return nil
	}

	cfg.Loader.Progress = func(stage string) {
		fmt.Fprintf(stderr, "%s...\n", title(stage))
	}

	if target == "" {
		return &exitErr{code: 2, err: fmt.Errorf("a target package is required for non-TTY output (see gorefact --help)")}
	}

	res, err := inspect.ResolveTarget(target, cfg)
	if err != nil {
		return &exitErr{code: 1, err: fmt.Errorf("inspect failed: %w", err)}
	}

	fmtOpts := inspect.FormatOptions{ModuleOnly: opts.moduleOnly}

	switch strings.ToLower(strings.TrimSpace(opts.format)) {
	case "text":
		_, _ = io.WriteString(stdout, inspect.FormatText(res, fmtOpts))
	case "json":
		data, err := inspect.FormatJSONWithOptions(res, fmtOpts)
		if err != nil {
			return &exitErr{code: 1, err: fmt.Errorf("format json failed: %w", err)}
		}
		_, _ = stdout.Write(append(data, '\n'))
	case "md", "markdown":
		_, _ = io.WriteString(stdout, inspect.FormatMarkdownWithOptions(res, fmtOpts))
	default:
		return &exitErr{code: 2, err: fmt.Errorf("unknown format %q", opts.format)}
	}
	return nil
}

// isTTY returns true when stdout is an interactive terminal.
func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// dirArg reports whether s denotes an explicit filesystem path to an existing
// directory, returning the (tilde-expanded) path. Only path-like arguments
// ("." | ".." | "./x" | "../x" | "/abs" | "~" | "~/x") qualify, so bare
// import-path names such as "internal/loader" remain package targets even when
// a same-named directory exists in the working directory.
func dirArg(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	pathLike := s == "." || s == ".." ||
		strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, "/") || s == "~" || strings.HasPrefix(s, "~/")
	if !pathLike {
		return "", false
	}
	expanded := s
	if s == "~" || strings.HasPrefix(s, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = home + strings.TrimPrefix(s, "~")
		}
	}
	if fi, err := os.Stat(expanded); err == nil && fi.IsDir() {
		return expanded, true
	}
	return "", false
}
