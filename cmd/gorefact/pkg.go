package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
)

// pkgOpts holds the flags shared by the `pkg` subcommands.
type pkgOpts struct {
	dir        string
	format     string
	tests      bool
	moduleOnly bool
}

func (o pkgOpts) config() inspect.Config {
	return inspect.Config{Loader: loader.Config{Dir: o.dir, Tests: o.tests}}
}

func newPkgCmd() *cobra.Command {
	var opts pkgOpts
	pkg := &cobra.Command{
		Use:   "pkg",
		Short: "Query the package graph without the TUI",
		Long: `Query the package graph without the TUI. Always prints a report.

Targets accept full import paths or bare suffixes (e.g. "loader").`,
		Example: `  gorefact pkg list
  gorefact pkg importers internal/loader
  gorefact pkg imports tui --format json
  gorefact pkg get internal/loader --format md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return &exitErr{code: 2, err: fmt.Errorf("unknown pkg subcommand %q (see gorefact pkg --help)", args[0])}
			}
			_ = cmd.Help()
			return &exitErr{code: 2}
		},
	}

	pf := pkg.PersistentFlags()
	pf.StringVar(&opts.dir, "dir", ".", "working directory")
	pf.StringVar(&opts.format, "format", "text", "output format: text|json (get also: md)")
	pf.BoolVar(&opts.tests, "tests", false, "include test packages")
	pf.BoolVar(&opts.moduleOnly, "module-only", true, "restrict output to the main module")

	list := &cobra.Command{
		Use:   "list",
		Short: "List packages in the module",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPkgList(opts, cmd.OutOrStdout())
		},
	}
	get := &cobra.Command{
		Use:   "get <pkg>",
		Short: "Full report: public API + importers + references",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPkgGet(opts, args[0], cmd.OutOrStdout())
		},
	}
	imports := &cobra.Command{
		Use:   "imports <pkg>",
		Short: "Packages that <pkg> imports",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPkgEdges(opts, "imports", args[0], cmd.OutOrStdout())
		},
	}
	importers := &cobra.Command{
		Use:   "importers <pkg>",
		Short: "Packages that import <pkg>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPkgEdges(opts, "importers", args[0], cmd.OutOrStdout())
		},
	}

	pkg.AddCommand(list, get, imports, importers)
	return pkg
}

// checkPkgFormat validates --format for the pkg subcommands.
func checkPkgFormat(format string, allowMD bool) (string, error) {
	f := strings.ToLower(strings.TrimSpace(format))
	switch {
	case f == "text" || f == "json":
		return f, nil
	case allowMD && (f == "md" || f == "markdown"):
		return f, nil
	default:
		return "", &exitErr{code: 2, err: fmt.Errorf("unknown format %q", format)}
	}
}

func runPkgList(opts pkgOpts, stdout io.Writer) error {
	f, err := checkPkgFormat(opts.format, false)
	if err != nil {
		return err
	}
	pg, gerr := inspect.BuildGraph(opts.config())
	if gerr != nil {
		return &exitErr{code: 1, err: fmt.Errorf("pkg list failed: %w", gerr)}
	}
	var paths []string
	for _, p := range pg.AllPaths() {
		if inModule(pg, p, opts.moduleOnly) {
			paths = append(paths, p)
		}
	}
	return writePkgResult(stdout, f, paths, map[string]any{
		"module":   pg.Module,
		"packages": paths,
	})
}

func runPkgGet(opts pkgOpts, target string, stdout io.Writer) error {
	f, err := checkPkgFormat(opts.format, true)
	if err != nil {
		return err
	}
	res, rerr := inspect.ResolveTarget(target, opts.config())
	if rerr != nil {
		return &exitErr{code: 1, err: fmt.Errorf("pkg get failed: %w", rerr)}
	}
	fmtOpts := inspect.FormatOptions{ModuleOnly: opts.moduleOnly}
	switch f {
	case "text":
		_, _ = io.WriteString(stdout, inspect.FormatText(res, fmtOpts))
	case "json":
		data, jerr := inspect.FormatJSONWithOptions(res, fmtOpts)
		if jerr != nil {
			return &exitErr{code: 1, err: fmt.Errorf("format json failed: %w", jerr)}
		}
		_, _ = stdout.Write(append(data, '\n'))
	case "md", "markdown":
		_, _ = io.WriteString(stdout, inspect.FormatMarkdownWithOptions(res, fmtOpts))
	}
	return nil
}

// runPkgEdges handles `pkg imports` and `pkg importers` (kind selects which).
func runPkgEdges(opts pkgOpts, kind, target string, stdout io.Writer) error {
	f, err := checkPkgFormat(opts.format, false)
	if err != nil {
		return err
	}
	pg, gerr := inspect.BuildGraph(opts.config())
	if gerr != nil {
		return &exitErr{code: 1, err: fmt.Errorf("pkg %s failed: %w", kind, gerr)}
	}
	pkgPath := inspect.ResolvePackagePath(pg, target)
	if pkgPath == "" {
		return &exitErr{code: 1, err: fmt.Errorf("package %q not found in %s", target, opts.dir)}
	}
	var paths []string
	if kind == "imports" {
		for _, p := range pg.Nodes[pkgPath].Imports {
			if inModule(pg, p, opts.moduleOnly) {
				paths = append(paths, p)
			}
		}
	} else {
		for _, n := range pg.ImportersOf(pkgPath) {
			if inModule(pg, n.Path, opts.moduleOnly) {
				paths = append(paths, n.Path)
			}
		}
	}
	return writePkgResult(stdout, f, paths, map[string]any{
		"pkgPath": pkgPath,
		kind:      paths,
	})
}

// inModule reports whether p belongs to the main module (always true when
// moduleOnly is off or the module is unknown).
func inModule(pg *loader.PackageGraph, p string, moduleOnly bool) bool {
	if !moduleOnly || pg.Module == "" {
		return true
	}
	return p == pg.Module || strings.HasPrefix(p, pg.Module+"/")
}

// writePkgResult prints one path per line (text) or the given object (json).
func writePkgResult(stdout io.Writer, format string, lines []string, obj map[string]any) error {
	if format == "json" {
		data, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return &exitErr{code: 1, err: fmt.Errorf("format json failed: %w", err)}
		}
		_, _ = stdout.Write(append(data, '\n'))
		return nil
	}
	for _, l := range lines {
		fmt.Fprintln(stdout, l)
	}
	return nil
}
