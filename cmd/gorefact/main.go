package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// Version is injected at build time via -ldflags "-X main.Version=x.y.z".
// Falls back to debug.ReadBuildInfo (works for `go install` from a tagged release).
var Version string

// exitErr carries an explicit process exit code out of a cobra RunE. The
// wrapped error (if any) is printed to stderr by run().
type exitErr struct {
	code int
	err  error
}

func (e *exitErr) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return ""
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err == nil {
		return 0
	}
	if ee, ok := errors.AsType[*exitErr](err); ok {
		if ee.err != nil {
			fmt.Fprintln(stderr, ee.err)
		}
		return ee.code
	}
	// Cobra usage errors (unknown command/flag, bad arg count).
	fmt.Fprintln(stderr, err)
	return 2
}

func newRootCmd() *cobra.Command {
	var opts exploreOpts
	root := &cobra.Command{
		Use:   "gorefact [dir] [target]",
		Short: "Package-centric explorer for Go import and reference graphs",
		Long: `gorefact is a package-centric explorer for Go import and reference graphs.

With no flags on a terminal it opens the interactive explorer. With a target
and --format (or a non-TTY stdout) it prints a package report: public API,
importers, and a reference summary.

The first positional is the working directory when it is an explicit path to
an existing directory (".", "./x", "/abs", "~/x"); otherwise it is the target.

Target formats:
  github.com/acme/tasks              entire package
  tasks                              bare suffix, matched against module packages
  github.com/acme/tasks.Run          symbol named Run
  github.com/acme/tasks.Engine.Calc  method Calc on type Engine`,
		Example: `  gorefact
  gorefact ~/code/myproj
  gorefact ./sub internal/loader
  gorefact --format json github.com/acme/tasks | jq .
  gorefact pkg importers internal/loader`,
		Args:          cobra.MaximumNArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.formatExplicit = cmd.Flags().Changed("format")
			return runExplore(opts, args, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	fl := root.Flags()
	fl.StringVar(&opts.dir, "dir", ".", "working directory")
	fl.StringVar(&opts.format, "format", "text", "output format: text|json|md (non-TTY only)")
	fl.BoolVar(&opts.tests, "tests", false, "include test packages")
	fl.StringVar(&opts.filterPkg, "filter-pkg", "", "only include packages containing this path fragment")
	fl.BoolVar(&opts.moduleOnly, "module-only", true, "restrict importers and references to the main module")

	root.AddCommand(newPkgCmd(), newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the gorefact version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "gorefact %s\n", version())
		},
	}
}

func title(stage string) string {
	stage = strings.TrimSpace(strings.ToLower(stage))
	switch stage {
	case "loading packages":
		return "Loading packages"
	case "done":
		return "Done"
	case "loading package graph":
		return "Loading package graph"
	case "walking references":
		return "Walking references"
	default:
		return filepath.Clean(stage)
	}
}

func version() string {
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
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
