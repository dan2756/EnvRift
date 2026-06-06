package diff

import (
	"reflect"
	"testing"

	"github.com/dan2756/envdiff/internal/dotenv"
)

func parse(t *testing.T, name, src string) *dotenv.File {
	t.Helper()
	f, err := dotenv.Parse(name, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestCompareKeys(t *testing.T) {
	tests := []struct {
		name           string
		ref, target    string
		missing, extra []string
	}{
		{"identical", "A=1\nB=2", "A=1\nB=2", []string{}, []string{}},
		{"values ignored by default", "A=secret", "A=", []string{}, []string{}},
		{"order does not matter", "A=1\nB=2", "B=2\nA=1", []string{}, []string{}},
		{"missing", "A=1\nB=2\nC=3", "A=1", []string{"B", "C"}, []string{}},
		{"extra", "A=1", "A=1\nZ=1\nY=1", []string{}, []string{"Z", "Y"}},
		{"both", "A=1\nB=2", "B=2\nC=3", []string{"A"}, []string{"C"}},
		{"duplicates count once", "A=1\nA=2", "A=3\nB=1\nB=2", []string{}, []string{"B"}},
		{"empty target", "A=1", "", []string{"A"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Compare(parse(t, "ref", tt.ref), parse(t, "target", tt.target), Options{})
			if !reflect.DeepEqual(r.Missing, tt.missing) {
				t.Errorf("Missing = %q, want %q", r.Missing, tt.missing)
			}
			if !reflect.DeepEqual(r.Extra, tt.extra) {
				t.Errorf("Extra = %q, want %q", r.Extra, tt.extra)
			}
			if len(r.Changed) != 0 {
				t.Errorf("Changed = %v without Options.Values", r.Changed)
			}
			if want := len(tt.missing)+len(tt.extra) == 0; r.Equal() != want {
				t.Errorf("Equal() = %v, want %v", r.Equal(), want)
			}
		})
	}
}

func TestCompareValues(t *testing.T) {
	ref := parse(t, "ref", "A=1\nB=same\nC='quoted'\nD=x\nE=old\nE=new")
	target := parse(t, "target", "A=2\nB=same\nC=quoted\nE=new\nF=extra")
	r := Compare(ref, target, Options{Values: true})

	want := []Change{{Key: "A", Reference: "1", Target: "2"}}
	if !reflect.DeepEqual(r.Changed, want) {
		t.Errorf("Changed = %+v, want %+v", r.Changed, want)
	}
	if !reflect.DeepEqual(r.Missing, []string{"D"}) {
		t.Errorf("Missing = %q, want [D]", r.Missing)
	}
	if !reflect.DeepEqual(r.Extra, []string{"F"}) {
		t.Errorf("Extra = %q, want [F]", r.Extra)
	}
	if r.Equal() {
		t.Error("Equal() = true for files with differences")
	}
}

func TestCompareFilters(t *testing.T) {
	ref := parse(t, "ref", "A=1\nNEXT_PUBLIC_URL=x\nNEXT_PUBLIC_KEY=y\nSECRET=1\nLOCAL_ONLY=1")
	target := parse(t, "target", "A=2\nNEXT_PUBLIC_URL=z\nSECRET=1\nNEW=1\nNEXT_PUBLIC_NEW=1")
	tests := []struct {
		name           string
		opts           Options
		missing, extra []string
		changed        []string
		common         int
	}{
		{"none", Options{}, []string{"NEXT_PUBLIC_KEY", "LOCAL_ONLY"}, []string{"NEW", "NEXT_PUBLIC_NEW"}, nil, 3},
		{"ignore", Options{Ignore: []string{"LOCAL_ONLY", "NEW"}},
			[]string{"NEXT_PUBLIC_KEY"}, []string{"NEXT_PUBLIC_NEW"}, nil, 3},
		{"ignore prefix", Options{IgnorePrefixes: []string{"NEXT_PUBLIC_"}},
			[]string{"LOCAL_ONLY"}, []string{"NEW"}, nil, 2},
		{"only missing", Options{OnlyMissing: true},
			[]string{"NEXT_PUBLIC_KEY", "LOCAL_ONLY"}, []string{}, nil, 3},
		{"values with prefix ignored", Options{Values: true, IgnorePrefixes: []string{"NEXT_"}},
			[]string{"LOCAL_ONLY"}, []string{"NEW"}, []string{"A"}, 2},
		{"everything", Options{Values: true, OnlyMissing: true, Ignore: []string{"A", "LOCAL_ONLY"}, IgnorePrefixes: []string{"NEXT_PUBLIC_"}},
			[]string{}, []string{}, nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Compare(ref, target, tt.opts)
			if !reflect.DeepEqual(r.Missing, tt.missing) {
				t.Errorf("Missing = %q, want %q", r.Missing, tt.missing)
			}
			if !reflect.DeepEqual(r.Extra, tt.extra) {
				t.Errorf("Extra = %q, want %q", r.Extra, tt.extra)
			}
			var changed []string
			for _, c := range r.Changed {
				changed = append(changed, c.Key)
			}
			if !reflect.DeepEqual(changed, tt.changed) {
				t.Errorf("Changed = %q, want %q", changed, tt.changed)
			}
			if r.Common != tt.common {
				t.Errorf("Common = %d, want %d", r.Common, tt.common)
			}
		})
	}
}

func TestIgnored(t *testing.T) {
	o := Options{Ignore: []string{"PORT"}, IgnorePrefixes: []string{"VITE_", "NEXT_PUBLIC_"}}
	for key, want := range map[string]bool{
		"PORT":            true,
		"PORT2":           false,
		"VITE_API":        true,
		"NEXT_PUBLIC_URL": true,
		"NEXT_URL":        false,
		"vite_api":        false,
	} {
		if got := o.Ignored(key); got != want {
			t.Errorf("Ignored(%q) = %v, want %v", key, got, want)
		}
	}
}
