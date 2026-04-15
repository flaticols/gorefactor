package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestRunCheck(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "tasks", "task.go"), "package tasks\n\nimport \"example.com/test/adapters\"\n\nfunc Run() {\n\tadapters.Do()\n}\n")
	writeFile(t, filepath.Join(dir, "adapters", "adapters.go"), "package adapters\n\nfunc Do() {}\n")
	writeFile(t, filepath.Join(dir, "rules.toml"), "[[deny]]\nfrom = \"tasks\"\nto = \"adapters\"\nreason = \"tasks must not depend on adapters\"\n")

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"check",
		"--rules", filepath.Join(dir, "rules.toml"),
		"--format", "text",
		"--dir", dir,
		"./...",
	}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout.String(), "VIOLATION tasks -> adapters") {
		t.Fatalf("stdout missing violation report: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Found 1 violations across 1 rules") {
		t.Fatalf("stderr missing summary: %s", stderr.String())
	}
}

func TestRunCheckUsesDefaultRulesFile(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "tasks", "task.go"), "package tasks\n\nimport \"example.com/test/adapters\"\n\nfunc Run() {\n\tadapters.Do()\n}\n")
	writeFile(t, filepath.Join(dir, "adapters", "adapters.go"), "package adapters\n\nfunc Do() {}\n")
	writeFile(t, filepath.Join(dir, defaultRulesFile), "[[deny]]\nfrom = \"tasks\"\nto = \"adapters\"\nreason = \"tasks must not depend on adapters\"\n")

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"check",
		"--format", "text",
		"--dir", dir,
		"./...",
	}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout.String(), "VIOLATION tasks -> adapters") {
		t.Fatalf("stdout missing violation report: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "at tasks/task.go:6:13") {
		t.Fatalf("stdout missing relative callsite: %s", stdout.String())
	}
}

func TestRunCheckJSONAndMarkdown(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "tasks", "task.go"), "package tasks\n\nimport \"example.com/test/adapters\"\n\nfunc Run() {\n\tadapters.Do()\n}\n")
	writeFile(t, filepath.Join(dir, "adapters", "adapters.go"), "package adapters\n\nfunc Do() {}\n")
	writeFile(t, filepath.Join(dir, defaultRulesFile), "[[deny]]\nfrom = \"tasks\"\nto = \"adapters\"\nreason = \"tasks must not depend on adapters\"\n")

	cases := []struct {
		format string
		want   []string
	}{
		{
			format: "json",
			want: []string{
				`"working_dir":`,
				`"callsite":`,
				`"module": "example.com/test"`,
				`"file": "tasks/task.go"`,
			},
		},
		{
			format: "md",
			want: []string{
				"# Dependency Violations Report",
				"| # | Callsite | Caller | Callee | Dynamic |",
				"`tasks/task.go:6:13`",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := run([]string{
				"check",
				"--format", tc.format,
				"--dir", dir,
				"./...",
			}, &stdout, &stderr)

			if exitCode != 1 {
				t.Fatalf("run() exitCode = %d, want 1, stderr=%s", exitCode, stderr.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing %q: %s", want, stdout.String())
				}
			}
		})
	}
}

func TestRunCheckWithFilterPkgScopesGraph(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "tasks", "task.go"), "package tasks\n\nimport \"example.com/test/adapters\"\n\nfunc Run() {\n\tadapters.Do()\n}\n")
	writeFile(t, filepath.Join(dir, "adapters", "adapters.go"), "package adapters\n\nfunc Do() {}\n")
	writeFile(t, filepath.Join(dir, "rules.toml"), "[[deny]]\nfrom = \"tasks\"\nto = \"adapters\"\nreason = \"tasks must not depend on adapters\"\n")

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"check",
		"--rules", filepath.Join(dir, "rules.toml"),
		"--format", "text",
		"--dir", dir,
		"--filter-pkg", "tasks",
		"./...",
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("run() exitCode = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "No dependency violations found.") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunValidateRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.toml")
	writeFile(t, path, "[[deny]]\nfrom = \"tasks\"\nto = \"adapters\"\nreason = \"tasks must not depend on adapters\"\n")

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"validate-rules", "--rules", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exitCode = %d, want 0, stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "validated 1 rules") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"version"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exitCode = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "gorefact "+expectedVersion()) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exitCode = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Commands:") || !strings.Contains(stdout.String(), "gorefact help check") {
		t.Fatalf("stdout missing root help details: %s", stdout.String())
	}
}

func TestRunHelpCheck(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"help", "check"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exitCode = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "gorefact check [flags] [packages...]") || !strings.Contains(stdout.String(), "Build the call graph") {
		t.Fatalf("stdout missing check help details: %s", stdout.String())
	}
}

func TestRunCheckHelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", "-h"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exitCode = %d, want 0", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Usage:") || !strings.Contains(stderr.String(), "Flags:") || !strings.Contains(stderr.String(), "-format") {
		t.Fatalf("stderr missing flag help: %s", stderr.String())
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"bogus"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("run() exitCode = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Fatalf("stderr missing usage: %s", stderr.String())
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func expectedVersion() string {
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
