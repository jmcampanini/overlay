# PLAN: `overlay orphans` — detect stale rendered targets (issue #46)

The scenario: two machines share a dotfiles repo, each rendering with its own
profiles. A source stops producing a target (deleted, renamed, removed from
`sources`, or profile-inactive), the other machine pulls and renders, and the
previously rendered file is left on disk with nothing pointing at it. The
current checkout contains no evidence the file was ever overlay's, so detection
needs memory: a per-machine manifest of what render wrote.

This plan is the output of a settled design interview. Do not re-litigate the
decisions below; implement them.

## Design decisions (settled)

| # | Decision |
|---|---|
| 1 | Mechanism: per-machine ownership manifest maintained by `render`. Rejected: git-history comparison, committed remove-list, xattr/marker stamps. |
| 2 | Always-on. No config knob; render maintains the manifest unconditionally. |
| 3 | State file: fixed name `.overlay.state.json`, always next to the config file (config file's directory, else CWD — the same anchor `resolve()` uses for relative paths). Not configurable. |
| 4 | State model: cumulative registry of overlay's surviving outputs — render merges entries, never replaces the set, so `pull → render → orphans` works in any order. |
| 5 | Entry schema: absolute target path + absolute owning-source dir. Nothing else — no hashes, no timestamps, no layers. |
| 6 | Detection surfaces only through `overlay orphans`. Render's observable behavior is unchanged (no warnings, same output, same exit codes) apart from maintaining the state file. |
| 7 | `orphans` is strictly read-only: it never writes or GCs state. GC happens only inside render's save. |
| 8 | Narrowing (strict scoping): with positional/flag/env source narrowing, judge only entries owned by selected sources; unnarrowed runs judge every entry, including removed-from-config sources. |
| 9 | Exit codes mirror `diff`: `0` no orphans, `1` orphans found, `2` resolution/state/I-O failure. |
| 10 | Scope: detection only. No deletion, no `clean`, no future-proofing fields. |

## The invariant

An entry in `.overlay.state.json` means:

> "Overlay successfully wrote this target at some point, and a regular file
> still existed there the last time state was saved."

Orphan predicate:

```
orphan(entry) = entry is judged (narrowing rule)
              ∧ a regular file exists at entry.target (os.Lstat, mode regular)
              ∧ entry.target ∉ current active plan's target paths
```

A directory or symlink now sitting at a recorded path means overlay's artifact
is gone: detect skips it, and render's GC drops the entry.

## State file

Location: `filepath.Join(configBase, ".overlay.state.json")` where
`configBase` comes from `configBaseFromReport` (loaded config file's directory,
else `.`). Shape (entries sorted by `target`, 2-space indent, trailing
newline, perms 0o644):

```json
{
  "entries": [
    {
      "target": "/Users/x/.config/gibson/config.toml",
      "source": "/Users/x/dotfiles/gibson"
    }
  ]
}
```

- `target`: `Group.TargetPath`. `source`: `Group.SourceDir`. Entries are keyed
  by `target`; a later render of the same target from a different source
  re-keys the entry.
- A present file with an empty `entries` list is a valid, initialized baseline.
- Corrupt/unparseable JSON is a hard error naming the file and the remedy
  (delete it and re-render to re-baseline). Never silently rebuild.
- Save is atomic: temp file in the same directory, `os.Rename` over the target.
  No fsync — a crash leaves the old file intact and the next successful render
  re-claims its writes.

## New/changed code

### 1. `internal/state` (new package)

```go
type Entry struct { Target, Source string }
Load(path string) ([]Entry, error)   // missing file → distinguishable sentinel
Save(path string, entries []Entry) error
Merge(prior, claimed []Entry) []Entry
```

- `Merge`: prior map keyed by target; claimed entries overwrite/add.
- GC lives in `Save` (or a small helper it calls): drop any entry whose target
  is not currently a regular file, checked at save time.

### 2. `cmd/resolve.go`

- `Resolved` gains `StatePath string` (computed from the config base, always).
- `Resolved` gains `SourcesNarrowed bool`: true when positionals were given
  **or** the effective `sources` value came from flag/env rather than the
  config file/defaults (the load report's provenance already knows).

### 3. `render` integration (`internal/render/render.go`)

`Options` gains `StatePath string`. In `Run`:

1. `state.Load` at the start. Missing file → empty prior. Corrupt/unreadable →
   hard error before anything is composed or written.
2. Collect a `state.Entry` for every **successful** write.
3. Every run that gets past discovery and fail-fast composition saves state —
   including zero-group runs and error returns from the write loop. Save =
   `Merge(prior, claimed)` → GC → atomic write, guaranteed on every return
   path once writing can begin.
4. Runs that never get there (discovery error, fail-fast compose failure)
   leave state untouched.
5. A failed state save after files were written is a render error (non-zero).
   Never swallow it — a stale manifest silently poisons future detection.

Lifecycle (test these):

| Run outcome | Files written | State |
|---|---|---|
| Full success | all | claim all, GC, save |
| Zero groups discovered | none | claim nothing, GC, save |
| Compose failure, fail-fast | none | untouched |
| Compose failures + `--continue` | clean subset | claim subset; failed groups keep prior entries; GC; save |
| Write error mid-loop | prefix | claim prefix; rest keep prior entries; GC; save |
| Crash mid-render | some | old file intact (atomic rename); next successful render re-claims |

### 4. Shared exit-code type (`cmd/`)

Generalize `DiffExitCode` (`cmd/diff.go:14`) into one shared type used by
`diff` and `orphans`; update the `errors.As` in `main.go`. Internal rename,
fully updated in this change. `diff` behavior must not change.

### 5. `internal/orphans` (new package — pure predicate)

```go
type Orphan struct { Target, Source string }
type Options struct {
    Entries         []state.Entry
    PlanTargets     map[string]struct{} // active groups' TargetPath set
    SelectedSources map[string]struct{} // effective absolute source dirs
    Narrowed        bool
}
Detect(opts Options) []Orphan
```

Judged = `!Narrowed`, or `entry.Source ∈ SelectedSources`. Orphan = judged ∧
regular file exists ∧ target ∉ PlanTargets. Results sorted by target. No
printing, no file reads beyond `Lstat`.

### 6. `cmd/orphans.go` (new command)

Mirror `cmd/diff.go`. `RunE`:

1. `resolve(...)`; failure → `overlay: <err>` on stderr, exit 2.
2. `state.Load`: missing → stderr message "no state file yet; run
   `overlay render` to establish the baseline", exit 2. Corrupt → exit 2.
3. `discover.WalkDetailed`; warn missing source dirs to stderr exactly like
   render/diff. Skip `render.WarnUnusedPins` — orphans composes nothing.
4. `orphans.Detect(...)`.
5. Print each orphan's absolute target path, one per line, to stdout. Nothing
   else on stdout; never file contents. Diagnostics via the logger (stderr),
   respecting `--quiet`/`--verbose`.
6. Orphans found → exit 1. None → exit 0.

Register in `cmd/root.go`. Help text: reuse `sourceSelectionHelp` +
`profilePrecedenceHelp`, plus an exit-code block modeled on `diffOutputHelp`.

### 7. Docs

- README: an "Orphan detection" section — the state file and why it exists,
  recommend gitignoring it, the command, exit codes, the
  `overlay orphans | xargs rm` interim pattern, and the baseline caveat
  (files rendered before this feature exist are invisible until re-rendered).
- No `.overlay.toml` schema changes, so `overlay docs` is untouched.

## Behavioral edge cases (encode in tests)

1. Delete a complete source group after a render → path printed, exit 1.
2. Switch to a profile with no active layers for a rendered group → reported.
3. Rename a stem / relocate a mapped target → old target reported, new one not.
4. Manually remove an orphan → no longer reported; next render GCs the entry.
5. Unrelated pre-existing file under the target root → never reported.
6. Narrowed run: entries of unselected sources never reported; a removed
   source can still be targeted explicitly by its old path.
7. Unnarrowed run reports removed-from-config source orphans.
8. `--continue` / partial failures follow the lifecycle table.
9. No state file → baseline message, exit 2, empty stdout.
10. Entry whose path is now a directory or symlink → skipped by detect,
    dropped by render GC.
11. Same target later produced by a different source → entry re-keyed, not an
    orphan.
12. Config `target` changed → entries under the old absolute root are judged
    and reported (absolute paths make this work).
13. Multiple configs sharing a target root → independent state files next to
    each config; no cross-talk.

## Tests

Repo conventions: table-style in-package tests with `t.TempDir()`, silent
loggers, per-package helpers; command-level tests build the real root command
(copy the `runDiff` pattern from `cmd/diff_test.go`). One owner per behavior
at the lowest faithful layer:

- `internal/state`: load/save round-trip, sorted deterministic output, atomic
  rename (no temp leftover), corrupt JSON → error, missing-file sentinel,
  merge overwrite-by-target, GC of gone/dir/symlink entries.
- `internal/render`: the lifecycle table — success claims all; zero-group runs
  save; fail-fast leaves state byte-identical; `--continue` claims the clean
  subset and keeps prior entries for failed groups; state saved before the
  error return on a write failure; save failure surfaces as a render error.
- `internal/orphans`: predicate matrix — orphan / in-plan / missing file /
  dir-at-path / symlink-at-path / narrowing on and off / removed source.
- `cmd/orphans_test.go`: exit codes 0/1/2, uninitialized message, stdout is
  exactly the sorted path list, render-then-mutate flows through the real
  root command.

Run everything through `make` (never `go test` directly).

## End-to-end verification (final gate — agent-verified)

Prove the two-laptop story with the real binary, then run the full check suite:

1. `make build` → `build/overlay`.
2. Under `.claude-sandbox/orphans-e2e/`, create a source tree with
   `.overlay.toml` (`target` pointing at a sibling `target/` dir, a profile)
   and sources covering a unique base-layer file, a profile-only file, and two
   source roots.
3. Exercise and assert (stdout paths, stderr messages, exit codes via `$?`):
   a. render → `orphans` exits 0, empty stdout, `.overlay.state.json` exists.
   b. delete one complete source group → render → `orphans` prints exactly
      that target, exits 1 (order-independence: also verify `orphans` alone,
      without the intervening render, reports the same).
   c. `orphans <other-source>` (narrowed) → exits 0, no output.
   d. switch profiles so a group goes inactive → its target reported.
   e. `rm` the orphan → `orphans` exits 0; render; state no longer contains
      the entry (inspect JSON).
   f. drop an unrelated file into `target/` → never reported.
   g. fresh dir with no state file → exit 2 with the baseline message.
4. `make check` — the single source of truth for done.

If any assertion cannot be automated, stop and ask the user to verify manually
rather than skipping it.
