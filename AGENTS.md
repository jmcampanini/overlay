## Build and validate

- Use `make` — do not invoke `go build` / `go test` / `golangci-lint` directly.
- Run `make help` to discover the task list.
- Use these key tasks:
  - Run `make build` to compile to `build/overlay`.
  - Run `make test` to execute `go test -race ./...`.
  - Run `make check` to execute `fmt-check` + `tidy-check` + `lint` + `test`. **Run this before declaring work done.**

## Conventions

- The binary is built to `build/overlay`, not the repo root. `./build/` is gitignored.
- Scratch/smoke output goes under `.claude-sandbox/<scenario>/` — not `/tmp/`. This directory is auto-created and gitignored.
- Generated fixture output goes under `testdata/fixtures/*/.generated/`, which is gitignored.
- Keep production `.go` files free of references to prior implementations or legacy scripts. Tests, `.claude-sandbox/`, and `.claude-plans/` can reference them.
- Skip comments in `.go` files unless explaining why a non-obvious choice was necessary.

## Before committing

- Always run `make check`. It is the single source of truth for "this is ready".
