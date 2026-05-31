# Plan: Add singular `--profile` flag support

## Goal

Add a repeatable singular profile flag to Overlay using GoConfigLoader's `pflag_singular` support, matching sibling CLI repositories.

## Decision

Use strict GoConfigLoader semantics:

- Keep `--profiles a,b,c` as the canonical CSV/list flag.
- Add `--profile NAME` as a repeatable pflag-only alias for `profiles`.
- When both are provided, GoConfigLoader combines canonical `--profiles` values first, then repeated `--profile` values, and deduplicates.
- Do not add `.overlay.toml profile = ...`.
- Do not add `OVERLAY_PROFILE`.
- Empty singular values such as `--profile=` should be rejected by pflag parsing.

## Source-resolved basis

- Overlay already uses GoConfigLoader and `pflagloader.Register[config.Config]` for config-backed flags.
- Overlay already uses `pflag_singular:"source"` for `sources`.
- Sibling CLIs use `pflag_singular:"profile"` for profile slices.
- GoConfigLoader's singular flags are pflag-only, repeatable, reject empty values, do not CSV-split singular values, and dedupe after combining canonical values before singular values.

## Implementation steps

1. Update `internal/config/config.go`
   - Add `pflag_singular:"profile"` to `Config.Profiles`.
   - Adjust the help string to mention `--profiles` accepts comma-separated lists.

2. Update CLI resolution tests in `internal/cli/resolve_test.go`
   - Assert `--profile` is registered.
   - Add a test for a single `--profile work` overriding file/env profiles.
   - Add a test for repeated `--profile work --profile personal` preserving order.
   - Add a test for mixed `--profiles work,personal --profile client` producing canonical-then-singular deduped output.
   - Add a test that `--profile=` fails during flag parsing.

3. Update config/loading tests if useful in `internal/config/config_test.go`
   - Add a focused registration/load test only if the CLI tests do not sufficiently cover pflagloader integration.

4. Update documentation
   - README profile resolution section: mention `--profiles a,b,c` or repeated `--profile NAME`.
   - README examples: add one concise repeated `--profile` example.
   - `internal/config/schema.go`: update Profile Resolution Precedence and Config-backed Environment Variables sections to mention `--profile` as pflag-only.
   - `internal/cli/helptext.go`: update `profilePrecedenceHelp` similarly.

5. Validate
   - Run `make check` before declaring done.

## Non-goals

- No singular TOML key support.
- No singular environment variable support.
- No custom parser to preserve exact interleaving of mixed `--profiles` and `--profile` flags.
- No comma splitting for individual `--profile` values.

## Expected user-facing behavior

```sh
overlay --profiles work,personal plan
overlay --profile work --profile personal plan
overlay --profiles work --profile personal plan
```

All three select profiles through the same raw `profiles` field, with normal Overlay profile resolution still appending `env_profiles` afterward.
