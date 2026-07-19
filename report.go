package main

import (
	"encoding/json"
	"io"

	"github.com/dan2756/envdiff/internal/diff"
	"github.com/dan2756/envdiff/internal/dotenv"
)

// jsonReport is the --json output. Slices are always present (possibly
// empty) so consumers don't have to special-case null.
type jsonReport struct {
	Reference   string           `json:"reference"`
	Files       []string         `json:"files"`
	Match       bool             `json:"match"`
	Comparisons []jsonComparison `json:"comparisons"`
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

func writeJSON(w io.Writer, files []*dotenv.File, results []*diff.Result, showValues bool) error {
	value := mask
	if showValues {
		value = func(v string) string { return v }
	}
	rep := jsonReport{
		Reference:   files[0].Name,
		Files:       make([]string, 0, len(files)),
		Match:       true,
		Comparisons: make([]jsonComparison, 0, len(results)),
	}
	for _, f := range files {
		rep.Files = append(rep.Files, f.Name)
	}
	for _, r := range results {
		c := jsonComparison{
			File:    r.Target.Name,
			Match:   r.Equal(),
			Missing: r.Missing,
			Extra:   r.Extra,
			Differs: make([]jsonChange, 0, len(r.Changed)),
		}
		for _, ch := range r.Changed {
			c.Differs = append(c.Differs, jsonChange{
				Key:       ch.Key,
				Reference: value(ch.Reference),
				Target:    value(ch.Target),
			})
		}
		rep.Match = rep.Match && c.Match
		rep.Comparisons = append(rep.Comparisons, c)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(rep)
}
