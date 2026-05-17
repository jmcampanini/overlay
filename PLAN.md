# GoConfigLoader migration plan

## Goal

Use `github.com/jmcampanini/go-config-loader` for raw configuration loading:

- defaults
- TOML file loading
- environment variable loading for selected fields
- pflag loading for selected fields
- raw config reporting

Keep Overlay-specific runtime logic outside config loading:

- effective profiles derivation
- path normalization / expansion
- target-required checks
- reserved profile validation
- ignore/gitignore setup
- construction of `discover.Settings`

## Decisions

### Raw config vs runtime settings

`internal/config` should represent what was loaded, not what Overlay ultimately uses at runtime.

Runtime code may derive additional values from raw config, but those derived values are not the responsibility of the config loader.

### Config-backed flags

Use `pflagloader.Register[config.Config]` for only fields with `config` tags.

Manual flags remain for non-config command/runtime concerns.

| Flag | Owner | Config field | Env var |
|---|---|---|---|
| `--config` | manual | none | none |
| `--source` | `pflagloader.Register` | `Source` | `OVERLAY_SOURCE` |
| `--target` | `pflagloader.Register` | `Target` | `OVERLAY_TARGET` |
| `--profiles` | `pflagloader.Register` | `Profiles` | `OVERLAY_PROFILES` |
| `--continue` | `pflagloader.Register` | `ContinueOnError` | `OVERLAY_CONTINUE` |
| `--quiet`, `-q` | manual | none | none |
| `--verbose`, `-v` | manual | none | none |
| `config --validate` | manual local flag | none | none |

Do not add `config` tags to TOML-only fields.

### Config struct tags

Target shape:

```go
type Config struct {
	Source           string   `toml:"source" config:"source" help:"override source directory from config"`
	Target           string   `toml:"target" config:"target" help:"override target directory from config"`
	DotPrefix        bool     `toml:"dot_prefix"`
	Profiles         []string `toml:"profiles" config:"profiles" help:"comma-separated profile list"`
	EnvProfiles      string   `toml:"env_profiles"`
	ContinueOnError  bool     `toml:"continue_on_error" config:"continue" help:"continue past invalid source files"`
	Ignore           []string `toml:"ignore"`
	TraverseHidden   bool     `toml:"traverse_hidden"`
	RespectGitignore bool     `toml:"respect_gitignore"`
}
```

TOML-only fields remain:

- `dot_prefix`
- `env_profiles`
- `ignore`
- `traverse_hidden`
- `respect_gitignore`

### Effective profiles

`Profiles` is a raw loaded config value.

After GoConfigLoader has loaded defaults, file, env, and flags, Overlay derives effective profiles:

```text
effective_profiles = dedupe(cfg.Profiles + splitCSV(os.Getenv(cfg.EnvProfiles)))
```

This means `env_profiles` is considered after raw config loading.

Expected behavior change from current Overlay:

```toml
profiles = ["base-tools"]
env_profiles = "DOTFILES_PROFILE"
```

```shell
DOTFILES_PROFILE=work overlay --profiles personal plan
```

New effective profiles:

```text
["personal", "work"]
```

Current behavior was:

```text
["personal"]
```

This is intentional under the new model because `--profiles` sets raw `Profiles`, and effective profiles are derived afterward.

### Validation

Application-specific validation runs after full raw config loading and effective profile derivation.

Validation remains Overlay-owned:

- reserved profile names
- required `target`
- ignore glob validation
- gitignore setup errors
- path expansion errors

GoConfigLoader is not responsible for these checks.

### Path handling

Path handling remains Overlay runtime logic, not config loading.

GoConfigLoader loads strings as strings. Overlay may still post-process `Source` and `Target` for runtime use:

- config-file-relative paths
- `~`
- `$VAR` / `${VAR}`
- undefined env var errors

If this behavior is removed later, the user-visible losses are:

- `source = "~/dotfiles"` in TOML no longer expands to the home directory
- `target = "$HOME/out"` in TOML stays literal
- `--source=~/dotfiles` stays literal in shells that do not expand `~` after `=`
- relative paths in `--config path/to/.overlay.toml` become cwd-relative instead of config-file-relative

For this migration, preserve current runtime path behavior outside the config package.

### Reporting

Switch config reporting to `go-config-loader/configreporter`.

`overlay config` should report raw loaded config values and GoConfigLoader provenance, not runtime-derived values.

Consequences:

- `profiles` shows raw loaded `Profiles`, not effective profiles after `env_profiles` append.
- `source` / `target` show raw loaded strings, not expanded runtime paths.
- provenance comes from GoConfigLoader report sources.

## Implementation plan

### 1. Add dependency

Add `github.com/jmcampanini/go-config-loader` to `go.mod` using `make` workflows.

Expected dependency impact:

- root package: config loading, env loading, file loading
- `pflagloader`: config-backed flags
- `configreporter`: raw config output

### 2. Update `internal/config`

Replace custom TOML loading internals with GoConfigLoader composition.

Keep:

- `Config`
- `Default()`
- `DefaultFilename`
- Overlay validation helpers

Change/add:

- raw load function returning `Config`, existence, and `configloader.LoadReport`
- validate-file function implemented as required-file load plus Overlay validation

Remove or stop depending on:

- custom `decodeStrict`
- custom `LoadKeys`
- manual top-level key provenance tracking

### 3. Update flag binding

Change `GlobalFlags` to only hold non-config manual flags:

```go
type GlobalFlags struct {
	Config  string
	Quiet   bool
	Verbose bool
}
```

`Bind` should:

1. register `--config`, `--quiet`, `--verbose` manually
2. call `pflagloader.Register[config.Config](cmd.PersistentFlags())`

Expected registered config-backed flags:

- `--source`
- `--target`
- `--profiles`
- `--continue`

### 4. Update resolver

Resolver flow:

1. determine config path from `--config` or default `.overlay.toml`
2. choose optional vs required file loader based on explicit `--config`
3. compose loaders in precedence order:
   - file loader
   - environment loader with prefix `overlay`
   - pflag loader from `cmd.Flags()`
4. call `configloader.Load(config.Default(), loaders...)`
5. derive effective profiles from raw `cfg.Profiles` + `cfg.EnvProfiles`
6. run Overlay validation on effective profiles/final runtime requirements
7. normalize/expand runtime paths
8. build `discover.Settings`

### 5. Update `overlay config`

Use `configreporter.New(cfg, report)` and write TOML/provenance output for raw loaded config.

The command should no longer present itself as fully runtime-resolved settings unless we intentionally add a separate runtime-report command later.

### 6. Update docs

Update README/help/schema docs to reflect:

- GoConfigLoader-style env vars for tagged fields
- `env_profiles` append happens after raw loading
- `overlay config` reports raw loaded config, not runtime-effective settings
- path expansion remains runtime behavior for `source` / `target`

## Test plan

### Config loading tests

#### Missing config file

Given no `.overlay.toml`, loading uses `Default()`.

Expected:

- no error
- config exists false
- defaults preserved

#### Valid TOML file

Given all TOML fields set, raw config contains those exact values.

Expected:

- file is listed in `LoadReport.LoadedFiles`
- fields match TOML values
- TOML-only fields load correctly

#### Unknown TOML key

Given typo key like `respect_gitigore`.

Expected:

- load fails
- error mentions unknown key

#### Malformed TOML

Expected:

- load fails

### Flag tests

#### Registered flags

Root command has:

- `--config`
- `--source`
- `--target`
- `--profiles`
- `--continue`
- `--quiet`
- `--verbose`

Expected:

- config-backed flags are registered by `pflagloader.Register`
- manual flags are still available

#### Flags override file/env raw config

Given TOML and env values, CLI flags override tagged config fields.

Expected examples:

- `--source flag-src` overrides file/env source
- `--target flag-target` overrides file/env target
- `--profiles flag-a,flag-b` overrides file/env profiles
- `--continue=false` overrides file/env continue

#### TOML-only fields have no pflags

Expected absent flags:

- `--dot-prefix`
- `--env-profiles`
- `--ignore`
- `--traverse-hidden`
- `--respect-gitignore`

### Environment tests

#### Tagged env vars load

Set:

```text
OVERLAY_SOURCE=/env/source
OVERLAY_TARGET=/env/target
OVERLAY_PROFILES=env-a,env-b
OVERLAY_CONTINUE=true
```

Expected:

- raw config reflects env values when no higher-priority flag is set

#### TOML-only env vars do not load

Set hypothetical env vars:

```text
OVERLAY_DOT_PREFIX=false
OVERLAY_IGNORE=**/node_modules
OVERLAY_TRAVERSE_HIDDEN=true
OVERLAY_RESPECT_GITIGNORE=true
OVERLAY_ENV_PROFILES=SOME_VAR
```

Expected:

- raw config is unchanged by these env vars

### Effective profiles tests

#### File profiles only

```toml
profiles = ["work"]
```

Expected effective profiles:

```text
["work"]
```

#### Env profiles append after raw load

```toml
profiles = ["base-tools"]
env_profiles = "DOTFILES_PROFILE"
```

With:

```text
DOTFILES_PROFILE=work,vpn
```

Expected effective profiles:

```text
["base-tools", "work", "vpn"]
```

#### CLI profiles plus env_profiles

```toml
env_profiles = "DOTFILES_PROFILE"
```

With:

```shell
DOTFILES_PROFILE=work overlay --profiles personal plan
```

Expected raw profiles:

```text
["personal"]
```

Expected effective profiles:

```text
["personal", "work"]
```

#### Dedupe preserves first occurrence

Raw profiles:

```text
["a", "b"]
```

Env profiles:

```text
b,c,a
```

Expected effective profiles:

```text
["a", "b", "c"]
```

#### Reserved profile validation after derivation

If `env_profiles` injects `base` or `local`, validation fails.

Expected:

- error after effective profile derivation

### Path runtime tests

#### Config-relative source

Config file path:

```text
/root/sub/.overlay.toml
```

TOML:

```toml
source = "pkgs"
target = "/tmp/out"
```

Expected runtime source:

```text
/root/sub/pkgs
```

Raw reported config source remains:

```text
pkgs
```

#### Tilde/env expansion remains runtime-only

TOML:

```toml
source = "~/src"
target = "$OVERLAY_TEST_TARGET/out"
```

Expected:

- raw config keeps literal strings
- runtime settings use expanded paths
- undefined env vars produce runtime resolution errors

### Validation tests

#### Missing target after full load

No file/env/flag supplies target.

Expected:

- raw loading succeeds
- Overlay validation/resolution fails with target-required error

#### Env target satisfies required target

No file target, but `OVERLAY_TARGET=/tmp/out`.

Expected:

- raw loading succeeds
- Overlay validation passes

#### Flag target satisfies required target

No file/env target, but `--target /tmp/out`.

Expected:

- raw loading succeeds
- Overlay validation passes

### Reporter tests

#### Raw config report

Given:

```toml
source = "pkgs"
target = "~/out"
profiles = ["work"]
env_profiles = "DOTFILES_PROFILE"
```

With:

```text
DOTFILES_PROFILE=vpn
```

Expected `overlay config` raw report includes:

```toml
source = "pkgs"
target = "~/out"
profiles = ["work"]
env_profiles = "DOTFILES_PROFILE"
```

It should not show expanded paths or effective profiles.

Expected effective runtime profiles elsewhere:

```text
["work", "vpn"]
```

## Expected behavior changes

1. `overlay config` reports raw loaded config instead of fully runtime-resolved settings.
2. `OVERLAY_SOURCE`, `OVERLAY_TARGET`, `OVERLAY_PROFILES`, and `OVERLAY_CONTINUE` become supported.
3. `env_profiles` is applied after raw loading, so it appends even when raw `Profiles` came from `--profiles`.
4. Config-backed flags are registered through GoConfigLoader/pflagloader.

## Non-goals

- Do not make GoConfigLoader understand Overlay effective profiles.
- Do not move path expansion into GoConfigLoader.
- Do not move Overlay domain validation into GoConfigLoader.
- Do not add config/env/flag support for every TOML field.
- Do not preserve old `overlay config` fully-resolved output in this migration.
