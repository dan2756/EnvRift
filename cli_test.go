package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
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
		{"one file", []string{".env"}, 2, "", "need at least two files to compare, got 1"},
		{"no files", []string{}, 2, "", "got 0"},
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

func TestRunQuiet(t *testing.T) {
	setup(t, map[string]string{"a.env": "A=1\n", "b.env": "B=1\n", "bad.env": "nope\n"})
	for _, flag := range []string{"-q", "--quiet"} {
		code, out, _ := runCLI(flag, "a.env", "b.env")
		if code != 1 || out != "" {
			t.Errorf("%s: exit %d, stdout %q; want 1 and no output", flag, code, out)
		}
		code, out, _ = runCLI(flag, "a.env", "a.env")
		if code != 0 || out != "" {
			t.Errorf("%s on equal files: exit %d, stdout %q; want 0 and no output", flag, code, out)
		}
	}
	// Errors still go to stderr so a failing CI job explains itself.
	code, _, errOut := runCLI("-q", "a.env", "bad.env")
	if code != 2 || !strings.Contains(errOut, "bad.env:1:") {
		t.Errorf("quiet parse error: exit %d, stderr %q", code, errOut)
	}
}

func TestRunJSON(t *testing.T) {
	setup(t, map[string]string{
		".env":         "A=1\nB=a-long-secret-value\nC=3\n",
		".env.example": "B=another-long-value\nC=3\nD=4\n",
	})
	code, out, errOut := runCLI("--json", "--values", ".env", ".env.example")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (stderr %q)", code, errOut)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if rep.Match || rep.Reference != ".env" || len(rep.Comparisons) != 1 {
		t.Fatalf("unexpected report: %+v", rep)
	}
	c := rep.Comparisons[0]
	if c.File != ".env.example" || c.Match {
		t.Errorf("comparison = %+v", c)
	}
	if len(c.Missing) != 1 || c.Missing[0] != "A" || len(c.Extra) != 1 || c.Extra[0] != "D" {
		t.Errorf("missing = %q, extra = %q", c.Missing, c.Extra)
	}
	want := jsonChange{Key: "B", Reference: "a-****ue", Target: "an****ue"}
	if len(c.Differs) != 1 || c.Differs[0] != want {
		t.Errorf("differs = %+v, want [%+v]", c.Differs, want)
	}

	// Key-only comparisons still emit empty arrays, not null.
	_, out, _ = runCLI("--json", ".env", ".env")
	if !strings.Contains(out, `"missing": []`) || !strings.Contains(out, `"differs": []`) || !strings.Contains(out, `"match": true`) {
		t.Errorf("JSON for equal files:\n%s", out)
	}
}

func TestColor(t *testing.T) {
	setup(t, map[string]string{"a.env": "A=1\n", "b.env": "B=1\n"})
	orig := isTerminal
	t.Cleanup(func() { isTerminal = orig })
	isTerminal = func(io.Writer) bool { return true }
	t.Setenv("TERM", "xterm-256color")

	t.Setenv("NO_COLOR", "")
	if _, out, _ := runCLI("a.env", "b.env"); !strings.Contains(out, "\x1b[31m- A") {
		t.Errorf("expected colored output on a terminal, got %q", out)
	}
	t.Setenv("NO_COLOR", "1")
	if _, out, _ := runCLI("a.env", "b.env"); strings.Contains(out, "\x1b[") {
		t.Errorf("NO_COLOR set but output has escapes: %q", out)
	}
	t.Setenv("NO_COLOR", "")
	isTerminal = orig
	if _, out, _ := runCLI("a.env", "b.env"); strings.Contains(out, "\x1b[") {
		t.Errorf("non-terminal output has escapes: %q", out)
	}
}

func TestRunMatrix(t *testing.T) {
	setup(t, map[string]string{
		".env":            "A=1\nB=2\nC=3\nD=4\n",
		".env.staging":    "A=1\nB=2\nC=3\nD=4\nE=5\n",
		".env.production": "A=1\nB=two\nD=4\n",
		".env.copy":       "D=4\nC=3\nB=2\nA=1\n",
	})
	code, out, errOut := runCLI(".env", ".env.staging", ".env.production")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (stderr %q)", code, errOut)
	}
	want := "" +
		"KEY  .env  .env.staging  .env.production\n" +
		"C    ✓     ✓             -\n" +
		"E    -     ✓             -\n" +
		"\n" +
		"2 of 5 keys differ across 3 files\n"
	if out != want {
		t.Errorf("matrix output:\n%s\nwant:\n%s", out, want)
	}

	_, out, _ = runCLI("--values", "--show-values", "--only-missing", ".env", ".env.staging", ".env.production")
	for _, s := range []string{"B    ✓     ✓             ~", `~ B  .env="2" .env.production="two"`, "2 of 4 keys differ"} {
		if !strings.Contains(out, s) {
			t.Errorf("--values matrix missing %q:\n%s", s, out)
		}
	}

	code, out, _ = runCLI(".env", ".env.copy", ".env.copy")
	if code != 0 || out != "all 3 files match (4 keys)\n" {
		t.Errorf("matching files: exit %d, output %q", code, out)
	}

	code, out, _ = runCLI("--json", ".env", ".env.staging", ".env.copy")
	var rep jsonReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if code != 1 || rep.Match || len(rep.Files) != 3 || len(rep.Comparisons) != 2 ||
		rep.Comparisons[0].Match || !rep.Comparisons[1].Match {
		t.Errorf("JSON for three files: exit %d, %+v", code, rep)
	}
}

func TestRunStrict(t *testing.T) {
	setup(t, map[string]string{
		".env":         "API_KEY=abc\nDEBUG=0\nlog_level=info\nDEBUG=1\n",
		".env.example": "API_KEY=\nDEBUG=0\nlog_level=\n",
		"clean.env":    "API_KEY=x\nDEBUG=1\n",
		"clean2.env":   "API_KEY=y\nDEBUG=0\n",
	})
	if code, _, _ := runCLI(".env", ".env.example"); code != 0 {
		t.Fatalf("keys match without --strict, got exit %d", code)
	}
	code, out, _ := runCLI("--strict", ".env", ".env.example")
	if code != 1 {
		t.Errorf("--strict exit %d, want 1", code)
	}
	for _, want := range []string{
		"! DEBUG      duplicate in .env (lines 2, 4)",
		"! log_level  not UPPER_SNAKE_CASE in .env (line 3)",
		"! API_KEY    empty in .env.example (line 1)",
		"! log_level  empty in .env.example (line 3)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--strict output missing %q:\n%s", want, out)
		}
	}

	if code, out, _ := runCLI("--strict", "--ignore", "log_level,API_KEY,DEBUG", ".env", ".env.example"); code != 0 {
		t.Errorf("--strict with everything ignored: exit %d\n%s", code, out)
	}
	if code, _, _ := runCLI("--strict", "clean.env", "clean2.env", "clean.env"); code != 0 {
		t.Errorf("--strict on clean files: exit %d", code)
	}
	// A file given twice is only checked once.
	_, out, _ = runCLI("--strict", ".env", ".env.example", ".env")
	if n := strings.Count(out, "duplicate in .env"); n != 1 {
		t.Errorf("duplicate reported %d times:\n%s", n, out)
	}

	_, out, _ = runCLI("--strict", "--json", ".env", ".env.example")
	var rep jsonReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Match || len(rep.Issues) != 5 || rep.Issues[0].Kind != "duplicate" || len(rep.Comparisons[0].Missing) != 0 {
		t.Errorf("--strict --json: %+v", rep)
	}
}

func TestRunExpand(t *testing.T) {
	setup(t, map[string]string{
		"a.env": "HOST=db\nURL=postgres://${HOST}:${PORT:-5432}/app\nHOME_DIR=$ENVDIFF_TEST_HOME\n",
		"b.env": "HOST=db\nURL=postgres://db:5432/app\nHOME_DIR=/home/test\n",
		"c.env": "URL=${HOST\n",
	})
	t.Setenv("ENVDIFF_TEST_HOME", "/home/test")
	if code, out, _ := runCLI("--values", "a.env", "b.env"); code != 1 {
		t.Errorf("without --expand: exit %d, want 1\n%s", code, out)
	}
	if code, out, errOut := runCLI("--values", "--expand", "a.env", "b.env"); code != 0 {
		t.Errorf("with --expand: exit %d, want 0\n%s%s", code, out, errOut)
	}
	if code, _, errOut := runCLI("--expand", "a.env", "c.env"); code != 2 || !strings.Contains(errOut, "c.env:1: unterminated ${") {
		t.Errorf("bad reference: exit %d, stderr %q", code, errOut)
	}
}

func TestParseArgsFlagsAnywhere(t *testing.T) {
	tests := []struct {
		args   []string
		files  []string
		values bool
	}{
		{[]string{"--values", "a", "b"}, []string{"a", "b"}, true},
		{[]string{"a", "b", "--values"}, []string{"a", "b"}, true},
		{[]string{"a", "--values", "b", "-q"}, []string{"a", "b"}, true},
		{[]string{"a", "--ignore", "X,Y", "b", "--ignore=Z"}, []string{"a", "b"}, false},
		{[]string{"--values", "--", "a", "--weird-name"}, []string{"a", "--weird-name"}, true},
		{[]string{"a", "--", "-b", "--values"}, []string{"a", "-b", "--values"}, false},
	}
	for _, tt := range tests {
		c, err := parseArgs(tt.args)
		if err != nil {
			t.Errorf("parseArgs(%q): %v", tt.args, err)
			continue
		}
		if !reflect.DeepEqual(c.files, tt.files) {
			t.Errorf("parseArgs(%q) files = %q, want %q", tt.args, c.files, tt.files)
		}
		if c.values != tt.values {
			t.Errorf("parseArgs(%q) values = %v, want %v", tt.args, c.values, tt.values)
		}
	}

	c, err := parseArgs([]string{"a", "--ignore", "X,Y", "b", "--ignore=Z"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"X", "Y", "Z"}; !reflect.DeepEqual([]string(c.ignore), want) {
		t.Errorf("ignore = %q, want %q", c.ignore, want)
	}
}

func TestRunFlagsAfterFiles(t *testing.T) {
	setup(t, map[string]string{"a.env": "A=1\n", "b.env": "A=2\n"})
	if code, out, errOut := runCLI("a.env", "b.env", "--values"); code != 1 || !strings.Contains(out, "~ A") {
		t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	if code, out, _ := runCLI("a.env", "-q", "b.env", "--values"); code != 1 || out != "" {
		t.Errorf("exit %d, stdout %q", code, out)
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
		{"€uro-secret-token-€", "€u****-€"},
		{"ключ-доступа", "кл****па"},
		{"секретныйкл", "<redacted>"}, // 11 runes but 22 bytes
		{"pässwört", "<redacted>"},
	}
	for _, tt := range tests {
		got := mask(tt.in)
		if got != tt.want {
			t.Errorf("mask(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("mask(%q) = %q is not valid UTF-8", tt.in, got)
		}
	}
}

func TestRunEscapesMaskedValues(t *testing.T) {
	setup(t, map[string]string{
		"a.env": `KEY="-----BEGIN KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END KEY-----\n"` + "\nT=\x1b]0;x\x07abcdefghijklmnop\x1b[2J\n",
		"b.env": `KEY="-----BEGIN KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASD\n-----END KEY-----\n"` + "\nT=zzzzzzzzzzzzzzzzzzzz\n",
		"c.env": "KEY=x\nT=zzzzzzzzzzzzzzzzzzzz\n",
	})
	_, out, _ := runCLI("--values", "a.env", "b.env")
	want := "" +
		`~ KEY  differs: a.env=--****-\n b.env=--****-\n` + "\n" +
		`~ T    differs: a.env=\x1b]****2J b.env=zz****zz` + "\n"
	if out != want {
		t.Errorf("output:\n%q\nwant:\n%q", out, want)
	}

	_, out, _ = runCLI("--values", "a.env", "b.env", "c.env")
	if want := `~ KEY  a.env=--****-\n b.env=--****-\n c.env=<redacted>` + "\n"; !strings.Contains(out, want) {
		t.Errorf("matrix output:\n%s\nwant it to contain %q", out, want)
	}
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Errorf("matrix output has raw control characters: %q", out)
	}
}

func TestRunMatrixWideNames(t *testing.T) {
	setup(t, map[string]string{
		".env.example":       "A=1\nB=1\n",
		"本番.env":             "A=1\n",
		"ステージング.env":         "B=1\n",
		"cafe\u0301.env":     "A=1\n", // decomposed (NFD), as macOS tools often write it
		"\u1112\u1161\u11ab": "B=1\n", // decomposed Hangul
	})
	tests := []struct {
		files []string
		want  string
	}{
		{[]string{".env.example", "本番.env", "ステージング.env"}, "" +
			"KEY  .env.example  本番.env  ステージング.env\n" +
			"A    ✓             ✓         -\n" +
			"B    ✓             -         ✓\n"},
		{[]string{".env.example", "cafe\u0301.env", "\u1112\u1161\u11ab", ".env.example"}, "" +
			"KEY  .env.example  cafe\u0301.env  \u1112\u1161\u11ab  .env.example\n" +
			"A    ✓             ✓         -   ✓\n" +
			"B    ✓             -         ✓   ✓\n"},
	}
	for _, tt := range tests {
		_, out, _ := runCLI(tt.files...)
		if !strings.HasPrefix(out, tt.want) {
			t.Errorf("envdiff %s:\n%s\nwant:\n%s", strings.Join(tt.files, " "), out, tt.want)
		}
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{".env.production", 15},
		{"本番.env", 8},
		{"ステージング.env", 16},
		{"cafe\u0301.env", 8},
		{"\u1112\u1161\u11ab", 2},
		{"ｱ", 1}, // halfwidth katakana
		{"ＡＢ", 4},
		{"\u200b", 0},
	}
	for _, tt := range tests {
		if got := displayWidth(tt.in); got != tt.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
