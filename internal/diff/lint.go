package diff

import "github.com/dan2756/envdiff/internal/dotenv"

// IssueKind classifies a Lint finding.
type IssueKind string

const (
	BadName   IssueKind = "name"      // key is not UPPER_SNAKE_CASE
	Duplicate IssueKind = "duplicate" // key is assigned more than once
	Empty     IssueKind = "empty"     // key's effective value is empty
)

// Issue is a problem Lint found with one key.
type Issue struct {
	File  string
	Key   string
	Kind  IssueKind
	Lines []int // the relevant assignments: all of them for duplicates
}

// Lint reports keys in f that are not UPPER_SNAKE_CASE, are assigned more
// than once, or end up with an empty value. Ignored keys are skipped.
func Lint(f *dotenv.File, opts Options) []Issue {
	lines := make(map[string][]int)
	for _, e := range f.Entries {
		lines[e.Key] = append(lines[e.Key], e.Line)
	}
	var issues []Issue
	for _, k := range f.Keys() {
		if opts.Ignored(k) {
			continue
		}
		ls := lines[k]
		if !isUpperSnake(k) {
			issues = append(issues, Issue{File: f.Name, Key: k, Kind: BadName, Lines: ls[:1]})
		}
		if len(ls) > 1 {
			issues = append(issues, Issue{File: f.Name, Key: k, Kind: Duplicate, Lines: ls})
		}
		if v, _ := f.Lookup(k); v == "" {
			issues = append(issues, Issue{File: f.Name, Key: k, Kind: Empty, Lines: ls[len(ls)-1:]})
		}
	}
	return issues
}

// isUpperSnake reports whether s consists of A-Z, 0-9 and '_' and does
// not start with a digit.
func isUpperSnake(s string) bool {
	if s == "" || ('0' <= s[0] && s[0] <= '9') {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('A' <= c && c <= 'Z') && !('0' <= c && c <= '9') && c != '_' {
			return false
		}
	}
	return true
}
