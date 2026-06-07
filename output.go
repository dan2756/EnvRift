package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/dan2756/envdiff/internal/diff"
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
