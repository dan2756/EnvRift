package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dan2756/envdiff/internal/diff"
	"github.com/dan2756/envdiff/internal/dotenv"
)

const (
	exitOK    = 0
	exitDiff  = 1
	exitError = 2
)

const usage = `Usage: envdiff [flags] FILE OTHER

Compare two .env files. Keys present in FILE but not in OTHER are reported
as missing; keys only in OTHER are reported as extra. Only keys are compared
unless --values is given.

Flags:
      --values               also compare values; differing values are masked
      --show-values          print values in full (implies --values)
      --ignore KEYS          skip these keys (comma-separated, repeatable)
      --ignore-prefix PFX    skip keys starting with PFX (comma-separated, repeatable)
      --only-missing         only report keys missing from OTHER, not extras
  -h, --help                 show this help
      --version              print the version and exit

Exit status: 0 if the files match, 1 if they differ, 2 on error.
`

type config struct {
	files          []string
	values         bool
	showValues     bool
	onlyMissing    bool
	ignore         listFlag
	ignorePrefixes listFlag
	version        bool
}

// listFlag collects comma-separated values; the flag may be repeated.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(s string) error {
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			*l = append(*l, v)
		}
	}
	return nil
}

func parseArgs(args []string) (*config, error) {
	c := &config{}
	fs := flag.NewFlagSet("envdiff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&c.values, "values", false, "")
	fs.BoolVar(&c.showValues, "show-values", false, "")
	fs.BoolVar(&c.onlyMissing, "only-missing", false, "")
	fs.Var(&c.ignore, "ignore", "")
	fs.Var(&c.ignorePrefixes, "ignore-prefix", "")
	fs.BoolVar(&c.version, "version", false, "")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	c.files = fs.Args()
	if c.showValues {
		c.values = true
	}
	return c, nil
}

func (c *config) diffOptions() diff.Options {
	return diff.Options{
		Values:         c.values,
		OnlyMissing:    c.onlyMissing,
		Ignore:         c.ignore,
		IgnorePrefixes: c.ignorePrefixes,
	}
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(stdout, usage)
		return exitOK
	case err != nil:
		return usageError(stderr, err.Error())
	case cfg.version:
		fmt.Fprintln(stdout, "envdiff", buildVersion())
		return exitOK
	case len(cfg.files) != 2:
		return usageError(stderr, fmt.Sprintf("expected two files, got %d", len(cfg.files)))
	}

	files, ok := loadFiles(cfg.files, stderr)
	if !ok {
		return exitError
	}
	res := diff.Compare(files[0], files[1], cfg.diffOptions())
	p := &printer{w: stdout, showValues: cfg.showValues}
	p.result(res)
	if !res.Equal() {
		return exitDiff
	}
	return exitOK
}

// loadFiles parses every path, reporting all failures rather than
// stopping at the first one.
func loadFiles(paths []string, stderr io.Writer) ([]*dotenv.File, bool) {
	files := make([]*dotenv.File, 0, len(paths))
	ok := true
	for _, path := range paths {
		f, err := dotenv.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "envdiff: %v\n", err)
			ok = false
			continue
		}
		files = append(files, f)
	}
	return files, ok
}

func usageError(stderr io.Writer, msg string) int {
	fmt.Fprintf(stderr, "envdiff: %s\nTry 'envdiff --help' for more information.\n", msg)
	return exitError
}
