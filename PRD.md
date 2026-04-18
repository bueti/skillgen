# PRD: skillgen

A Go library for generating agent skill files from a CLI's command tree, analogous to how `spf13/cobra` generates shell tab completions.

## 1. Summary

`skillgen` lets CLI authors emit agent skills (Markdown + YAML frontmatter) directly from their cobra command tree. One line of integration gives a tool a `skills` subcommand — the same pattern cobra already uses for `completion`. The generated files teach AI coding agents (Claude Code and compatible tools) when the CLI is relevant and how to invoke it, without the author hand-writing and maintaining prose.

## 2. Motivation

Agent skills are becoming the standard way to make a CLI discoverable inside agent workflows. Authors today have three bad options:

- Hand-write skill Markdown, which drifts from the actual commands, flags, and help text.
- Duplicate help content across `--help`, README, docs site, and skill files.
- Ship no skill at all because the overhead isn't justified for a small tool.

Cobra solved the identical drift problem for shell completions by generating them from the authoritative source — the command tree. `skillgen` applies the same idea to agent skills.

## 3. Goals

- **G1** — Given a `*cobra.Command` tree, emit skill files that accurately describe the CLI.
- **G2** — Ship a drop-in `skills` subcommand authors register with a single line, matching cobra's `completion` ergonomics.
- **G3** — Produce output compatible with the Claude Code skill format (frontmatter `name` + `description`, Markdown body).
- **G4** — Let authors enrich generated skills with trigger hints, examples, and custom instructions without forking the library's templates.
- **G5** — Keep re-runs deterministic so generated files live comfortably in git.

## 4. Non-goals

- Not a skill runtime or executor — this only generates files.
- Not a replacement for `--help` or a docs site.
- Not opinionated about where skills get installed; that is the agent's concern.
- Not targeting non-cobra CLI frameworks in v1.

## 5. Users

- **Primary** — Go CLI authors who already use cobra and want their tool to be discoverable and usable by coding agents.
- **Secondary** — Platform teams standardizing how internal CLIs expose themselves to agents across an organization.

## 6. Skill output format (v1)

**Default: one skill per CLI.** A single `.md` file describes the whole tool and every subcommand, Claude Code-compatible:

```markdown
---
name: mytool
description: Build and deploy mytool services. Use when the user asks to build, deploy, promote, ship, release, or inspect the state of a mytool service.
---

# mytool

A CLI for building and deploying services.

## When to use
Any task involving building, deploying, or inspecting mytool services.

## Commands

### mytool build <service>
Build an artifact of a service.
Flags: `--tag`, `--push`.
Example: `mytool build api --tag v1.2.3`

### mytool deploy <service>
Deploy a built artifact to an environment.
Flags: `--env` (required), `--dry-run`.
Example: `mytool deploy api --env staging`

### mytool status <service>
Show current deployment state.
Example: `mytool status api --env prod`
```

**Opt-in: split mode.** For large CLIs (dozens of subcommands, or independent command groups), authors can split output into one skill per leaf command — each with a precise, subcommand-specific `description` — optionally alongside a thin overview skill that points into them.

The split threshold is a deliberate author decision, not auto-detected: the tradeoffs (index size, trigger precision, context cost on load) depend on how the CLI is used, not just how big it is.

## 7. Functional requirements

- **F1** — Generate a single skill describing the whole CLI by default, with all subcommands rendered as sections.
- **F2** — Support an opt-in split mode: one skill per leaf command (precise triggers) with an optional overview skill that links to them.
- **F3** — Source `Short`, `Long`, `Example`, subcommand list, and flag metadata directly from cobra.
- **F4** — Support per-command augmentation via `cmd.Annotations` (e.g. `skill.trigger`, `skill.examples`, `skill.skip`) so authors don't need wrapper types.
- **F5** — Expose a template hook so authors can override the Markdown body while keeping the frontmatter generator.
- **F6** — `mytool skills generate --dir .claude/skills/` writes files with stable, slug-based filenames so re-runs produce clean diffs.
- **F7** — Honor `Hidden`, a new `SkipSkill` annotation, and a caller-supplied predicate for exclusion.
- **F8** — Validate frontmatter on write (non-empty `name`, description length bounds) and fail loudly on invalid output.

## 8. API sketch

Drop-in subcommand:

```go
import "github.com/bueti/skillgen"

root := &cobra.Command{Use: "mytool"}
// ... register subcommands ...
root.AddCommand(skillgen.NewSkillsCmd(root))
```

Programmatic:

```go
gen := skillgen.New(root,
    skillgen.WithSplit(skillgen.SplitPerLeaf), // optional; default is SplitNone (single skill)
    skillgen.WithOverview(true),                // only meaningful in split mode
    skillgen.WithTemplate(myTmpl),
)
if err := gen.WriteTo("./.claude/skills"); err != nil { ... }
```

Augmentation via annotations:

```go
deployCmd.Annotations = map[string]string{
    "skill.trigger":  "deploy, promote, ship, release",
    "skill.examples": "mytool deploy api --env staging",
}
```

## 9. Integration flow

1. Author adds `skillgen` as a dependency.
2. Author registers `NewSkillsCmd(root)` on their root command.
3. In CI or a Makefile, `mytool skills generate --dir ./.claude/skills` runs on every build.
4. Generated files are checked in (or published as a release artifact) so consumers get them without running the tool.

## 10. Open questions

- **Non-cobra frameworks** — recommend cobra-only for v1, extract a small `Commander` interface once the shape stabilizes.
- **Skill-format evolution** — agents may add fields (e.g. `allowed-tools`, `model`); plan for an opt-in field set rather than baking one schema in.
- **Install story** — embed installation instructions inside the single skill (or overview skill in split mode), or ship a separate `skills install` helper? Leaning toward embedding to keep the library install-agnostic.
- **Split-mode heuristics** — should the library suggest split mode when a CLI crosses some size threshold, or leave it fully to the author? Leaning author-only until we see real usage.

## 11. Milestones

1. **M1** — Core generator (single-skill mode) + cobra integration + Claude Code-compatible output.
2. **M2** — Annotation-based augmentation + template override hook.
3. **M3** — `skills generate` subcommand, stable filenames, validation.
4. **M4** — Split mode (one skill per leaf + optional overview) for large CLIs.
5. **M5** — Example repo and docs showing both single and split modes with generated skills checked in.

## 12. Success criteria

- A cobra-based CLI can produce a working, agent-usable skill set in under 10 lines of integration code.
- Regenerating skills on an unchanged command tree produces a byte-identical diff.
- At least one external CLI adopts `skillgen` and reports that agents invoke it correctly based on the generated skills.
