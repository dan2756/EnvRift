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
func Parse(name string, data []byte) (*File, error) {
	f := newFile(name)
	for i, line := range strings.Split(string(data), "\n") {
		n := i + 1
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(rest)
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok {
			return nil, &ParseError{name, n, fmt.Sprintf("expected '=' after %q", key)}
		}
		if !validKey(key) {
			return nil, &ParseError{name, n, fmt.Sprintf("invalid key %q", key)}
		}
		value = strings.TrimSpace(value)
		if j := strings.Index(value, " #"); j >= 0 {
			value = strings.TrimSpace(value[:j])
		}
		f.add(Entry{Key: key, Value: value, Line: n})
	}
	return f, nil
}

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
