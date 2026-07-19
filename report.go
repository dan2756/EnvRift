package main

import (
	"encoding/json"
	"io"

	"github.com/dan2756/envdiff/internal/diff"
	"github.com/dan2756/envdiff/internal/dotenv"
)

// report is everything a run found.
type report struct {
	files   []*dotenv.File
	results []*diff.Result // one per file after the reference
	rows    []diff.Row     // key matrix, only built for three or more files
	issues  []diff.Issue   // --strict findings
}

func compare(files []*dotenv.File, opts diff.Options, strict bool) *report {
	rep := &report{files: files}
	for _, f := range files[1:] {
		rep.results = append(rep.results, diff.Compare(files[0], f, opts))
	}
	if len(files) > 2 {
		rep.rows = diff.Matrix(files, opts)
	}
	if strict {
		linted := make(map[string]bool)
		for _, f := range files {
			if !linted[f.Name] {
				linted[f.Name] = true
				rep.issues = append(rep.issues, diff.Lint(f, opts)...)
			}
		}
	}
	return rep
}

// match reports whether the run should exit 0.
func (r *report) match() bool {
	for _, res := range r.results {
		if !res.Equal() {
			return false
		}
	}
	return len(r.issues) == 0
}

// jsonReport is the --json output. Slices are always present (possibly
// empty) so consumers don't have to special-case null.
type jsonReport struct {
	Reference   string           `json:"reference"`
	Files       []string         `json:"files"`
	Match       bool             `json:"match"`
	Comparisons []jsonComparison `json:"comparisons"`
	Issues      []jsonIssue      `json:"issues"`
}

type jsonComparison struct {
	File    string       `json:"file"`
	Match   bool         `json:"match"`
	Missing []string     `json:"missing"`
	Extra   []string     `json:"extra"`
	Differs []jsonChange `json:"differs"`
}

type jsonChange struct {
	Key       string `json:"key"`
	Reference string `json:"reference"`
	Target    string `json:"target"`
}

type jsonIssue struct {
	File  string `json:"file"`
	Key   string `json:"key"`
	Kind  string `json:"kind"`
	Lines []int  `json:"lines"`
}

func writeJSON(w io.Writer, r *report, showValues bool) error {
	value := mask
	if showValues {
		value = func(v string) string { return v }
	}
	out := jsonReport{
		Reference:   r.files[0].Name,
		Files:       make([]string, 0, len(r.files)),
		Match:       r.match(),
		Comparisons: make([]jsonComparison, 0, len(r.results)),
		Issues:      make([]jsonIssue, 0, len(r.issues)),
	}
	for _, f := range r.files {
		out.Files = append(out.Files, f.Name)
	}
	for _, res := range r.results {
		c := jsonComparison{
			File:    res.Target.Name,
			Match:   res.Equal(),
			Missing: res.Missing,
			Extra:   res.Extra,
			Differs: make([]jsonChange, 0, len(res.Changed)),
		}
		for _, ch := range res.Changed {
			c.Differs = append(c.Differs, jsonChange{
				Key:       ch.Key,
				Reference: value(ch.Reference),
				Target:    value(ch.Target),
			})
		}
		out.Comparisons = append(out.Comparisons, c)
	}
	for _, is := range r.issues {
		out.Issues = append(out.Issues, jsonIssue{File: is.File, Key: is.Key, Kind: string(is.Kind), Lines: is.Lines})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
