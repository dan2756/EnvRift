package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setup writes files into a fresh temp dir and makes it the working
// directory, so reports mention short relative names.
func setup(t *testing.T, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
}

func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunExitCodes(t *testing.T) {
	setup(t, map[string]string{
		".env":         "A=1\nB=secret-value-1234\n",
		".env.example": "A=\nB=changeme\n",
		".env.short":   "A=1\n",
		"bad.env":      "A=1\nB\n",
	})
	tests := []struct {
		name string
		args []string
		code int
		out  string // substring of stdout
		err  string // substring of stderr
	}{
		{"keys match", []string{".env", ".env.example"}, 0, ".env and .env.example match (2 keys)", ""},
		{"missing key", []string{".env", ".env.short"}, 1, "- B  missing in .env.short", ""},
		{"extra key", []string{".env.short", ".env"}, 1, "+ B  only in .env", ""},
		{"values differ", []string{"--values", ".env", ".env.example"}, 1, "~ A  differs", ""},
		{"same file values", []string{"--values", ".env", ".env"}, 0, "match", ""},
		{"unreadable file", []string{".env", "nope.env"}, 2, "", "nope.env"},
		{"parse error", []string{".env", "bad.env"}, 2, "", "bad.env:2:"},
		{"one file", []string{".env"}, 2, "", "expected two files, got 1"},
		{"unknown flag", []string{"--nope", ".env", ".env"}, 2, "", "flag provided but not defined"},
		{"help", []string{"--help"}, 0, "Usage: envdiff", ""},
		{"short help", []string{"-h"}, 0, "Usage: envdiff", ""},
		{"version", []string{"--version"}, 0, "envdiff ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runCLI(tt.args...)
			if code != tt.code {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.code, out, errOut)
			}
			if !strings.Contains(out, tt.out) {
				t.Errorf("stdout = %q, want it to contain %q", out, tt.out)
			}
			if !strings.Contains(errOut, tt.err) {
				t.Errorf("stderr = %q, want it to contain %q", errOut, tt.err)
			}
		})
	}
}

func TestRunMasksValues(t *testing.T) {
	setup(t, map[string]string{
		"a.env": "TOKEN=abcdefghijklmnopqrstuvwxyz\nPIN=1234\n",
		"b.env": "TOKEN=abcdefghijklmnopqrstuvwxyZ\nPIN=4321\n",
	})
	_, out, _ := runCLI("--values", "a.env", "b.env")
	for _, want := range []string{"a.env=ab****yz", "b.env=ab****yZ", "a.env=<redacted>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "1234") || strings.Contains(out, "cdefgh") {
		t.Errorf("output leaks a secret:\n%s", out)
	}

	_, out, _ = runCLI("--show-values", "a.env", "b.env")
	if !strings.Contains(out, `a.env="1234" b.env="4321"`) {
		t.Errorf("--show-values output = %q", out)
	}
}

func TestRunFilters(t *testing.T) {
	setup(t, map[string]string{
		".env":         "PORT=3000\nDATABASE_URL=x\nNEXT_PUBLIC_API=y\nNEXT_PUBLIC_SITE=z\n",
		".env.example": "DATABASE_URL=\nLEGACY=1\n",
	})
	tests := []struct {
		args []string
		code int
	}{
		{[]string{".env", ".env.example"}, 1},
		{[]string{"--ignore", "PORT", "--ignore-prefix", "NEXT_PUBLIC_", ".env", ".env.example"}, 1},
		{[]string{"--ignore", "PORT,LEGACY", "--ignore-prefix", "NEXT_PUBLIC_", ".env", ".env.example"}, 0},
		{[]string{"--ignore=PORT", "--ignore=LEGACY", "--ignore-prefix=NEXT_PUBLIC_API,NEXT_PUBLIC_SITE", ".env", ".env.example"}, 0},
		{[]string{"--only-missing", "--ignore", "PORT", "--ignore-prefix", "NEXT_PUBLIC_", ".env", ".env.example"}, 0},
		{[]string{"--only-missing", ".env.example", ".env"}, 1},
	}
	for _, tt := range tests {
		code, out, errOut := runCLI(tt.args...)
		if code != tt.code {
			t.Errorf("envdiff %s: exit %d, want %d\n%s%s", strings.Join(tt.args, " "), code, tt.code, out, errOut)
		}
	}
}

func TestMask(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"x", "<redacted>"},
		{"hunter2", "<redacted>"},
		{"elevenchars", "<redacted>"},
		{"twelve-chars", "tw****rs"},
		{"postgres://user:pw@db:5432/app", "po****pp"},
	}
	for _, tt := range tests {
		if got := mask(tt.in); got != tt.want {
			t.Errorf("mask(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
