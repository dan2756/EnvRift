package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

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
				b.WriteString(strings.Repeat(" ", max(displayWidth(f.Name)-1, 0)))
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
// masked otherwise. The characters a mask keeps are escaped the same way,
// so a trailing newline or a terminal escape can't break the line.
func (p *printer) value(v string) string {
	if p.showValues {
		return strconv.Quote(v)
	}
	if v == "" {
		return `""`
	}
	q := strconv.Quote(mask(v))
	return q[1 : len(q)-1]
}

// mask hides most of a value. Long values keep two characters at each end
// so they can still be told apart; short ones are redacted entirely.
// Lengths are counted in runes so multibyte characters are never split.
func mask(v string) string {
	r := []rune(v)
	switch {
	case len(r) == 0:
		return ""
	case len(r) < 12:
		return "<redacted>"
	}
	return string(r[:2]) + "****" + string(r[len(r)-2:])
}

// displayWidth returns the number of terminal columns s takes up:
// combining marks and other zero-width characters take none, East Asian
// wide and fullwidth characters take two.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, combiningJamo):
		case unicode.Is(wide, r):
			n += 2
		default:
			n++
		}
	}
	return n
}

// combiningJamo are the medial vowels and final consonants of
// decomposed (NFD) Hangul, which combine with the preceding consonant.
var combiningJamo = &unicode.RangeTable{R16: []unicode.Range16{{Lo: 0x1160, Hi: 0x11ff, Stride: 1}}}

// wide holds the East Asian wide and fullwidth ranges, as in Markus Kuhn's
// wcwidth.
var wide = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x1100, Hi: 0x115f, Stride: 1}, // Hangul Jamo initial consonants
		{Lo: 0x2329, Hi: 0x232a, Stride: 1}, // angle brackets
		{Lo: 0x2e80, Hi: 0x303e, Stride: 1}, // CJK radicals, symbols and punctuation
		{Lo: 0x3040, Hi: 0xa4cf, Stride: 1}, // kana, CJK ideographs, Yi
		{Lo: 0xac00, Hi: 0xd7a3, Stride: 1}, // Hangul syllables
		{Lo: 0xf900, Hi: 0xfaff, Stride: 1}, // CJK compatibility ideographs
		{Lo: 0xfe10, Hi: 0xfe19, Stride: 1}, // vertical forms
		{Lo: 0xfe30, Hi: 0xfe6f, Stride: 1}, // CJK compatibility forms
		{Lo: 0xff00, Hi: 0xff60, Stride: 1}, // fullwidth forms
		{Lo: 0xffe0, Hi: 0xffe6, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x1f300, Hi: 0x1f64f, Stride: 1}, // pictographs and emoticons
		{Lo: 0x1f900, Hi: 0x1f9ff, Stride: 1},
		{Lo: 0x20000, Hi: 0x2fffd, Stride: 1}, // CJK extensions
		{Lo: 0x30000, Hi: 0x3fffd, Stride: 1},
	},
}
