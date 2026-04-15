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
	"runtime/debug"
	"strings"
	"syscall"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/rpc"
	"go.flaticols.dev/gorefactor/internal/rules"
)

const defaultRulesFile = "gorefact.rules.toml"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRootHelp(stderr)
		return 2
	}

	switch args[0] {
	case "help", "-h", "--help":
		return runHelp(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "validate-rules":
		return runValidateRules(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n\n", args[0])
		printRootHelp(stderr)
		return 2
	}
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		printCheckHelp(fs.Output())
		printFlagDefaults(fs)
	}

	var (
		rulesPath = fs.String("rules", defaultRulesFile, "path to gorefact.rules.toml")
		format    = fs.String("format", "text", "output format: text|json|md|qf")
		dir       = fs.String("dir", ".", "working directory")
		tests     = fs.Bool("tests", false, "include tests")
		filterPkg = fs.String("filter-pkg", "", "only include packages containing this path fragment")
	)

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	resolvedRulesPath := resolvePath(*dir, *rulesPath)
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

	ruleSet, err := rules.Parse(resolvedRulesPath)
	if err != nil {
		fmt.Fprintf(stderr, "parse rules failed: %v\n", err)
		return 1
	}

	violations := rules.Check(g, ruleSet)
	formatOpts := rules.FormatOptions{BaseDir: *dir}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "text":
		_, _ = io.WriteString(stdout, rules.FormatText(violations, formatOpts))
	case "json":
		data, err := rules.FormatJSON(violations, len(ruleSet), formatOpts)
		if err != nil {
			fmt.Fprintf(stderr, "format json failed: %v\n", err)
			return 1
		}
		_, _ = stdout.Write(append(data, '\n'))
	case "md", "markdown":
		_, _ = io.WriteString(stdout, rules.FormatMarkdown(violations, len(ruleSet), formatOpts))
	case "qf", "quickfix":
		_, _ = io.WriteString(stdout, rules.FormatQuickfix(violations, formatOpts))
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
	fs.Usage = func() {
		printServeHelp(fs.Output())
		printFlagDefaults(fs)
	}

	var (
		rulesPath = fs.String("rules", defaultRulesFile, "path to gorefact.rules.toml")
		dir       = fs.String("dir", ".", "working directory")
		tests     = fs.Bool("tests", false, "include tests")
		filterPkg = fs.String("filter-pkg", "", "only include packages containing this path fragment")
	)

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	resolvedRulesPath := resolvePath(*dir, *rulesPath)
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
	if strings.TrimSpace(resolvedRulesPath) != "" {
		ruleSet, err = rules.Parse(resolvedRulesPath)
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

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch strings.TrimSpace(args[0]) {
		case "-h", "--help", "help":
			printVersionHelp(stdout)
			return 0
		default:
			fmt.Fprintf(stderr, "version does not accept arguments: %s\n\n", strings.Join(args, " "))
			printVersionHelp(stderr)
			return 2
		}
	}
	_, _ = fmt.Fprintf(stdout, "gorefact %s\n", version())
	return 0
}

func runValidateRules(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate-rules", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		printValidateRulesHelp(fs.Output())
		printFlagDefaults(fs)
	}

	rulesPath := fs.String("rules", defaultRulesFile, "path to gorefact.rules.toml")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
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

func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRootHelp(stdout)
		return 0
	}
	if len(args) > 1 {
		fmt.Fprintf(stderr, "help accepts at most one topic\n\n")
		printRootHelp(stderr)
		return 2
	}
	switch strings.TrimSpace(args[0]) {
	case "check":
		printCheckHelp(stdout)
		return 0
	case "serve":
		printServeHelp(stdout)
		return 0
	case "version":
		printVersionHelp(stdout)
		return 0
	case "validate-rules":
		printValidateRulesHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown help topic %q\n\n", args[0])
		printRootHelp(stderr)
		return 2
	}
}

func printRootHelp(w io.Writer) {
	fmt.Fprintln(w, "gorefact inspects Go package dependencies and reports architectural violations.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gorefact <command> [flags] [packages...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  check           build the graph and report rule violations")
	fmt.Fprintln(w, "  serve           start the JSON-RPC server for the Neovim plugin")
	fmt.Fprintln(w, "  validate-rules  parse and validate a rules file without loading packages")
	fmt.Fprintln(w, "  version         print the gorefact version")
	fmt.Fprintln(w, "  help            show general or command-specific help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gorefact check ./...")
	fmt.Fprintln(w, "  gorefact check --format json --dir . ./...")
	fmt.Fprintln(w, "  gorefact serve --rules gorefact.rules.toml ./...")
	fmt.Fprintln(w, "  gorefact help check")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `gorefact help <command>` or `gorefact <command> -h` for details.")
}

func printCheckHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gorefact check [flags] [packages...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Build the call graph for the target packages, evaluate dependency rules, and")
	fmt.Fprintln(w, "print violations in text, JSON, Markdown, or quickfix format.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "If no package pattern is provided, gorefact defaults to `./...`.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gorefact check ./...")
	fmt.Fprintln(w, "  gorefact check --rules gorefact.rules.toml --format json ./...")
	fmt.Fprintln(w, "  gorefact check --dir /path/to/repo --filter-pkg tasks ./...")
	fmt.Fprintln(w)
}

func printServeHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gorefact serve [flags] [packages...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Build the call graph once, load rules, then serve newline-delimited JSON-RPC")
	fmt.Fprintln(w, "requests on stdin/stdout for the Neovim plugin.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "If no package pattern is provided, gorefact defaults to `./...`.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gorefact serve ./...")
	fmt.Fprintln(w, "  gorefact serve --dir /path/to/repo --rules gorefact.rules.toml ./...")
	fmt.Fprintln(w)
}

func printVersionHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gorefact version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Print the gorefact binary version.")
}

func printValidateRulesHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gorefact validate-rules [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Parse the TOML rules file and report whether it is valid without loading any")
	fmt.Fprintln(w, "Go packages.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gorefact validate-rules")
	fmt.Fprintln(w, "  gorefact validate-rules --rules gorefact.rules.toml")
	fmt.Fprintln(w)
}

func printFlagDefaults(fs *flag.FlagSet) {
	if fs == nil {
		return
	}
	fmt.Fprintln(fs.Output(), "Flags:")
	fs.PrintDefaults()
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

func resolvePath(baseDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return "(unknown)"
	}
	if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
		return v
	}
	if rev := buildSetting(info, "vcs.revision"); rev != "" {
		rev = shortRevision(rev)
		if buildSetting(info, "vcs.modified") == "true" {
			return rev + "-dirty"
		}
		return rev
	}
	if strings.TrimSpace(info.Main.Version) != "" {
		return info.Main.Version
	}
	return "(devel)"
}

func buildSetting(info *debug.BuildInfo, key string) string {
	if info == nil {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == key {
			return strings.TrimSpace(setting.Value)
		}
	}
	return ""
}

func shortRevision(rev string) string {
	rev = strings.TrimSpace(rev)
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

func writeNotification(w io.Writer, method string, params any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(rpc.Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}
