package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
	"go.flaticols.dev/gorefactor/internal/rules"
	"go.flaticols.dev/gorefactor/internal/tui"
)

func runInspect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		printInspectHelp(fs.Output())
		printFlagDefaults(fs)
	}

	var (
		rulesPath = fs.String("rules", defaultRulesFile, "path to gorefact.rules.toml")
		dir       = fs.String("dir", ".", "working directory")
		format    = fs.String("format", "text", "output format: text|json|md|qf (non-TTY only)")
		tests     = fs.Bool("tests", false, "include test packages")
		filterPkg = fs.String("filter-pkg", "", "only include packages containing this path fragment")
		noTUI     = fs.Bool("no-tui", false, "force CLI output even on TTY")
	)

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	target := ""
	if len(fs.Args()) > 0 {
		target = strings.TrimSpace(fs.Args()[0])
	}

	resolvedRulesPath := resolvePath(*dir, *rulesPath)
	var ruleSet []rules.Rule
	if resolvedRulesPath != "" {
		var err error
		ruleSet, err = rules.Parse(resolvedRulesPath)
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "parse rules failed: %v\n", err)
			return 1
		}
	}

	cfg := inspect.Config{
		Loader: loader.Config{
			Dir:       *dir,
			Tests:     *tests,
			FilterPkg: *filterPkg,
		},
		Rules: ruleSet,
	}

	if !*noTUI && isTTY() {
		if err := tui.Run(target, cfg); err != nil {
			fmt.Fprintf(stderr, "tui error: %v\n", err)
			return 1
		}
		return 0
	}

	cfg.Loader.Progress = func(stage string) {
		fmt.Fprintf(stderr, "%s...\n", title(stage))
	}

	res, err := inspect.ResolveTarget(target, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "inspect failed: %v\n", err)
		return 1
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "text":
		_, _ = io.WriteString(stdout, inspect.FormatText(res, inspect.FormatOptions{}))
	case "json":
		data, err := inspect.FormatJSON(res)
		if err != nil {
			fmt.Fprintf(stderr, "format json failed: %v\n", err)
			return 1
		}
		_, _ = stdout.Write(append(data, '\n'))
	case "md", "markdown":
		_, _ = io.WriteString(stdout, inspect.FormatMarkdown(res))
	case "qf", "quickfix":
		_, _ = io.WriteString(stdout, inspect.FormatQuickfix(res))
	default:
		fmt.Fprintf(stderr, "unknown format %q\n", *format)
		return 2
	}
	return 0
}

// isTTY returns true when stdout is an interactive terminal.
func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func printInspectHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gorefact inspect [target] [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Show everything that imports or references a given package, type, function,")
	fmt.Fprintln(w, "const, or var — full reference tree using T1+T2 engine.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Target formats:")
	fmt.Fprintln(w, "  github.com/acme/tasks              — entire package")
	fmt.Fprintln(w, "  github.com/acme/tasks.Run          — symbol named Run")
	fmt.Fprintln(w, "  github.com/acme/tasks.Engine.Calc  — method Calc on type Engine")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Without a target on a TTY, opens the TUI with an empty search box.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gorefact inspect github.com/acme/tasks")
	fmt.Fprintln(w, "  gorefact inspect github.com/acme/tasks.Engine --format json")
	fmt.Fprintln(w, "  gorefact inspect github.com/acme/tasks --no-tui --format text")
	fmt.Fprintln(w)
}
