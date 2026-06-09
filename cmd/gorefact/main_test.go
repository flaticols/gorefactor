package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

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
	for _, want := range []string{"Commands:", "version", "--module-only"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q: %s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "inspect") {
		t.Fatalf("root help should no longer mention the inspect subcommand: %s", stdout.String())
	}
}

func TestRunHelp_UnknownTopic(t *testing.T) {
	// Any unknown help topic could be a package target, so cobra resolves it
	// to the root command and prints root help with exit 0.
	var out, errOut strings.Builder
	code := run([]string{"help", "bogus"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "gorefact [dir] [target]") {
		t.Errorf("help bogus should print root help: %s", out.String())
	}
}

func TestRunInspect_HelpFlag(t *testing.T) {
	var out, errOut strings.Builder
	code := run([]string{"-h"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "gorefact [dir] [target]") {
		t.Errorf("help output missing usage line: %s", out.String())
	}
}

func TestRunInspect_NoTUI_Text(t *testing.T) {
	dir := t.TempDir()
	// Write a minimal two-package module so inspect has real data to walk.
	mustWriteFile(t, filepath.Join(dir, "go.mod"), "module example.com/testmod\n\ngo 1.26.2\n")
	mustWriteFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

type Widget struct{}
`)
	mustWriteFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/testmod/alpha"

func Make() alpha.Widget { return alpha.Widget{} }
`)

	var out, errOut strings.Builder
	code := run([]string{
		"--format", "text",
		"--dir", dir,
		"example.com/testmod/alpha",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, errOut.String())
	}
	for _, want := range []string{"example.com/testmod/alpha", "Public API:", "Importers:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q: %s", want, out.String())
		}
	}
}

// TestRunInspect_FlagsAfterTarget guards the goal form `gorefact <pkg> --format json`,
// where the flag follows the positional target.
func TestRunInspect_FlagsAfterTarget(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "go.mod"), "module example.com/testmod\n\ngo 1.26.2\n")
	mustWriteFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

type Widget struct{}
`)
	mustWriteFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/testmod/alpha"

func Make() alpha.Widget { return alpha.Widget{} }
`)

	var out, errOut strings.Builder
	// Flag after the target, and a partial-path target ("testmod/alpha").
	code := run([]string{
		"--dir", dir,
		"testmod/alpha",
		"--format", "json",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, errOut.String())
	}
	s := strings.TrimSpace(out.String())
	if !strings.HasPrefix(s, "{") {
		t.Fatalf("expected JSON output when --format json follows the target, got:\n%s", s)
	}
	if !strings.Contains(s, "example.com/testmod/alpha") {
		t.Errorf("partial-path target did not resolve to full package path: %s", s)
	}
}

// TestRunInspect_DirAsFirstArg guards the form `gorefact <dir> <target>`, where
// an explicit directory path is the first positional and the target follows.
func TestRunInspect_DirAsFirstArg(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "go.mod"), "module example.com/testmod\n\ngo 1.26.2\n")
	mustWriteFile(t, filepath.Join(dir, "alpha", "alpha.go"), "package alpha\n\ntype Widget struct{}\n")

	var out, errOut strings.Builder
	// Absolute dir path as first positional (starts with "/" -> treated as dir),
	// target second, --format makes it the non-TTY report path.
	code := run([]string{dir, "testmod/alpha", "--format", "json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, errOut.String())
	}
	s := strings.TrimSpace(out.String())
	if !strings.Contains(s, "example.com/testmod/alpha") {
		t.Fatalf("dir-as-first-arg did not resolve target within that dir: %s", s)
	}
}

// writeTestModule lays down a three-package module: beta imports alpha; alpha
// and beta are both importable; gamma imports nothing.
func writeTestModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "go.mod"), "module example.com/testmod\n\ngo 1.26.2\n")
	mustWriteFile(t, filepath.Join(dir, "alpha", "alpha.go"), "package alpha\n\ntype Widget struct{}\n")
	mustWriteFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/testmod/alpha"

func Make() alpha.Widget { return alpha.Widget{} }
`)
	mustWriteFile(t, filepath.Join(dir, "gamma", "gamma.go"), "package gamma\n\nconst N = 1\n")
	return dir
}

func TestRunPkg_List(t *testing.T) {
	dir := writeTestModule(t)
	var out, errOut strings.Builder
	code := run([]string{"pkg", "list", "--dir", dir}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, errOut.String())
	}
	for _, want := range []string{"example.com/testmod/alpha", "example.com/testmod/beta", "example.com/testmod/gamma"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("pkg list missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "fmt") {
		t.Errorf("module-only list should not contain stdlib packages:\n%s", out.String())
	}
}

func TestRunPkg_Importers(t *testing.T) {
	dir := writeTestModule(t)
	var out, errOut strings.Builder
	code := run([]string{"pkg", "importers", "alpha", "--dir", dir}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, errOut.String())
	}
	if strings.TrimSpace(out.String()) != "example.com/testmod/beta" {
		t.Errorf("importers of alpha = %q, want beta only", out.String())
	}
}

func TestRunPkg_ImportsJSON(t *testing.T) {
	dir := writeTestModule(t)
	var out, errOut strings.Builder
	code := run([]string{"pkg", "imports", "beta", "--dir", dir, "--format", "json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, errOut.String())
	}
	s := out.String()
	if !strings.Contains(s, `"pkgPath": "example.com/testmod/beta"`) ||
		!strings.Contains(s, "example.com/testmod/alpha") {
		t.Errorf("pkg imports json mismatch:\n%s", s)
	}
}

func TestRunPkg_Get(t *testing.T) {
	dir := writeTestModule(t)
	var out, errOut strings.Builder
	code := run([]string{"pkg", "get", "alpha", "--dir", dir, "--format", "json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, errOut.String())
	}
	for _, want := range []string{`"pkgPath"`, `"publicApi"`, `"importers"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("pkg get json missing %s:\n%s", want, out.String())
		}
	}
}

func TestRunPkg_Errors(t *testing.T) {
	dir := writeTestModule(t)
	var out, errOut strings.Builder
	if code := run([]string{"pkg"}, &out, &errOut); code != 2 {
		t.Errorf("bare pkg should exit 2, got %d", code)
	}
	errOut.Reset()
	if code := run([]string{"pkg", "bogus"}, &out, &errOut); code != 2 {
		t.Errorf("unknown subcommand should exit 2, got %d", code)
	}
	errOut.Reset()
	if code := run([]string{"pkg", "importers", "--dir", dir}, &out, &errOut); code != 2 {
		t.Errorf("importers without target should exit 2, got %d", code)
	}
	errOut.Reset()
	if code := run([]string{"pkg", "importers", "nosuchpkg", "--dir", dir}, &out, &errOut); code != 1 {
		t.Errorf("unknown package should exit 1, got %d", code)
	}
}

func TestDirArg(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		in     string
		wantOK bool
	}{
		{dir, true},                      // absolute existing dir
		{".", true},                      // cwd
		{"internal/loader", false},       // bare import-path name stays a target
		{"github.com/acme/tasks", false}, // bare package path stays a target
		{"./does-not-exist-xyz", false},  // path-like but missing
		{"", false},                      // empty
	}
	for _, c := range cases {
		if _, ok := dirArg(c.in); ok != c.wantOK {
			t.Errorf("dirArg(%q) ok = %v, want %v", c.in, ok, c.wantOK)
		}
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
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
