package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dan2756/envdiff/internal/diff"
)

const (
	red     = "31"
	green   = "32"
	yellow  = "33"
	magenta = "35"
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

// entry is one line of the list output: "<sym> KEY  message".
type entry struct {
	sym   byte
	color string
	key   string
	msg   string
}

// print writes the human-readable report.
func (p *printer) print(r *report) {
	if len(r.files) > 2 {
		p.matrix(r)
		return
	}
	res := r.results[0]
	if r.match() {
		fmt.Fprintf(p.w, "%s and %s match (%d keys)\n", res.Reference.Name, res.Target.Name, res.Common)
		return
	}
	ref, target := res.Reference.Name, res.Target.Name
	var es []entry
	for _, k := range res.Missing {
		es = append(es, entry{'-', red, k, "missing in " + target})
	}
	for _, k := range res.Extra {
		es = append(es, entry{'+', green, k, "only in " + target})
	}
	for _, c := range res.Changed {
		es = append(es, entry{'~', yellow, c.Key, fmt.Sprintf("differs: %s=%s %s=%s",
			ref, p.value(c.Reference), target, p.value(c.Target))})
	}
	p.entries(append(es, issueEntries(r.issues)...))
}

// matrix prints the keys that are not identical everywhere as a table with
// one column per file, followed by the differing values when --values is on
// and any --strict issues.
func (p *printer) matrix(r *report) {
	var bad []diff.Row
	for _, row := range r.rows {
		if !row.Complete() {
			bad = append(bad, row)
		}
	}
	if len(bad) == 0 {
		fmt.Fprintf(p.w, "all %d files match (%d keys)\n", len(r.files), len(r.rows))
		if len(r.issues) > 0 {
			fmt.Fprintln(p.w)
			p.entries(issueEntries(r.issues))
		}
		return
	}

	width := len("KEY")
	for _, row := range bad {
		width = max(width, len(row.Key))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s", width, "KEY")
	for _, f := range r.files {
		b.WriteString("  " + f.Name)
	}
	fmt.Fprintln(p.w, b.String())

	ref := r.files[0]
	var details []entry
	for _, row := range bad {
		b.Reset()
		fmt.Fprintf(&b, "%-*s", width, row.Key)
		var vals []string
		for i, f := range r.files {
			mark, color := "✓", green
			switch {
			case row.Differs[i]:
				mark, color = "~", yellow
				v, _ := f.Lookup(row.Key)
				vals = append(vals, f.Name+"="+p.value(v))
			case !row.Present[i]:
				mark, color = "-", red
			}
			b.WriteString("  " + p.paint(color, mark))
			if i < len(r.files)-1 {
				b.WriteString(strings.Repeat(" ", utf8.RuneCountInString(f.Name)-1))
			}
		}
		fmt.Fprintln(p.w, b.String())
		if len(vals) > 0 {
			v, _ := ref.Lookup(row.Key)
			msg := ref.Name + "=" + p.value(v) + " " + strings.Join(vals, " ")
			details = append(details, entry{'~', yellow, row.Key, msg})
		}
	}
	for _, es := range [][]entry{details, issueEntries(r.issues)} {
		if len(es) > 0 {
			fmt.Fprintln(p.w)
			p.entries(es)
		}
	}
	fmt.Fprintf(p.w, "\n%d of %d keys differ across %d files\n", len(bad), len(r.rows), len(r.files))
}

func issueEntries(issues []diff.Issue) []entry {
	var es []entry
	for _, is := range issues {
		var problem string
		switch is.Kind {
		case diff.BadName:
			problem = "not UPPER_SNAKE_CASE"
		case diff.Duplicate:
			problem = "duplicate"
		case diff.Empty:
			problem = "empty"
		}
		es = append(es, entry{'!', magenta, is.Key, fmt.Sprintf("%s in %s (%s)", problem, is.File, lineList(is.Lines))})
	}
	return es
}

func lineList(lines []int) string {
	if len(lines) == 1 {
		return "line " + strconv.Itoa(lines[0])
	}
	s := make([]string, len(lines))
	for i, n := range lines {
		s[i] = strconv.Itoa(n)
	}
	return "lines " + strings.Join(s, ", ")
}

// entries prints es with the keys aligned.
func (p *printer) entries(es []entry) {
	width := 0
	for _, e := range es {
		width = max(width, len(e.key))
	}
	for _, e := range es {
		head := fmt.Sprintf("%c %-*s", e.sym, width, e.key)
		fmt.Fprintf(p.w, "%s  %s\n", p.paint(e.color, head), e.msg)
	}
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
