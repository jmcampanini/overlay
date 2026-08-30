## Build and validate

- Use `make` - do not invoke `go build` / `go test` / `golangci-lint` directly.
- Run `make help` to discover the task list.
- Use these key tasks:
  - Run `make build` to compile to `build/overlay`.
  - Run `make test` to execute `go test -race ./...`.
  - Run `make check` to execute `fmt-check` + `tidy-check` + `lint` + `test`. **Run this before declaring work done.**

## Conventions

- The binary is built to `build/overlay`, not the repo root. `./build/` is gitignored.
- Generated fixture output goes under `testdata/fixtures/*/.generated/`, which is gitignored.

## Before committing

- Always run `make check`. It is the single source of truth for "this is ready".
