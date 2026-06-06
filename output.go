package main

import (
	"fmt"
	"io"
	"strconv"

	"github.com/dan2756/envdiff/internal/diff"
)

// printer writes the human-readable report.
type printer struct {
	w          io.Writer
	showValues bool
}

func (p *printer) result(r *diff.Result) {
	ref, target := r.Reference.Name, r.Target.Name
	if r.Equal() {
		fmt.Fprintf(p.w, "%s and %s match (%d keys)\n", ref, target, len(r.Reference.Keys()))
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
		p.line('-', width, k, "missing in "+target)
	}
	for _, k := range r.Extra {
		p.line('+', width, k, "only in "+target)
	}
	for _, c := range r.Changed {
		p.line('~', width, c.Key, fmt.Sprintf("differs: %s=%s %s=%s",
			ref, p.value(c.Reference), target, p.value(c.Target)))
	}
}

func (p *printer) line(sym byte, width int, key, msg string) {
	fmt.Fprintf(p.w, "%c %-*s  %s\n", sym, width, key, msg)
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
