# envdiff

Compare `.env` files and exit nonzero when their keys don't match.

`.env.example` drifts. Someone adds `STRIPE_WEBHOOK_SECRET` to their local `.env`,
forgets the example file, and the next person to clone the repo gets a confusing
crash on startup. `envdiff` catches that before it ships.

## Install

```sh
go install github.com/dan2756/envdiff@latest
```

## Usage

```sh
envdiff .env .env.example
```

```
- REDIS_URL     missing in .env.example
+ LEGACY_TOKEN  only in .env.example
```

Only keys are compared, since example files usually hold placeholder values.
To compare values too, pass `--values`. Values are masked so the output is safe
to paste into a CI log: long values keep two characters at each end, short ones
are redacted completely. `--show-values` prints them in full.

```sh
envdiff --values .env.staging .env.production
```

```
~ DATABASE_URL  differs: .env.staging=po****ng .env.production=po****od
~ DEBUG         differs: .env.staging=<redacted> .env.production=<redacted>
```

Exit status is `0` when the files define the same keys, `1` when they differ and
`2` on errors (unreadable file, syntax error).

### More than two files

Pass any number of files; each one is compared against the first. Instead of a
list you get a matrix of the keys that aren't present everywhere:

```sh
envdiff .env.example .env.staging .env.production
```

```
KEY            .env.example  .env.staging  .env.production
SENTRY_DSN     ✓             ✓             -
FEATURE_FLAGS  -             ✓             -

2 of 23 keys differ across 3 files
```

With `--values`, a `~` marks a value that differs from the first file's, and the
masked values are listed below the matrix.

### Filtering

```sh
# keys that are fine to differ
envdiff --ignore PORT,LOG_LEVEL .env .env.example

# public build-time variables live elsewhere
envdiff --ignore-prefix NEXT_PUBLIC_ --ignore-prefix VITE_ .env .env.example

# only care that production has everything the example lists
envdiff --only-missing .env.example .env.production
```

`--ignore` and `--ignore-prefix` take comma-separated lists and can be repeated.
`--only-missing` drops the `+` lines: keys the other file has that the first
one doesn't.

### Strict mode

`--strict` also checks every file for keys assigned more than once, keys with an
empty value and keys that aren't `UPPER_SNAKE_CASE`. Any finding makes the exit
status `1`.

```
! DEBUG      duplicate in .env (lines 4, 31)
! log_level  not UPPER_SNAKE_CASE in .env (line 9)
! API_KEY    empty in .env (line 2)
```

Empty placeholders are common in example files, so you may want to run strict
checks on the real files only, or `--ignore` the keys that are allowed to be
blank.

## Output

The default output is a diff-style list, colored when stdout is a terminal.
Set `NO_COLOR` to turn color off.

| Line | Meaning |
| --- | --- |
| `- KEY  missing in OTHER` | the first file has `KEY`, the other doesn't |
| `+ KEY  only in OTHER` | the other file has `KEY`, the first doesn't |
| `~ KEY  differs: ...` | both have `KEY` with different values (`--values`) |
| `! KEY  ...` | a `--strict` finding |

`--json` prints a machine-readable report instead:

```json
{
  "reference": ".env",
  "files": [".env", ".env.example"],
  "match": false,
  "comparisons": [
    {
      "file": ".env.example",
      "match": false,
      "missing": ["REDIS_URL"],
      "extra": ["LEGACY_TOKEN"],
      "differs": []
    }
  ],
  "issues": []
}
```

`issues` holds `--strict` findings as `{"file", "key", "kind", "lines"}` where
`kind` is `duplicate`, `empty` or `name`. Values in `differs` are masked the same way as in the text output unless
`--show-values` is given. `-q`/`--quiet` prints nothing and only sets the exit
status; errors still go to stderr.

## Syntax

envdiff has its own dotenv parser and understands what you'd expect to find in
the wild:

```sh
# full-line comments
PLAIN=value                  # inline comment (needs a space before '#')
export EXPORTED=1
SPACED = around the equals
COLOR=#fff                   # '#' with no space before it is part of the value
SINGLE='literal $HOME \n'
DOUBLE="escapes: \n \t \" \\"
BACKTICK=`it's "quoted"`
PRIVATE_KEY="-----BEGIN KEY-----
spans several lines
-----END KEY-----"
```

CRLF line endings and a UTF-8 byte order mark are fine. Syntax errors are
reported as `file:line: message`.

### Variable expansion

By default values are compared exactly as written, so `${HOST}` is just text.
With `--expand`, references in unquoted and double-quoted values are resolved
first:

```sh
HOST=db.internal
DATABASE_URL=postgres://${HOST}:${PORT:-5432}/app
CACHE_DIR=$HOME/.cache
LITERAL="costs \$5"         # \$ is a literal dollar sign
SINGLE='${NOT_EXPANDED}'    # single quotes and backticks are never expanded
```

`$VAR`, `${VAR}`, `${VAR:-default}` (unset or empty) and `${VAR-default}`
(unset) are supported. A variable is looked up among the keys defined earlier
in the same file, then in the process environment; anything else expands to an
empty string.

## License

MIT
