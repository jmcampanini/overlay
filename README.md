# overlay

overlay renders layered dotfiles from git source repositories into a target directory. Later layers replace earlier files, merge JSON, TOML, and YAML fragments, or concatenate fragments of any other text file, so shared and personal repositories can contribute to the same configuration. Rendering preserves overlay's active configuration and never deletes a rendered file. Bootstrapping leaves existing checkouts untouched and never lets git prompt for credentials.


## Install

Versioned releases ship binaries for Linux and macOS on amd64 and arm64, with SHA-256 checksums and GitHub build provenance attestations.

Install or upgrade with [mise](https://mise.jdx.dev):

```sh
mise use -g github:jmcampanini/overlay
mise upgrade github:jmcampanini/overlay
```

Install or upgrade with [Homebrew](https://brew.sh):

```sh
brew install jmcampanini/tap/overlay
brew upgrade jmcampanini/tap/overlay
```

Download a [release archive](https://github.com/jmcampanini/overlay/releases) with [GitHub CLI](https://cli.github.com) (`gh`) and install or upgrade into `~/.local/bin`, which must be on your `PATH`:

```sh
# This example selects macOS arm64; use linux or amd64 for other platforms.
gh release download --repo jmcampanini/overlay \
  --pattern 'overlay_*_darwin_arm64.tar.gz' --output overlay.tar.gz --clobber
tar -xzf overlay.tar.gz
mkdir -p ~/.local/bin
install -m 755 overlay ~/.local/bin/overlay
# For a macOS archive downloaded through a browser instead of gh:
# xattr -d com.apple.quarantine ~/.local/bin/overlay
```

Build from source with `git` and [mise](https://mise.jdx.dev), installing into `~/.local/bin`, which must be on your `PATH`:

```sh
git clone https://github.com/jmcampanini/overlay
cd overlay
mise trust
mise install
mise run build
mkdir -p ~/.local/bin
install -m 755 build/overlay ~/.local/bin/overlay
```


## Representative commands

| Command | Result |
|---|---|
| `overlay bootstrap --source-url URL --source-path PATH --layers LIST` | Clone each source that is not checked out yet and write the configuration file. |
| `overlay config` | Print the configuration in effect as TOML with the effective layers and resolved sources. |
| `overlay render --dry-run` | Print the files the effective layers would write to the home directory, with their status and what they replace. |
| `overlay render` | Write those files, skipping ones already identical, and print the same plan. |
| `overlay render --target ~/staging --json` | Render into another directory and print the plan as one JSON document. |
| `overlay diff` | Print a unified patch from the home directory's current files to what `overlay render` would write; from `~`, it feeds `git apply`. |
| `overlay values --provenance` | Print the merged values templates read as `.Values`, with the file that last set each top-level key. |
| `overlay files 'packages/homebrew/Brewfile.{layer}'` | Print matching file paths in layer, source, and pattern order. |
| `overlay cat --list 'packages/pacman/packages.{layer}'` | Print unique list entries in match order, trimming whitespace and comments. |

`overlay --help`, each command's `--help`, and the help topic `overlay help exit-codes` are the canonical reference for every user-facing contract.


## Required external programs

- `git`: `overlay bootstrap` runs it from `PATH` to clone sources and to read the origin of an existing checkout.


## Configuration

overlay reads `$XDG_CONFIG_HOME/overlay/config.toml`, or `~/.config/overlay/config.toml` when `XDG_CONFIG_HOME` is unset, empty, or relative, and searches nowhere else. `overlay config --help` documents configuration precedence, environment-based layer additions, and the file schema. Templates read per-machine data as `.Values`, merged from every `*.toml` in the `values` directory beside the configuration file and from `--values PATH` operands; `overlay render --help` documents the merge order.

```toml
layers = ["abc", "xyz"]
env_layers = ["OVERLAY_EXTRA_LAYERS"]

[[source]]
url = "git@github.com:jmcampanini/dotfiles-base.git"
path = "~/code/github.com/jmcampanini/dotfiles-base"

[[source]]
url = "git@github.com:jmcampanini/dotfiles-xyz.git"
path = "~/code/github.com/jmcampanini/dotfiles-xyz"
```
