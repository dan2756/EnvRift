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

// ParseError describes malformed input at a specific line. Msg never
// quotes the input: a malformed line is often part of a value, such as an
// unquoted multi-line private key, and errors end up in CI logs.
type ParseError struct {
	File string
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
}

// Options controls optional parser behavior. The zero value reads values
// literally.
type Options struct {
	// Expand resolves $VAR, ${VAR}, ${VAR:-default} and ${VAR-default} in
	// unquoted and double-quoted values. Variables are looked up among the
	// assignments earlier in the file, then with LookupEnv.
	Expand bool
	// LookupEnv resolves variables the file doesn't define, typically
	// os.LookupEnv. Nil means undefined variables expand to "".
	LookupEnv func(string) (string, bool)
}

// ReadFile reads and parses the .env file at path with default options.
func ReadFile(path string) (*File, error) { return Options{}.ReadFile(path) }

// Parse parses data with default options. See Options.Parse.
func Parse(name string, data []byte) (*File, error) { return Options{}.Parse(name, data) }

// ReadFile reads and parses the .env file at path.
func (o Options) ReadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return o.Parse(path, data)
}

// Parse parses data as the contents of a .env file. name is used in
// error messages.
//
// Supported syntax: KEY=value lines with optional "export " prefix and
// whitespace around '=', full-line and inline comments, single-quoted
// (literal), double-quoted (with \n, \t, \", \\ and \$ escapes) and
// backtick-quoted values. Quoted values may span several lines. CRLF line
// endings and a leading UTF-8 byte order mark are accepted.
func (o Options) Parse(name string, data []byte) (*File, error) {
	src := strings.TrimPrefix(string(data), "\uFEFF")
	src = strings.ReplaceAll(src, "\r\n", "\n")
	p := &parser{name: name, src: src, line: 1, opts: o, file: newFile(name)}
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
	opts Options
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
		return p.errorf("expected KEY=value")
	}
	if !validKey(key) {
		return p.errorf("invalid key: must start with a letter or '_'")
	}
	p.skipBlanks()
	if p.eof() || p.src[p.pos] != '=' {
		return p.errorf("expected '=' after key")
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
	return p.unquoted()
}

// unquoted reads the rest of the line. A '#' preceded by whitespace
// starts a comment; surrounding whitespace is trimmed.
func (p *parser) unquoted() (string, error) {
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
	s = strings.TrimRight(s, " \t")
	if p.opts.Expand {
		return p.expandAll(s)
	}
	return s, nil
}

// quoted reads a value delimited by q, which may span lines. Escape
// sequences are only interpreted when escapes is set (double quotes).
// Errors for unterminated values point at the line of the opening quote.
func (p *parser) quoted(q byte, escapes bool) (string, error) {
	open := p.line
	p.pos++
	var b strings.Builder
	for {
		if p.eof() {
			return "", &ParseError{p.name, open, "unterminated " + quoteName(q) + " value"}
		}
		c := p.src[p.pos]
		switch {
		case c == q:
			p.pos++
			return b.String(), p.afterQuote(q)
		case c == '\\' && escapes:
			p.escape(&b)
		case c == '$' && escapes && p.opts.Expand:
			v, n, err := p.expand(p.src[p.pos:])
			if err != nil {
				return "", err
			}
			b.WriteString(v)
			p.pos += n
		case c == '\n':
			b.WriteByte(c)
			p.pos++
			p.line++
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
	case '"', '\\', '$':
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
	return p.errorf("unexpected text after closing %c", q)
}

// expandAll expands every variable reference in an unquoted value.
// "\$" stands for a literal dollar sign.
func (p *parser) expandAll(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == '$':
			b.WriteByte('$')
			i += 2
		case s[i] == '$':
			v, n, err := p.expand(s[i:])
			if err != nil {
				return "", err
			}
			b.WriteString(v)
			i += n
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String(), nil
}

// expand resolves the variable reference at the start of s, which begins
// with '$', returning its value and the number of bytes it spans. A '$'
// that doesn't start a reference stands for itself.
func (p *parser) expand(s string) (string, int, error) {
	if !strings.HasPrefix(s, "${") {
		n := 1
		for n < len(s) && isVarChar(s[n], n == 1) {
			n++
		}
		if n == 1 {
			return "$", 1, nil
		}
		v, _ := p.lookup(s[1:n])
		return v, n, nil
	}

	end := strings.IndexAny(s, "}\n")
	if end < 0 || s[end] != '}' {
		return "", 0, p.errorf("unterminated ${ in value")
	}
	name, op, def := s[2:end], "", ""
	if i := strings.Index(name, ":-"); i >= 0 {
		name, op, def = name[:i], ":-", name[i+2:]
	} else if i := strings.IndexByte(name, '-'); i >= 0 {
		name, op, def = name[:i], "-", name[i+1:]
	}
	if !isVarName(name) {
		return "", 0, p.errorf("invalid variable reference")
	}
	v, ok := p.lookup(name)
	if (op == ":-" && v == "") || (op == "-" && !ok) {
		v = def
	}
	return v, end + 1, nil
}

// lookup finds a variable among the assignments so far, then in the
// environment.
func (p *parser) lookup(name string) (string, bool) {
	if v, ok := p.file.Lookup(name); ok {
		return v, true
	}
	if p.opts.LookupEnv != nil {
		return p.opts.LookupEnv(name)
	}
	return "", false
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

func isVarName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isVarChar(s[i], i == 0) {
			return false
		}
	}
	return true
}

// isVarChar reports whether c can appear in a $VAR reference; digits are
// not allowed first.
func isVarChar(c byte, first bool) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || (!first && isDigit(c))
}
