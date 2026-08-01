# Stateless renders and JSON orphan output

## Goal

Add two opt-in CLI capabilities needed by callers that render disposable output and safely automate orphan handling:

1. `overlay render --no-state` renders normally without reading, validating, creating, modifying, or collision-checking `.overlay.state.json`.
2. `overlay orphans --json` writes the detected absolute target paths as a JSON array while preserving the existing exit-code contract.

Both flags are additive. Existing invocations and default output remain unchanged.

## Decisions

### `render --no-state`

- Make `--no-state` a render-only invocation flag, not a config field or environment variable. Statelessness is an explicit property of one disposable render and should not be persisted accidentally.
- Preserve discovery, composition, substitution, `--continue`, target writes, logging, and render exit behavior.
- Bypass the entire ownership-state lifecycle:
  - Do not require or resolve a usable state path for rendering behavior.
  - Do not load or validate existing state.
  - Do not check rendered targets for collisions with the state path.
  - Do not construct ownership claims solely for persistence.
  - Do not create, garbage-collect, or save state, including zero-group and partial-success runs.
- A malformed existing state file must not prevent a stateless render, and the file must remain byte-identical.
- Keep normal stateful rendering as the default.

### `orphans --json`

- Make `--json` an orphans-only output flag.
- Emit a top-level JSON array of absolute path strings in the same deterministic order as the existing text output.
- Always emit valid JSON on successful detection:
  - No findings: `[]` plus a trailing newline, exit `0`.
  - Findings: populated array plus a trailing newline, exit `1`.
- Preserve operational failures as exit `2`, with diagnostics on stderr and no partial JSON on stdout.
- Keep the default one-path-per-line output unchanged.
- Encode paths with Go's JSON encoder so embedded newlines, quotes, backslashes, and Unicode remain unambiguous.

## Implementation outline

1. **Render CLI flag**
   - Add the command-local `--no-state` flag in `cmd/render.go`.
   - Pass the selected mode explicitly into `render.Options`; avoid using an empty state path as an implicit mode switch.
   - Extend render help text to explain that output is written normally but ownership state is untouched.

2. **Render orchestration**
   - Refactor `internal/render.Run` so state setup, collision checks, claim construction, and state saves are conditional on state maintenance.
   - Keep shared composition and write paths identical between stateful and stateless renders.
   - Ensure every success and failure branch returns directly in stateless mode rather than entering state-save behavior.

3. **Orphan JSON flag**
   - Add the command-local `--json` flag in `cmd/orphans.go`.
   - Complete detection before writing stdout.
   - Convert the sorted orphan results to a non-nil `[]string`, then encode the complete array once.
   - Preserve the existing text writer when the flag is absent and preserve exit `1` after either output format reports findings.

4. **Help and upstream documentation**
   - Update `cmd/helptext.go` and command help to document both orphan output formats and their unchanged exit codes.
   - Update README examples for disposable stateless renders and JSON orphan consumption.

## Tests

These tests protect real CLI contracts at the layers closest to likely defects:

- **Render command tests:** prove `--no-state` is render-scoped, accepted by `render`, and leaves the sidecar absent or byte-identical while still writing targets.
- **Render orchestration tests:** prove stateless rendering skips malformed-state loading, state-path collision checks, zero-group initialization, partial-success claims, and final state saves without changing ordinary rendering behavior.
- **Orphans command tests:** prove `--json` emits `[]` with exit `0`, emits sorted paths with exit `1`, safely round-trips unusual path characters, leaves stdout empty on exit `2`, and does not alter default line output.
- Keep each behavior owned at one primary layer; do not duplicate the full state or orphan detection matrices.

## Non-goals

- No `overlay prune` or deletion behavior.
- No configurable state path.
- No global JSON mode or changes to other commands.
- No changes to the default stateful render or line-oriented orphan output.
- No dotfiles-repository changes in this worktree.

## Agent-verified end-to-end workflow

1. Run `make check`.
2. Run `make build` and use `build/overlay` for all smoke checks.
3. Under `.claude-sandbox/stateless-json-smoke/`, create an isolated config, source layer, normal target, and disposable generated target.
4. Run a normal render to establish state; record the state file's checksum and contents.
5. Render the disposable target with `--no-state`; verify the output exists, the operational state checksum is unchanged, and no disposable target claim appears in state.
6. Replace the state file temporarily with malformed JSON, run another `--no-state` render, and verify rendering succeeds without changing that malformed file.
7. Restore a valid baseline, remove or rename a rendered source, and run `orphans --json`; capture exit `1`, parse stdout as JSON, and verify the exact sorted absolute orphan paths.
8. Remove the orphan target and rerun `orphans --json`; verify stdout parses as `[]` and the command exits `0`.
9. Run default `orphans` and default `render` once more to verify their existing text output and stateful behavior remain unchanged.
10. Run `make check` again after the smoke workflow.
