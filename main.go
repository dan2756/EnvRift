// Command envdiff compares .env files and exits nonzero when they differ.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/dan2756/envdiff/internal/dotenv"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: envdiff [flags] FILE OTHER

Compare the keys of two .env files. Keys present in FILE but not in OTHER
are reported as missing; keys only in OTHER are reported as extra.

Flags:
  -h, --help      show this help
      --version   print the version and exit

Exit status: 0 if the files match, 1 if they differ, 2 on error.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("envdiff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	showVersion := fs.Bool("version", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		return usageError(stderr, err.Error())
	}
	if *showVersion {
		fmt.Fprintln(stdout, "envdiff", buildVersion())
		return 0
	}
	if fs.NArg() != 2 {
		return usageError(stderr, fmt.Sprintf("expected two files, got %d", fs.NArg()))
	}

	ref, err := dotenv.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "envdiff: %v\n", err)
		return 2
	}
	other, err := dotenv.ReadFile(fs.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "envdiff: %v\n", err)
		return 2
	}

	var missing, extra []string
	for _, k := range ref.Keys() {
		if !other.Has(k) {
			missing = append(missing, k)
		}
	}
	for _, k := range other.Keys() {
		if !ref.Has(k) {
			extra = append(extra, k)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		fmt.Fprintf(stdout, "%s and %s match (%d keys)\n", ref.Name, other.Name, len(ref.Keys()))
		return 0
	}

	width := 0
	for _, k := range append(missing, extra...) {
		width = max(width, len(k))
	}
	for _, k := range missing {
		fmt.Fprintf(stdout, "- %-*s  missing in %s\n", width, k, other.Name)
	}
	for _, k := range extra {
		fmt.Fprintf(stdout, "+ %-*s  only in %s\n", width, k, other.Name)
	}
	return 1
}

func usageError(stderr io.Writer, msg string) int {
	fmt.Fprintf(stderr, "envdiff: %s\nTry 'envdiff --help' for more information.\n", msg)
	return 2
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
