# Changelog

All notable changes to this project are documented in this file. The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow [SemVer](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/bueti/skillgen/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/bueti/skillgen/releases/tag/v0.2.0
[0.1.1]: https://github.com/bueti/skillgen/releases/tag/v0.1.1
[0.1.0]: https://github.com/bueti/skillgen/releases/tag/v0.1.0
