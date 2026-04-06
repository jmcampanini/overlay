# CLAUDE.md

Guidance for Claude Code when working in this repo. Keep it short.

## Build and validate

Use `make` — do not invoke `go build` / `go test` / `golangci-lint` directly.

| command | purpose |
|---|---|
| `make build` | Compile the CLI to `build/overlay`. |
| `make test` | Run all Go unit and integration tests (`go test ./...`). |
| `make test-race` | Same as `test` but with the race detector. |
| `make cover` | Run tests with per-package coverage. |
| `make vet` | `go vet ./...`. |
| `make lint` | Run `golangci-lint` with the config in `.golangci.yml`. |
| `make lint-fix` | Same as `lint` with `--fix` (applies safe autofixes). |
| `make fmt` | `gofmt -w .`. |
| `make fmt-check` | Fail if any file needs `gofmt`. |
| `make check` | `fmt-check` + `vet` + `lint` + `test`. **Run this before declaring work done.** |
| `make clean` | Remove `build/`, `coverage.out`, `coverage.html`. |
| `make parity` | Build-tag–gated parity test against the reference Python script. |

## Conventions

- The binary is built to `build/overlay`, not the repo root. `./build/` is gitignored.
- Scratch/smoke output goes under `.claude-sandbox/<scenario>/` — not `/tmp/`. This directory is auto-created and gitignored.
- Generated fixture output goes under `testdata/fixtures/*/.generated/`, which is gitignored.
- Keep production `.go` files free of references to prior implementations or legacy scripts. Tests, `.claude-sandbox/`, and `.claude-plans/` can reference them.
- Skip comments in `.go` files unless explaining why a non-obvious choice was necessary.

## Before committing

Always run `make check`. It is the single source of truth for "this is ready".
