package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/rpc"
	"go.flaticols.dev/gorefactor/internal/rules"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "version":
		return runVersion(stdout)
	case "validate-rules":
		return runValidateRules(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		rulesPath = fs.String("rules", "", "path to rules.toml")
		format    = fs.String("format", "text", "output format: text|json|md|qf")
		dir       = fs.String("dir", ".", "working directory")
		tests     = fs.Bool("tests", false, "include tests")
		filterPkg = fs.String("filter-pkg", "", "only include packages containing this path fragment")
	)

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*rulesPath) == "" {
		fmt.Fprintln(stderr, "--rules is required")
		return 2
	}

	progress := func(stage string) {
		fmt.Fprintf(stderr, "%s...\n", title(stage))
	}

	g, err := graph.Build(graph.BuildConfig{
		Dir:       *dir,
		Tests:     *tests,
		FilterPkg: *filterPkg,
		Patterns:  fs.Args(),
		Progress:  progress,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build failed: %v\n", err)
		return 1
	}

	ruleSet, err := rules.Parse(*rulesPath)
	if err != nil {
		fmt.Fprintf(stderr, "parse rules failed: %v\n", err)
		return 1
	}

	violations := rules.Check(g, ruleSet)

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "text":
		_, _ = io.WriteString(stdout, rules.FormatText(violations))
	case "json":
		data, err := rules.FormatJSON(violations, len(ruleSet))
		if err != nil {
			fmt.Fprintf(stderr, "format json failed: %v\n", err)
			return 1
		}
		_, _ = stdout.Write(append(data, '\n'))
	case "md", "markdown":
		_, _ = io.WriteString(stdout, rules.FormatMarkdown(violations, len(ruleSet)))
	case "qf", "quickfix":
		_, _ = io.WriteString(stdout, rules.FormatQuickfix(violations))
	default:
		fmt.Fprintf(stderr, "unknown format %q\n", *format)
		return 2
	}

	fmt.Fprintf(stderr, "Found %d violations across %d rules\n", len(violations), len(ruleSet))
	if len(violations) > 0 {
		return 1
	}
	return 0
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		rulesPath = fs.String("rules", "", "path to rules.toml")
		dir       = fs.String("dir", ".", "working directory")
		tests     = fs.Bool("tests", false, "include tests")
		filterPkg = fs.String("filter-pkg", "", "only include packages containing this path fragment")
	)

	if err := fs.Parse(args); err != nil {
		return 2
	}
	progress := func(stage string) {
		_ = writeNotification(stdout, "gorefact.progress", map[string]string{"stage": title(stage)})
	}

	g, err := graph.Build(graph.BuildConfig{
		Dir:       *dir,
		Tests:     *tests,
		FilterPkg: *filterPkg,
		Patterns:  fs.Args(),
		Progress:  progress,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build failed: %v\n", err)
		return 1
	}

	var ruleSet []rules.Rule
	if strings.TrimSpace(*rulesPath) != "" {
		ruleSet, err = rules.Parse(*rulesPath)
		if err != nil {
			fmt.Fprintf(stderr, "parse rules failed: %v\n", err)
			return 1
		}
	}

	if err := writeNotification(stdout, "gorefact.ready", map[string]any{
		"rules": len(ruleSet),
	}); err != nil {
		fmt.Fprintf(stderr, "ready notification failed: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	srv := rpc.NewServer(g, ruleSet, stdout)
	if err := srv.Serve(ctx, os.Stdin); err != nil && err != context.Canceled {
		fmt.Fprintf(stderr, "rpc server failed: %v\n", err)
		return 1
	}
	return 0
}

func runVersion(stdout io.Writer) int {
	_, _ = fmt.Fprintf(stdout, "gorefact %s\n", version)
	return 0
}

func runValidateRules(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate-rules", flag.ContinueOnError)
	fs.SetOutput(stderr)

	rulesPath := fs.String("rules", "", "path to rules.toml")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*rulesPath) == "" {
		fmt.Fprintln(stderr, "--rules is required")
		return 2
	}
	ruleSet, err := rules.Parse(*rulesPath)
	if err != nil {
		fmt.Fprintf(stderr, "parse rules failed: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "validated %d rules\n", len(ruleSet))
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: gorefact <check|serve|version|validate-rules> [flags] [packages...]")
	fmt.Fprintln(w, "  check  run dependency violation checks")
	fmt.Fprintln(w, "  serve  start the JSON-RPC server")
	fmt.Fprintln(w, "  version  print the gorefact version")
	fmt.Fprintln(w, "  validate-rules  parse and validate a rules file")
}

func title(stage string) string {
	stage = strings.TrimSpace(strings.ToLower(stage))
	switch stage {
	case "loading packages":
		return "Loading packages"
	case "building ssa":
		return "Building SSA"
	case "building call graph":
		return "Building call graph"
	case "done":
		return "Done"
	default:
		return filepath.Clean(stage)
	}
}

func writeNotification(w io.Writer, method string, params any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(rpc.Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}
