// Package diff compares parsed .env files.
package diff

import (
	"slices"
	"strings"

	"github.com/dan2756/envdiff/internal/dotenv"
)

// Options controls what Compare reports.
type Options struct {
	// Values also compares the values of keys present in both files.
	Values bool
	// OnlyMissing drops extra keys: those the target has but the
	// reference does not.
	OnlyMissing bool
	// Ignore lists keys to leave out entirely.
	Ignore []string
	// IgnorePrefixes leaves out every key starting with one of them.
	IgnorePrefixes []string
}

// Ignored reports whether key is excluded by Ignore or IgnorePrefixes.
func (o Options) Ignored(key string) bool {
	if slices.Contains(o.Ignore, key) {
		return true
	}
	for _, p := range o.IgnorePrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// Change is a key whose value differs between the reference and the target.
type Change struct {
	Key       string
	Reference string
	Target    string
}

// Result describes how a target file differs from the reference.
type Result struct {
	Reference *dotenv.File
	Target    *dotenv.File
	Missing   []string // keys in the reference that the target lacks
	Extra     []string // keys in the target that the reference lacks
	Changed   []Change // keys whose values differ (Options.Values only)
	Common    int      // number of compared keys present in both files
}

// Equal reports whether no differences were found.
func (r *Result) Equal() bool {
	return len(r.Missing) == 0 && len(r.Extra) == 0 && len(r.Changed) == 0
}

// Compare reports how target differs from ref. Missing and changed keys
// are listed in the reference's order, extra keys in the target's.
func Compare(ref, target *dotenv.File, opts Options) *Result {
	r := &Result{
		Reference: ref,
		Target:    target,
		Missing:   []string{},
		Extra:     []string{},
		Changed:   []Change{},
	}
	for _, k := range ref.Keys() {
		if opts.Ignored(k) {
			continue
		}
		tv, ok := target.Lookup(k)
		if !ok {
			r.Missing = append(r.Missing, k)
			continue
		}
		r.Common++
		if rv, _ := ref.Lookup(k); opts.Values && rv != tv {
			r.Changed = append(r.Changed, Change{Key: k, Reference: rv, Target: tv})
		}
	}
	if opts.OnlyMissing {
		return r
	}
	for _, k := range target.Keys() {
		if !ref.Has(k) && !opts.Ignored(k) {
			r.Extra = append(r.Extra, k)
		}
	}
	return r
}

// Row is one key's presence across a set of files.
type Row struct {
	Key     string
	Present []bool // per file, in the order given to Matrix
	Differs []bool // per file: value differs from the reference's (Options.Values only)
}

// Complete reports whether the key exists in every file, with the same
// value as the reference when values are compared.
func (r Row) Complete() bool {
	for i := range r.Present {
		if !r.Present[i] || r.Differs[i] {
			return false
		}
	}
	return true
}

// Matrix reports which keys exist in which of files, where files[0] is the
// reference. Rows come in the reference's order, followed by keys that only
// other files have, in the order they are first seen. With OnlyMissing, keys
// the reference lacks are left out.
func Matrix(files []*dotenv.File, opts Options) []Row {
	if len(files) == 0 {
		return nil
	}
	ref := files[0]
	var keys []string
	seen := make(map[string]bool)
	add := func(k string) {
		if !seen[k] && !opts.Ignored(k) {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, k := range ref.Keys() {
		add(k)
	}
	if !opts.OnlyMissing {
		for _, f := range files[1:] {
			for _, k := range f.Keys() {
				add(k)
			}
		}
	}

	rows := make([]Row, 0, len(keys))
	for _, k := range keys {
		row := Row{Key: k, Present: make([]bool, len(files)), Differs: make([]bool, len(files))}
		rv, inRef := ref.Lookup(k)
		for i, f := range files {
			v, ok := f.Lookup(k)
			row.Present[i] = ok
			row.Differs[i] = opts.Values && inRef && ok && v != rv
		}
		rows = append(rows, row)
	}
	return rows
}
