package dotenv

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type kv struct{ Key, Value string }

func pairs(f *File) []kv {
	out := []kv{}
	for _, e := range f.Entries {
		out = append(out, kv{e.Key, e.Value})
	}
	return out
}

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []kv
	}{
		{"empty file", "", []kv{}},
		{"simple", "FOO=bar", []kv{{"FOO", "bar"}}},
		{"empty value", "FOO=", []kv{{"FOO", ""}}},
		{"trailing newline", "FOO=bar\n", []kv{{"FOO", "bar"}}},
		{"export", "export FOO=bar", []kv{{"FOO", "bar"}}},
		{"export with tab", "export\tFOO=bar", []kv{{"FOO", "bar"}}},
		{"key named export", "export=1", []kv{{"export", "1"}}},
		{"key starting with export", "exporter=otlp", []kv{{"exporter", "otlp"}}},
		{"comment lines", "# top\nFOO=bar\n  # indented\n", []kv{{"FOO", "bar"}}},
		{"blank lines", "\n\n  \nFOO=bar\n\n", []kv{{"FOO", "bar"}}},
		{"spaces around equals", "FOO = bar", []kv{{"FOO", "bar"}}},
		{"tabs around equals", "FOO\t=\tbar", []kv{{"FOO", "bar"}}},
		{"leading whitespace", "   FOO=bar", []kv{{"FOO", "bar"}}},
		{"trailing whitespace", "FOO=bar  \t", []kv{{"FOO", "bar"}}},
		{"inner spaces kept", "GREETING=hello  world", []kv{{"GREETING", "hello  world"}}},
		{"equals in value", "DSN=a=b=c", []kv{{"DSN", "a=b=c"}}},
		{"inline comment", "FOO=bar # the bar", []kv{{"FOO", "bar"}}},
		{"inline comment after tab", "FOO=bar\t# the bar", []kv{{"FOO", "bar"}}},
		{"hash without space is value", "COLOR=#fff", []kv{{"COLOR", "#fff"}}},
		{"hash inside value", "URL=http://x/a#frag", []kv{{"URL", "http://x/a#frag"}}},
		{"only comment after equals", "FOO= # set me", []kv{{"FOO", ""}}},
		{"apostrophe in unquoted", "NAME=it's", []kv{{"NAME", "it's"}}},
		{"unquoted keeps backslashes", `PATTERN=a\nb`, []kv{{"PATTERN", `a\nb`}}},
		{"single quoted", "FOO='bar baz'", []kv{{"FOO", "bar baz"}}},
		{"single quoted is literal", `FOO='a\nb $HOME "x"'`, []kv{{"FOO", `a\nb $HOME "x"`}}},
		{"single quoted keeps hash", "FOO='a # b'", []kv{{"FOO", "a # b"}}},
		{"single quoted keeps spaces", "FOO='  padded  '", []kv{{"FOO", "  padded  "}}},
		{"double quoted", `FOO="bar baz"`, []kv{{"FOO", "bar baz"}}},
		{"double quoted escapes", `FOO="a\nb\tc\\d\"e"`, []kv{{"FOO", "a\nb\tc\\d\"e"}}},
		{"double quoted carriage return", `FOO="a\rb"`, []kv{{"FOO", "a\rb"}}},
		{"unknown escape kept", `FOO="a\qb"`, []kv{{"FOO", `a\qb`}}},
		{"double quoted keeps hash", `FOO="a # b" # comment`, []kv{{"FOO", "a # b"}}},
		{"comment right after quote", `FOO="bar"#c`, []kv{{"FOO", "bar"}}},
		{"empty double quotes", `FOO=""`, []kv{{"FOO", ""}}},
		{"empty single quotes", `FOO=''`, []kv{{"FOO", ""}}},
		{"backtick", "FOO=`it's \"quoted\"`", []kv{{"FOO", `it's "quoted"`}}},
		{"backtick is literal", "FOO=`a\\nb`", []kv{{"FOO", `a\nb`}}},
		{"export and quotes", `export FOO = "x y"`, []kv{{"FOO", "x y"}}},
		{"dots and dashes in keys", "spring.profile-name=dev", []kv{{"spring.profile-name", "dev"}}},
		{"lowercase key", "foo=1", []kv{{"foo", "1"}}},
		{"duplicate keys kept in order", "A=1\nB=2\nA=3", []kv{{"A", "1"}, {"B", "2"}, {"A", "3"}}},
		{"multiline double quoted", "KEY=\"-----BEGIN-----\nabc\n-----END-----\"\nNEXT=1",
			[]kv{{"KEY", "-----BEGIN-----\nabc\n-----END-----"}, {"NEXT", "1"}}},
		{"multiline with escapes", "MSG=\"line one\\n\nline \\\"two\\\"\"", []kv{{"MSG", "line one\n\nline \"two\""}}},
		{"multiline single quoted", "A='x\ny'", []kv{{"A", "x\ny"}}},
		{"multiline backtick", "A=`x\n# not a comment\ny`", []kv{{"A", "x\n# not a comment\ny"}}},
		{"crlf", "A=1\r\nB=two words\r\n# c\r\nC='q'\r\n", []kv{{"A", "1"}, {"B", "two words"}, {"C", "q"}}},
		{"crlf inline comment", "A=1 # c\r\nB=2\r\n", []kv{{"A", "1"}, {"B", "2"}}},
		{"crlf multiline", "A=\"x\r\ny\"\r\n", []kv{{"A", "x\ny"}}},
		{"cr only", "A=1\rB=2\r", []kv{{"A", "1"}, {"B", "2"}}},
		{"crlf last line without lf", "A=1\r\nB=2\r", []kv{{"A", "1"}, {"B", "2"}}},
		{"cr multiline", "A=\"x\ry\"\rB=2", []kv{{"A", "x\ny"}, {"B", "2"}}},
		{"bom", "\uFEFFA=1\nB=2", []kv{{"A", "1"}, {"B", "2"}}},
		{"bom and crlf", "\uFEFFexport A=1\r\n", []kv{{"A", "1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse("test.env", []byte(tt.in))
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if got := pairs(f); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		line int
		msg  string
	}{
		{"missing equals", "FOO", 1, "expected '=' after key"},
		{"missing equals later", "A=1\n\nB", 3, "expected '=' after key"},
		{"export without assignment", "export FOO", 1, "expected '=' after key"},
		{"space in key", "MY KEY=1", 1, "expected '=' after key"},
		{"key starting with digit", "1FOO=bar", 1, "invalid key: must start with a letter or '_'"},
		{"no key", "=bar", 1, "expected KEY=value"},
		{"quoted key", `"FOO"=bar`, 1, "expected KEY=value"},
		{"unterminated double", `FOO="bar`, 1, "unterminated double-quoted value"},
		{"unterminated single", "A=1\nFOO='bar", 2, "unterminated single-quoted value"},
		{"unterminated backtick", "FOO=`bar", 1, "unterminated backtick-quoted value"},
		{"text after quote", `FOO="bar" baz`, 1, `unexpected text after closing "`},
		{"unterminated multiline", "A=1\nB=\"start\nmore\nC=3\n", 2, "unterminated double-quoted value"},
		{"error after multiline value", "A=\"x\ny\nz\"\nBAD LINE\n", 4, "expected '=' after key"},
		{"crlf error line", "A=1\r\nB=2\r\noops\r\n", 3, "expected '=' after key"},
		{"cr error line", "A=1\rB\r", 2, "expected '=' after key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse("test.env", []byte(tt.in))
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("Parse(%q) error = %v, want *ParseError", tt.in, err)
			}
			if pe.Line != tt.line {
				t.Errorf("line = %d, want %d", pe.Line, tt.line)
			}
			if !strings.Contains(pe.Msg, tt.msg) {
				t.Errorf("msg = %q, want it to contain %q", pe.Msg, tt.msg)
			}
			if prefix := "test.env:"; !strings.HasPrefix(err.Error(), prefix) {
				t.Errorf("Error() = %q, want prefix %q", err.Error(), prefix)
			}
		})
	}
}

// A malformed line is often part of a value, such as an unquoted
// multi-line private key, so errors must not quote the input.
func TestParseErrorsDontQuoteInput(t *testing.T) {
	tests := []struct {
		in     string
		expand bool
		secret string
	}{
		{"PRIVATE_KEY=-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA3Tz2mr7SZiAMfQyuvBjM4Oi/x\n-----END RSA PRIVATE KEY-----\n", false, "MIIEpAIBAAKC"},
		{"PRIVATE_KEY=-----BEGIN RSA PRIVATE KEY-----\n3Tz2mr7SZiAMfQyuvBjM4Oi\n", false, "3Tz2mr7SZ"},
		{"PRIVATE_KEY=-----BEGIN RSA PRIVATE KEY-----\n+Tz2mr7SZiAMfQyuvBjM4Oi\n", false, "Tz2mr7SZ"},
		{`"API_KEY"=sk_live_51Habcdefghijklmnop`, false, "sk_live"},
		{`PASSWORD="hunter2"supersecret`, false, "supersecret"},
		{"PASSWORD=pa${$s:-w0rd}", true, "w0rd"},
	}
	for _, tt := range tests {
		_, err := Options{Expand: tt.expand}.Parse("test.env", []byte(tt.in))
		if err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", tt.in)
			continue
		}
		if strings.Contains(err.Error(), tt.secret) {
			t.Errorf("Parse(%q) error %q quotes the input", tt.in, err)
		}
	}
}

func TestEntryLines(t *testing.T) {
	f, err := Parse("test.env", []byte("A=1\n\n# comment\nB=2\n  C=3\n"))
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, e := range f.Entries {
		got = append(got, e.Line)
	}
	if want := []int{1, 4, 5}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %v, want %v", got, want)
	}
}

func TestEntryLinesAfterMultiline(t *testing.T) {
	f, err := Parse("test.env", []byte("A=\"one\ntwo\nthree\"\nB=2\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Entries[1].Line; got != 4 {
		t.Errorf("B on line %d, want 4", got)
	}
}

func TestLookupLastWins(t *testing.T) {
	f, err := Parse("test.env", []byte("A=1\nB=2\nA=3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := f.Lookup("A"); !ok || v != "3" {
		t.Errorf("Lookup(A) = %q, %v; want \"3\", true", v, ok)
	}
	if _, ok := f.Lookup("C"); ok {
		t.Error("Lookup(C) found a value for an unset key")
	}
	if got, want := f.Keys(), []string{"A", "B"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
}

func TestParseExpand(t *testing.T) {
	env := map[string]string{"HOME": "/home/me", "EMPTY": ""}
	opts := Options{Expand: true, LookupEnv: func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}}
	tests := []struct {
		name string
		in   string
		want string // value of the last entry
	}{
		{"braces", "A=x\nB=${A}y", "xy"},
		{"bare", "A=x\nB=$A/y", "x/y"},
		{"bare stops at non-name char", "A=x\nB=$A-$A.$A", "x-x.x"},
		{"double quoted", "A=x\nB=\"${A} $A\"", "x x"},
		{"multiline double quoted", "A=x\nB=\"1\n${A}\n2\"", "1\nx\n2"},
		{"single quoted untouched", "A=x\nB='${A} $A'", "${A} $A"},
		{"backtick untouched", "A=x\nB=`$A`", "$A"},
		{"escaped in double quotes", "A=x\nB=\"\\${A} \\$A\"", "${A} $A"},
		{"escaped unquoted", "A=x\nB=\\$A", "$A"},
		{"from environment", "H=${HOME}/x", "/home/me/x"},
		{"file before environment", "HOME=/srv\nH=$HOME", "/srv"},
		{"self reference uses environment", "HOME=$HOME/sub", "/home/me/sub"},
		{"later assignment wins", "A=1\nA=2\nB=$A", "2"},
		{"defined later is not visible", "B=[$A]\nA=1\nC=$B", "[]"},
		{"undefined is empty", "B=[${NOPE}]", "[]"},
		{"default when unset", "B=${NOPE:-fallback}", "fallback"},
		{"default when empty", "B=${EMPTY:-fallback}", "fallback"},
		{"dash default ignores empty", "B=[${EMPTY-fallback}]", "[]"},
		{"dash default when unset", "B=${NOPE-fallback}", "fallback"},
		{"default may contain spaces", "B=\"${NOPE:-a b}\"", "a b"},
		{"trailing dollar", "PRICE=5$", "5$"},
		{"dollar before space", "B=a $ b", "a $ b"},
		{"dollar before digit", "B=$1", "$1"},
		{"comment after reference", "A=x\nB=$A # $NOPE", "x"},
		{"dash default may contain :-", "B=${NOPE-a:-b}", "a:-b"},
		{"dash default not used when set", "A=set\nB=${A-a:-b}", "set"},
		{"colon default may contain :-", "B=${NOPE:-a:-b}", "a:-b"},
		{"nested references in default", "HOST=db.internal\nPORT=5432\nB=${URL:-postgres://${HOST}:${PORT}/app}", "postgres://db.internal:5432/app"},
		{"nested default of default", "HOST=db\nB=${URL:-postgres://${HOST}:${PORT:-5432}/app}", "postgres://db:5432/app"},
		{"nested reference in double quotes", "HOST=db\nB=\"x${URL:-postgres://${HOST}/app}y\"", "xpostgres://db/appy"},
		{"set value skips the default", "URL=keep\nB=x${URL:-postgres://${MISSING}/app}y", "xkeepy"},
		{"escaped dollar does not nest", `B=${NOPE:-\${HOST}tail}`, "${HOSTtail}"},
		{"escaped dollar in default", `B=${NOPE:-cost \$5}`, "cost $5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := opts.Parse("test.env", []byte(tt.in))
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if got := f.Entries[len(f.Entries)-1].Value; got != tt.want {
				t.Errorf("Parse(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseNoExpandByDefault(t *testing.T) {
	f, err := Parse("test.env", []byte("A=x\nB=${A}\nC=\"$A\"\nD=\"\\$A\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []kv{{"A", "x"}, {"B", "${A}"}, {"C", "$A"}, {"D", "$A"}}
	if got := pairs(f); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseExpandErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		line int
		msg  string
	}{
		{"unterminated", "A=1\nB=${A", 2, "unterminated ${"},
		{"unterminated in quotes", "B=\"${A\nmore\"", 1, "unterminated ${"},
		{"invalid name", "B=${1A}", 1, "invalid variable reference"},
		{"empty name", "B=${}", 1, "invalid variable reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Options{Expand: true}.Parse("test.env", []byte(tt.in))
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("error = %v, want *ParseError", err)
			}
			if pe.Line != tt.line || !strings.Contains(pe.Msg, tt.msg) {
				t.Errorf("got line %d %q, want line %d containing %q", pe.Line, pe.Msg, tt.line, tt.msg)
			}
		})
	}
	// Without --expand the same input is just a literal value.
	if _, err := Parse("test.env", []byte("B=${A")); err != nil {
		t.Errorf("unexpanded parse failed: %v", err)
	}
}
