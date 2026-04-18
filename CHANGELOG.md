# Changelog

All notable changes to this project are documented in this file. The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow [SemVer](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed (breaking)

- **Output layout now follows the [agentskills.io spec](https://agentskills.io/specification).** Each skill is written as `<name>/SKILL.md` inside the target directory instead of `<name>.md` flat. Agent loaders that expect the spec layout (including Claude Code) now find skillgen output correctly.
- **`Skill.Filename` renamed to `Skill.Path`** and now contains the relative path `<name>/SKILL.md` rather than a basename. Callers that referenced `Filename` need to update.

### Added

- **Spec-standard frontmatter annotations**: `skill.license`, `skill.compatibility`, and `skill.metadata.<key>` (prefix pattern that assembles a sorted `metadata:` map). Emitted under every target.
- **Spec-limit lint rules**: name length > 64 (error), name format regex (error), description length > 1024 (error), compatibility length > 500 (error), body > 5000 tokens (warning), body > 500 lines (warning).
- **Per-skill budget warnings** in `skills generate` when any one skill body exceeds the spec's 5000-token recommendation, in addition to the existing aggregate warning.
- `Skill.Dir()` returns the skill's directory name from its `Path`.
- `SpecMaxNameLength`, `SpecMaxDescriptionLength`, `SpecMaxCompatibilityLength`, `SpecMaxBodyTokens`, `SpecMaxBodyLines` exposed as constants.

### Fixed

- **`skill.trigger` no longer double-prefixes full-sentence inputs.** The annotation previously had an undocumented contract — it had to be a fragment like `"deploy, ship"` because the library prepended `"Use when the user asks to "` and appended `"."`. Authors who supplied a complete sentence like `"Use when the user asks to deploy."` got `"…Use when the user asks to Use when the user asks to deploy.."`. Both forms now work: fragments are wrapped as before, full-sentence inputs are detected (case-insensitive `"use when the user asks"` prefix) and used as-is with a single trailing period. Documented on the annotation constant.

### Changed

- **"When to use" section no longer restates the description.** It now contains only the trigger clause ("Use when the user asks to …"), which is the content agents actually need. When no trigger signal is available — no `skill.trigger`, no aliases — the section is omitted entirely instead of faking guidance by restating Short/Long.
- **Root commands without explicit triggers now synthesize one from visible child names.** A CLI with subcommands `[dev, gpu, k8s]` gets "Use when the user asks about dev, gpu, or k8s." added automatically. Leaves do not synthesize (their own name would just echo the description).

### Added

- **`WithTarget(TargetClaudeCode)`** — emits Claude Code-specific frontmatter fields. Initial support for `allowed-tools` via the new `skill.allowed-tools` annotation (comma-separated list like `"Bash, Read, Edit"`). `TargetGeneric` remains the default and is unchanged.
- **Output budget summary** — `skills generate` now prints `N skill(s), ~K tokens (X KB)` after writing and warns to stderr when output exceeds ~15 000 tokens. Helps authors see how much agent context their skills consume.
- **Three new lint rules**:
  - `operator-subtree` — warns when a command name ends `-operator`, `-daemon`, or `-runner` (usually an internal server command that wants `skill.skip`).
  - `depth` — warns when a command's path is 4+ levels deep (agents struggle with deeply nested matching).
  - `sibling-variance` — warns a parent when its children's description lengths vary wildly (max > 3× min with short ≤ 40 chars), flagging asymmetric skill quality.

## [0.3.0] — 2026-04-18

### Added

- **Cobra field mining.** `cmd.Aliases` now render as an `Aliases: …` note and auto-derive a trigger suffix when `skill.trigger` isn't set. `cmd.Deprecated` renders a prominent `> **Deprecated:** …` callout. Deprecated flags (`pflag.Flag.Deprecated`) are filtered from the rendered flag list entirely.
- **Anti-pattern annotations.** `skill.avoid` and `skill.prefer-over` render dedicated `## Avoid` and `## Prefer over` sections in root / leaf bodies and inline bold labels inside nested command sections. These capture the single highest-value content in most real skills: what *not* to do.
- **`skills lint` subcommand.** `Generator.Lint()` walks the same tree the generator walks and returns `[]Issue` (errors + warnings) for missing descriptions, overly short `Short`, leaves without trigger hints, and deprecated commands without a useful message. `skills lint` prints the report and exits non-zero on errors; `--strict` promotes warnings to errors for CI enforcement. Respects Hidden, `skill.skip`, and the `WithSkip` predicate so lint and generation stay in sync.

## [0.2.0] — 2026-04-18

### Added

- Split mode: `WithSplit(SplitPerLeaf)` emits one skill per leaf command of the cobra tree. Filenames are slugged from the full command path (`mytool deploy` → `mytool-deploy.md`).
- `WithOverview(true)` opts into a short overview skill alongside split-mode leaves, listing every leaf with a one-line summary. Suppressed automatically when only a single leaf exists.
- Documentation: README gained a "Split mode" section; PRD M4 marked complete.

## [0.1.1] — 2026-04-18

### Changed

- Retag of `v0.1.0` on the same commit. No code changes. Needed to sidestep a stale negative cache on `proxy.golang.org` caused by an earlier malformed `0.1.0` tag (missing the leading `v`).

## [0.1.0] — 2026-04-18

> Note: the tag exists on GitHub but cannot be resolved by `go get` because of a poisoned proxy cache. Consumers should pin to `v0.1.1`.

### Added

- Initial release.
- `NewSkillsCmd(root)` drop-in subcommand with `skills generate` / `skills print`, mirroring cobra's `completion`.
- Single-skill mode: one Markdown file with YAML frontmatter compatible with the Claude Code skill format.
- Annotations: `skill.trigger`, `skill.description`, `skill.name`, `skill.skip`, `skill.examples`.
- Options: `WithTemplate`, `WithSkip`, `WithFilenamePrefix`, `WithIncludeBuiltins`.
- Auto-filter for cobra's injected `help` / `completion` subcommands (depth-aware, so a user-defined nested `help` is preserved).
- MIT license, README, PRD, runnable `./example` CLI.

[Unreleased]: https://github.com/bueti/skillgen/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/bueti/skillgen/releases/tag/v0.3.0
[0.2.0]: https://github.com/bueti/skillgen/releases/tag/v0.2.0
[0.1.1]: https://github.com/bueti/skillgen/releases/tag/v0.1.1
[0.1.0]: https://github.com/bueti/skillgen/releases/tag/v0.1.0
