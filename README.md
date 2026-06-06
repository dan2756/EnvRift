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

Exit status is `0` when both files define the same keys, `1` when they differ and
`2` on errors (unreadable file, syntax error).

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

## License

MIT
