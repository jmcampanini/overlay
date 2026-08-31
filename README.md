# overlay

overlay merges layered JSON, TOML, and YAML configuration files by profile and copies other profile-specific files through as whole files. It walks one or more source directories for files named `<stem>.olay.<profile>[.<ext>]`, groups them by target path, orders the active layers `base`, then each selected profile, then `local`, and renders one file per group into a target directory. It is built for dotfiles layouts where a base file, a machine profile, and a machine-local override compose into a single rendered file.

Command help is the canonical reference: `overlay --help` and each command's `--help` describe every user-facing contract, `overlay config --help` describes configuration precedence and the config report, `overlay help exit-codes` describes exit statuses, and `overlay docs` prints the `.overlay.toml` schema reference.

## Install

### Homebrew `--HEAD` (macOS)

```shell
brew tap jmcampanini/overlay https://github.com/jmcampanini/overlay
brew install --HEAD jmcampanini/overlay/overlay
```

Upgrade to the latest commit:

```shell
brew upgrade --fetch-HEAD overlay
```

### From source

```shell
make build
# binary at ./build/overlay - copy or symlink onto your PATH
```

## Quick start

```shell
# Example layout
src/
├── .overlay.toml
└── dot-claude/
    ├── settings.olay.base.json      # always merged first
    ├── settings.olay.work.json      # "work" profile
    └── settings.olay.local.json     # always merged last

# Run
cd src
overlay --profiles work render
```

With `dot_prefix = true` (the default) and `target = "~/"`, the merged file is written to `~/.claude/settings.json`.

For stow-style dotfiles repos, configure package directories as separate source roots:

```toml
sources = ["pi", "codex", "opencode"]
target = "~/"
dot_prefix = true
```

Then `pi/dot-pi/agent/models.olay.base.json` renders to `~/.pi/agent/models.json`; the package directory (`pi`) is the source root and does not appear in the target path.

## Representative commands

| Command | Result |
|---|---|
| `overlay plan [source...]` | Print the table of files a run would generate; writes nothing. |
| `overlay diff [source...]` | Print a unified diff between rendered output and the current target files; exit 1 on drift. |
| `overlay render [source...]` | Write the rendered files and record them in `.overlay.state.json`. |
| `overlay render --no-state` | Write the files without reading or updating the ownership state. |
| `overlay orphans [--json]` | List targets an earlier render wrote that the current plan no longer produces; exit 1 when any exist. |
| `overlay config [--validate PATH]` | Print the effective configuration with provenance, or validate one file. |
| `overlay docs` | Print the `.overlay.toml` schema reference. |

Positional sources select package roots for one run (`overlay plan pi codex`, `overlay render pi`). Selecting profiles for one run looks like `overlay --profiles work render` or `OVERLAY_PROFILES=work overlay render`. `overlay diff` pipes into any diff viewer, for example `overlay diff | delta`.

## Required external programs

None. overlay runs no external program and never accesses the network.

## Configuration

overlay reads `.overlay.toml` from the current directory when it exists, or from the file named by `--config`, which must exist. Parent directories and user-level locations are not searched. A minimal file:

```toml
target = "~/"              # required
profiles = ["work"]        # optional
```

`sources`, `target`, `profiles`, `continue_on_error`, and variable pins can also come from `OVERLAY_SOURCES`, `OVERLAY_TARGET`, `OVERLAY_PROFILES`, `OVERLAY_CONTINUE`, and `OVERLAY_VARS`, or from the matching global flags; a flag beats the environment, which beats the file. `overlay config` prints the values in effect, `overlay config --help` documents the precedence and the report, and `overlay docs` documents every field.

`overlay render` also maintains `.overlay.state.json` beside the config file so `overlay orphans` can find stale targets. The file is machine-specific; add it to `.gitignore`.

## Known gaps

- Symlink behavior is not yet a stable contract. Source-side behavior is tracked in [#54](https://github.com/jmcampanini/overlay/issues/54) and target-write behavior in [#34](https://github.com/jmcampanini/overlay/issues/34); each command's `--help` states what happens today.

## Development

Run `make` to list the tasks; `make check` is the full validation before a change is done.
