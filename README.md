# overlay

overlay renders layered dotfiles from git source repositories into a target directory. One configuration file lists the layers to apply and the sources that provide files; `overlay bootstrap` writes that file and clones the sources, and `overlay config` prints the configuration in effect. overlay never overwrites its configuration file, never touches an existing checkout, and never lets git prompt for credentials. Rendering commands arrive in later releases.

Command help is the canonical reference: `overlay --help` and each command's `--help` describe every user-facing contract, `overlay config --help` describes configuration precedence and the file schema, and `overlay help exit-codes` describes exit statuses.

## Install

Each release ships binaries for Linux (amd64 and arm64) and macOS on Apple Silicon (arm64), with a SHA-256 checksums file and GitHub build provenance attestations. Releases are at <https://github.com/jmcampanini/overlay/releases>.

### mise

```sh
mise use -g github:jmcampanini/overlay
```

mise verifies the checksum and the attestation on download.

### Homebrew

```sh
brew install jmcampanini/tap/overlay
```

The cask removes macOS's quarantine attribute after install because the binary is not notarized.

### Direct download

Download the archive and checksums file for a release, verify them, and put `overlay` on your `PATH`:

```sh
gh release download --repo jmcampanini/overlay --pattern '*_linux_amd64.tar.gz' --pattern '*_checksums.txt'
sha256sum --check --ignore-missing overlay_*_checksums.txt
gh attestation verify overlay_*_linux_amd64.tar.gz --repo jmcampanini/overlay
tar -xzf overlay_*_linux_amd64.tar.gz overlay
```

On macOS use `shasum -a 256 --check` and the `darwin_arm64` archive. A binary downloaded through a browser carries the quarantine attribute and macOS refuses to run it; clear it with `xattr -d com.apple.quarantine overlay`. Downloads made with `curl` or `gh` do not have the attribute.

### From source

```sh
make build
# then copy ./build/overlay to a directory on your PATH
```

## Representative commands

| Command | Result |
|---|---|
| `overlay bootstrap --source-url URL --source-path PATH --layers LIST` | Clone each source that is not checked out yet and write the configuration file. |
| `overlay config` | Print the configuration in effect as TOML with the effective layers and resolved sources. |
| `overlay config --provenance` | Also name where each field came from: default, file, environment, or flag. |
| `overlay config --layers work,laptop` | Preview the configuration with a different layer selection. |

## Required external programs

`overlay bootstrap` runs `git` from `PATH` to clone sources and to read the origin of an existing checkout. It sets `GIT_TERMINAL_PROMPT=0`, so git never prompts; set up SSH or token authentication first. `overlay config` runs no external program.

## Configuration

overlay reads `$XDG_CONFIG_HOME/overlay/config.toml`, or `~/.config/overlay/config.toml` when `XDG_CONFIG_HOME` is empty, and searches nowhere else. `--config PATH` reads an explicit file that must exist, or for `bootstrap` names the file to write. Layers can also be chosen per invocation with `OVERLAY_LAYERS`, `--layers`, or `--layer`; `overlay config --help` documents the precedence and every field.

```toml
layers = ["personal", "server"]

[[source]]
url = "git@github.com:jmcampanini/dotfiles-base.git"
path = "~/dotfiles/base"

[[source]]
url = "git@github.com:jmcampanini/dotfiles-personal.git"
path = "~/dotfiles/personal"
```
