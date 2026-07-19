package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dan2756/envdiff/internal/diff"
	"github.com/dan2756/envdiff/internal/dotenv"
)

const (
	red    = "31"
	green  = "32"
	yellow = "33"
)

// printer writes the human-readable report.
type printer struct {
	w          io.Writer
	showValues bool
	color      bool
}

// colorEnabled reports whether output to w should be colored: only on a
// terminal, and never when NO_COLOR is set or TERM is "dumb".
func colorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTerminal(w)
}

// isTerminal is a variable so tests can pretend to be a terminal.
var isTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func (p *printer) paint(color, s string) string {
	if !p.color {
		return s
	}
	return "\x1b[" + color + "m" + s + "\x1b[0m"
}

func (p *printer) result(r *diff.Result) {
	ref, target := r.Reference.Name, r.Target.Name
	if r.Equal() {
		fmt.Fprintf(p.w, "%s and %s match (%d keys)\n", ref, target, r.Common)
		return
	}
	width := 0
	for _, k := range r.Missing {
		width = max(width, len(k))
	}
	for _, k := range r.Extra {
		width = max(width, len(k))
	}
	for _, c := range r.Changed {
		width = max(width, len(c.Key))
	}
	for _, k := range r.Missing {
		p.line('-', red, width, k, "missing in "+target)
	}
	for _, k := range r.Extra {
		p.line('+', green, width, k, "only in "+target)
	}
	for _, c := range r.Changed {
		p.line('~', yellow, width, c.Key, fmt.Sprintf("differs: %s=%s %s=%s",
			ref, p.value(c.Reference), target, p.value(c.Target)))
	}
}

// matrix prints the keys that are not identical everywhere as a table with
// one column per file, followed by the differing values when --values is on.
func (p *printer) matrix(files []*dotenv.File, rows []diff.Row) {
	var bad []diff.Row
	for _, r := range rows {
		if !r.Complete() {
			bad = append(bad, r)
		}
	}
	if len(bad) == 0 {
		fmt.Fprintf(p.w, "all %d files match (%d keys)\n", len(files), len(rows))
		return
	}

	width := len("KEY")
	for _, r := range bad {
		width = max(width, len(r.Key))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s", width, "KEY")
	for _, f := range files {
		b.WriteString("  " + f.Name)
	}
	fmt.Fprintln(p.w, b.String())

	for _, r := range bad {
		b.Reset()
		fmt.Fprintf(&b, "%-*s", width, r.Key)
		for i, f := range files {
			mark, color := "✓", green
			switch {
			case r.Differs[i]:
				mark, color = "~", yellow
			case !r.Present[i]:
				mark, color = "-", red
			}
			b.WriteString("  " + p.paint(color, mark))
			if i < len(files)-1 {
				b.WriteString(strings.Repeat(" ", utf8.RuneCountInString(f.Name)-1))
			}
		}
		fmt.Fprintln(p.w, b.String())
	}

	var details []string
	for _, r := range bad {
		var vals []string
		for i, f := range files {
			if r.Differs[i] {
				v, _ := f.Lookup(r.Key)
				vals = append(vals, f.Name+"="+p.value(v))
			}
		}
		if len(vals) > 0 {
			ref, _ := files[0].Lookup(r.Key)
			details = append(details, fmt.Sprintf("%s %s  %s=%s %s", p.paint(yellow, "~"),
				r.Key, files[0].Name, p.value(ref), strings.Join(vals, " ")))
		}
	}
	if len(details) > 0 {
		fmt.Fprintln(p.w)
		for _, d := range details {
			fmt.Fprintln(p.w, d)
		}
	}
	fmt.Fprintf(p.w, "\n%d of %d keys differ across %d files\n", len(bad), len(rows), len(files))
}

func (p *printer) line(sym byte, color string, width int, key, msg string) {
	head := fmt.Sprintf("%c %-*s", sym, width, key)
	fmt.Fprintf(p.w, "%s  %s\n", p.paint(color, head), msg)
}

// value formats a value for display: quoted in full with --show-values,
// masked otherwise.
func (p *printer) value(v string) string {
	if p.showValues {
		return strconv.Quote(v)
	}
	if v == "" {
		return `""`
	}
	return mask(v)
}

// mask hides most of a value. Long values keep two characters at each end
// so they can still be told apart; short ones are redacted entirely.
func mask(v string) string {
	switch {
	case v == "":
		return ""
	case len(v) < 12:
		return "<redacted>"
	}
	return v[:2] + "****" + v[len(v)-2:]
}
