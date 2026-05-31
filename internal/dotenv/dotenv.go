// Package dotenv reads .env files.
package dotenv

import (
	"fmt"
	"os"
	"strings"
)

// Entry is a single KEY=value assignment.
type Entry struct {
	Key   string
	Value string
	Line  int // 1-based line on which the assignment starts
}

// File holds the assignments of a parsed .env file in source order.
// A key may appear more than once; the last assignment wins.
type File struct {
	Name    string
	Entries []Entry
	last    map[string]int
}

func newFile(name string) *File {
	return &File{Name: name, last: make(map[string]int)}
}

func (f *File) add(e Entry) {
	f.Entries = append(f.Entries, e)
	f.last[e.Key] = len(f.Entries) - 1
}

// Keys returns the distinct keys in the order they first appear.
func (f *File) Keys() []string {
	seen := make(map[string]bool, len(f.last))
	keys := make([]string, 0, len(f.last))
	for _, e := range f.Entries {
		if !seen[e.Key] {
			seen[e.Key] = true
			keys = append(keys, e.Key)
		}
	}
	return keys
}

// Has reports whether key is assigned anywhere in the file.
func (f *File) Has(key string) bool {
	_, ok := f.last[key]
	return ok
}

// Lookup returns the value of the last assignment to key.
func (f *File) Lookup(key string) (string, bool) {
	i, ok := f.last[key]
	if !ok {
		return "", false
	}
	return f.Entries[i].Value, true
}

// ParseError describes malformed input at a specific line.
type ParseError struct {
	File string
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
}

// ReadFile reads and parses the .env file at path.
func ReadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, data)
}

// Parse parses data as the contents of a .env file. name is used in
// error messages.
//
// Supported syntax: KEY=value lines with optional "export " prefix and
// whitespace around '=', full-line and inline comments, single-quoted
// (literal), double-quoted (with \n, \t, \" and \\ escapes) and
// backtick-quoted values.
func Parse(name string, data []byte) (*File, error) {
	p := &parser{name: name, src: string(data), line: 1, file: newFile(name)}
	if err := p.parse(); err != nil {
		return nil, err
	}
	return p.file, nil
}

type parser struct {
	name string
	src  string
	pos  int
	line int
	file *File
}

func (p *parser) parse() error {
	for {
		p.skipBlanks()
		if p.eof() {
			return nil
		}
		switch p.src[p.pos] {
		case '\n':
			p.pos++
			p.line++
		case '#':
			p.skipLine()
		default:
			if err := p.assignment(); err != nil {
				return err
			}
		}
	}
}

// assignment parses one KEY=value statement, leaving the cursor at the
// end of its (last) line.
func (p *parser) assignment() error {
	line := p.line
	if p.hasExport() {
		p.pos += len("export")
		p.skipBlanks()
	}
	start := p.pos
	for !p.eof() && isKeyChar(p.src[p.pos]) {
		p.pos++
	}
	key := p.src[start:p.pos]
	if key == "" {
		return p.errorf("expected KEY=value, got %q", p.restOfLine())
	}
	if !validKey(key) {
		return p.errorf("invalid key %q", key)
	}
	p.skipBlanks()
	if p.eof() || p.src[p.pos] != '=' {
		return p.errorf("expected '=' after key %q", key)
	}
	p.pos++
	spaced := p.skipBlanks()
	value, err := p.value(spaced)
	if err != nil {
		return err
	}
	p.file.add(Entry{Key: key, Value: value, Line: line})
	return nil
}

// value parses the right-hand side of an assignment. spaced reports
// whether whitespace followed the '=', which turns a leading '#' into a
// comment rather than part of the value.
func (p *parser) value(spaced bool) (string, error) {
	if p.eof() {
		return "", nil
	}
	switch c := p.src[p.pos]; c {
	case '\'', '`':
		return p.quoted(c, false)
	case '"':
		return p.quoted(c, true)
	case '#':
		if spaced {
			p.skipLine()
			return "", nil
		}
	}
	return p.unquoted(), nil
}

// unquoted reads the rest of the line. A '#' preceded by whitespace
// starts a comment; surrounding whitespace is trimmed.
func (p *parser) unquoted() string {
	end := strings.IndexByte(p.src[p.pos:], '\n')
	if end < 0 {
		end = len(p.src) - p.pos
	}
	s := p.src[p.pos : p.pos+end]
	p.pos += end
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && isBlank(s[i-1]) {
			s = s[:i]
			break
		}
	}
	return strings.TrimRight(s, " \t")
}

// quoted reads a value delimited by q. Escape sequences are only
// interpreted when escapes is set (double quotes).
func (p *parser) quoted(q byte, escapes bool) (string, error) {
	open := p.line
	p.pos++
	var b strings.Builder
	for {
		if p.eof() || p.src[p.pos] == '\n' {
			return "", &ParseError{p.name, open, "unterminated " + quoteName(q) + " value"}
		}
		c := p.src[p.pos]
		switch {
		case c == q:
			p.pos++
			return b.String(), p.afterQuote(q)
		case c == '\\' && escapes:
			p.escape(&b)
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
}

// escape decodes the backslash sequence at the cursor.
func (p *parser) escape(b *strings.Builder) {
	if p.pos+1 >= len(p.src) || p.src[p.pos+1] == '\n' {
		b.WriteByte('\\')
		p.pos++
		return
	}
	c := p.src[p.pos+1]
	p.pos += 2
	switch c {
	case 'n':
		b.WriteByte('\n')
	case 'r':
		b.WriteByte('\r')
	case 't':
		b.WriteByte('\t')
	case '"', '\\':
		b.WriteByte(c)
	default:
		// Unknown escapes are kept as written, as most loaders do.
		b.WriteByte('\\')
		b.WriteByte(c)
	}
}

// afterQuote allows only whitespace or a comment after a closing quote.
func (p *parser) afterQuote(q byte) error {
	p.skipBlanks()
	if p.eof() || p.src[p.pos] == '\n' {
		return nil
	}
	if p.src[p.pos] == '#' {
		p.skipLine()
		return nil
	}
	return p.errorf("unexpected %q after closing %c", p.restOfLine(), q)
}

func (p *parser) hasExport() bool {
	const kw = "export"
	return strings.HasPrefix(p.src[p.pos:], kw) &&
		p.pos+len(kw) < len(p.src) && isBlank(p.src[p.pos+len(kw)])
}

func (p *parser) skipBlanks() bool {
	start := p.pos
	for !p.eof() && isBlank(p.src[p.pos]) {
		p.pos++
	}
	return p.pos > start
}

func (p *parser) skipLine() {
	if i := strings.IndexByte(p.src[p.pos:], '\n'); i >= 0 {
		p.pos += i
	} else {
		p.pos = len(p.src)
	}
}

func (p *parser) restOfLine() string {
	s := p.src[p.pos:]
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > 40 {
		s = s[:40] + "..."
	}
	return s
}

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) errorf(format string, args ...any) error {
	return &ParseError{File: p.name, Line: p.line, Msg: fmt.Sprintf(format, args...)}
}

func quoteName(q byte) string {
	switch q {
	case '\'':
		return "single-quoted"
	case '`':
		return "backtick-quoted"
	}
	return "double-quoted"
}

func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// validKey accepts the usual shell variable names plus '.' and '-',
// which some tools use in .env files.
func validKey(s string) bool {
	if s == "" || isDigit(s[0]) || s[0] == '.' || s[0] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isKeyChar(s[i]) {
			return false
		}
	}
	return true
}

func isKeyChar(c byte) bool {
	return c == '_' || c == '.' || c == '-' || isDigit(c) ||
		('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }
