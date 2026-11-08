// verinc_test.go - regression tests for verinc.
// Run with: go test ./...
package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBumpVersion covers patch / minor / major bumps and the new --nowrap rule.
func TestBumpVersion(t *testing.T) {
	cases := []struct {
		name                 string
		in, want             string
		minor, major, nowrap bool
	}{
		{"patch", "3.19.6", "3.19.7", false, false, false},
		{"patch-wrap", "3.20.9", "3.21.1", false, false, false},
		{"patch-nowrap", "3.20.9", "3.20.10", false, false, true},
		{"patch-nowrap-1.7.9", "1.7.9", "1.7.10", false, false, true},
		{"minor", "3.19.6", "3.20.1", true, false, false},
		{"minor-nowrap-ignored", "3.19.6", "3.20.1", true, false, true},
		{"major", "3.19.6", "4.0.1", false, true, false},
		{"major-with-nowrap", "3.19.6", "4.0.1", false, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := fnBumpVersion(c.in, c.minor, c.major, c.nowrap)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	} // end:for
}

// TestProcessLine exercises detection and replacement incl. CRLF preservation.
func TestProcessLine(t *testing.T) {
	cfg := &tConfig{}
	cases := []struct {
		name    string
		in      string
		want    string
		changed bool
	}{
		{"version-token", "const _VERSION = \"1.2.3\"\n", "const _VERSION = \"1.2.4\"\n", true},
		{"no-token", "// just a comment 1.2.3\n", "// just a comment 1.2.3\n", false},
		{"product-version", "\"ProductVersion\": \"1.2.3\"\n", "\"ProductVersion\": \"1.2.4\"\n", true},
		{"crlf", "const _VERSION = \"1.2.3\"\r\n", "const _VERSION = \"1.2.4\"\r\n", true},
		{"no-trailing-newline", "const _VERSION = \"1.2.3\"", "const _VERSION = \"1.2.4\"", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			changed := false
			got, err := fnProcessLine("test", c.in, cfg, &changed)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
			if changed != c.changed {
				t.Errorf("changed=%v want %v", changed, c.changed)
			}
		})
	} // end:for
}

// TestProcessFilePreservesCRLF verifies that CRLF files stay CRLF.
func TestProcessFilePreservesCRLF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "crlf.txt")
	content := "const _VERSION = \"1.2.3\"\r\n// other line\r\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &tConfig{}
	changed, code, err := fnProcessFile(p, cfg)
	if err != nil || !changed || code != iExitOK {
		t.Fatalf("changed=%v code=%d err=%v", changed, code, err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	want := "const _VERSION = \"1.2.4\"\r\n// other line\r\n"
	if string(got) != want {
		t.Errorf("got %q want %q", string(got), want)
	}
} // end:TestProcessFilePreservesCRLF

// TestProcessFilePreservesPermissions ensures we do not lose the exec bit.
func TestProcessFilePreservesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "file.sh")
	if err := os.WriteFile(p, []byte("const _VERSION = \"1.2.3\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &tConfig{}
	if _, _, err := fnProcessFile(p, cfg); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("perm got %v want 0755", fi.Mode().Perm())
	}
} // end:TestProcessFilePreservesPermissions

// TestGetVersion covers the --get code path.
func TestGetVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(p, []byte("// _VERSION = \"9.8.7\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := fnGetVersion(p)
	if err != nil {
		t.Fatal(err)
	}
	if v != "9.8.7" {
		t.Errorf("got %q", v)
	}
} // end:TestGetVersion

// TestParseArgsHelp verifies -h/--help does not become "invalid args".
func TestParseArgsHelp(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"verinc", "-h"}
	cfg, err := fnParseArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.bShowHelp {
		t.Errorf("expected bShowHelp")
	}
} // end:TestParseArgsHelp

// TestParseArgsNoFiles verifies the sentinel error path (exit code 3).
func TestParseArgsNoFiles(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"verinc"}
	_, err := fnParseArgs()
	if !errors.Is(err, errNoFiles) {
		t.Errorf("expected errNoFiles, got %v", err)
	}
} // end:TestParseArgsNoFiles
