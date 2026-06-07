package main

import (
	"encoding/json"
	"io"

	"github.com/dan2756/envdiff/internal/diff"
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

func writeJSON(w io.Writer, r *diff.Result, showValues bool) error {
	value := mask
	if showValues {
		value = func(v string) string { return v }
	}
	c := jsonComparison{
		File:    r.Target.Name,
		Match:   r.Equal(),
		Missing: r.Missing,
		Extra:   r.Extra,
		Differs: []jsonChange{},
	}
	for _, ch := range r.Changed {
		c.Differs = append(c.Differs, jsonChange{
			Key:       ch.Key,
			Reference: value(ch.Reference),
			Target:    value(ch.Target),
		})
	}
	rep := jsonReport{
		Reference:   r.Reference.Name,
		Files:       []string{r.Reference.Name, r.Target.Name},
		Match:       r.Equal(),
		Comparisons: []jsonComparison{c},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(rep)
}
