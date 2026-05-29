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

Exit status is `0` when both files define the same keys, `1` when they differ and
`2` on errors (unreadable file, syntax error).

## License

MIT
