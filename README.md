# overlay

overlay renders layered dotfiles from git source repositories into a target directory. One configuration file lists the layers to apply and the sources that provide files; `overlay bootstrap` writes that file and clones the sources, `overlay config` prints the configuration in effect, and `overlay render` copies the files the layers select into the target directory. overlay never overwrites its configuration file, never touches an existing checkout, never deletes a rendered file, and never lets git prompt for credentials.

Command help is the canonical reference: `overlay --help` and each command's `--help` describe every user-facing contract, `overlay config --help` describes configuration precedence and the file schema, `overlay render --help` describes the source manifest and the rendering rules, and `overlay help exit-codes` describes exit statuses.

## Install

Releases ship binaries for Linux and macOS on amd64 and arm64.

```sh
mise use -g github:jmcampanini/overlay   # Install with mise
mise upgrade github:jmcampanini/overlay  # Upgrade with mise
brew install jmcampanini/tap/overlay     # Install with Homebrew
brew upgrade jmcampanini/tap/overlay     # Upgrade with Homebrew
```

Or download an archive from the [releases page](https://github.com/jmcampanini/overlay/releases) and put `overlay` on your `PATH`; to upgrade, download the new release and replace the binary. Each release includes a SHA-256 checksums file and GitHub build provenance attestations. On macOS, a browser download needs `xattr -d com.apple.quarantine overlay` before it runs; mise, Homebrew, `curl`, and `gh` downloads do not.

To build from source, install [mise](https://mise.jdx.dev) (`brew install mise` on macOS) and run `mise trust`, `mise install`, then `mise run build`; copy `./build/overlay` onto your `PATH`.

## Representative commands

| Command | Result |
|---|---|
| `overlay bootstrap --source-url URL --source-path PATH --layers LIST` | Clone each source that is not checked out yet and write the configuration file. |
| `overlay config` | Print the configuration in effect as TOML with the effective layers and resolved sources. |
| `overlay config --provenance` | Also name where each field came from: default, file, environment, or flag. |
| `overlay config --layers work,laptop` | Preview the configuration with a different layer selection. |
| `overlay render --dry-run` | Print the files the effective layers would write to the home directory, with their status and what they replace. |
| `overlay render` | Write those files, skipping ones already identical, and print the same plan. |
| `overlay render --target ~/staging --json` | Render into another directory and print the plan as one JSON document. |

## Required external programs

`overlay bootstrap` runs `git` from `PATH` to clone sources and to read the origin of an existing checkout. It sets `GIT_TERMINAL_PROMPT=0`, so git never prompts; set up SSH or token authentication first. `overlay config` and `overlay render` run no external program.

## Configuration

overlay reads `$XDG_CONFIG_HOME/overlay/config.toml`, or `~/.config/overlay/config.toml` when `XDG_CONFIG_HOME` is empty, and searches nowhere else. `--config PATH` reads an explicit file that must exist, or for `bootstrap` names the file to write. Layers can also be chosen per invocation with `OVERLAY_LAYERS`, `--layers`, or `--layer`; `overlay config --help` documents the precedence and every field.

```toml
layers = ["abc", "xyz"]

[[source]]
url = "git@github.com:jmcampanini/dotfiles-base.git"
path = "~/code/github.com/jmcampanini/dotfiles-base"

[[source]]
url = "git@github.com:jmcampanini/dotfiles-xyz.git"
path = "~/code/github.com/jmcampanini/dotfiles-xyz"
```

See `overlay render --help` for the source manifest and rendering rules.

```toml
[layers.base]
packages = ["bash", "fish", "ssh"]

[layers.personal]
packages = ["ssh"]
```
