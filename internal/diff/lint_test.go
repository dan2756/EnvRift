package diff

import (
	"reflect"
	"testing"
)

func TestLint(t *testing.T) {
	f := parse(t, ".env", `# a file with problems
DATABASE_URL=postgres://localhost/app
api_key=abc
DEBUG=false
EMPTY=
QUOTED_EMPTY=""
DEBUG=true
Mixed_Case=1
dotted.key=1
dashed-key=1
SET_LATER=
SET_LATER=now
CLEARED_LATER=x
CLEARED_LATER=
NEXT_PUBLIC_EMPTY=
`)
	want := []Issue{
		{".env", "api_key", BadName, []int{3}},
		{".env", "DEBUG", Duplicate, []int{4, 7}},
		{".env", "EMPTY", Empty, []int{5}},
		{".env", "QUOTED_EMPTY", Empty, []int{6}},
		{".env", "Mixed_Case", BadName, []int{8}},
		{".env", "dotted.key", BadName, []int{9}},
		{".env", "dashed-key", BadName, []int{10}},
		{".env", "SET_LATER", Duplicate, []int{11, 12}},
		{".env", "CLEARED_LATER", Duplicate, []int{13, 14}},
		{".env", "CLEARED_LATER", Empty, []int{14}},
	}
	got := Lint(f, Options{IgnorePrefixes: []string{"NEXT_PUBLIC_"}})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lint()\n got %v\nwant %v", got, want)
	}

	if got := Lint(parse(t, "ok", "A=1\nB_2=x\n_PRIVATE=y\n"), Options{}); len(got) != 0 {
		t.Errorf("Lint() on a clean file = %v", got)
	}
}

func TestIsUpperSnake(t *testing.T) {
	for s, want := range map[string]bool{
		"FOO":          true,
		"FOO_BAR":      true,
		"FOO2":         true,
		"_FOO":         true,
		"S3_BUCKET":    true,
		"foo":          false,
		"Foo":          false,
		"FOO-BAR":      false,
		"FOO.BAR":      false,
		"2FOO":         false,
		"":             false,
		"DATABASE_URL": true,
	} {
		if got := isUpperSnake(s); got != want {
			t.Errorf("isUpperSnake(%q) = %v, want %v", s, got, want)
		}
	}
}
