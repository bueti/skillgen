# skillgen

Generate agent skill files from a [cobra](https://github.com/spf13/cobra) command tree — the way cobra itself generates shell tab completions.

`skillgen` emits a Markdown file with YAML frontmatter that teaches an AI coding agent (Claude Code or compatible) when the CLI is relevant and how to invoke it. The output is derived from the command tree, so it stays in sync with the CLI instead of drifting like a hand-written skill would.

## Install

```sh
go get github.com/bueti/skillgen
```

## Output layout

skillgen writes to the [agentskills.io specification](https://agentskills.io/specification) layout — each skill is its own directory containing a `SKILL.md` file:

```
.claude/skills/
└── mytool/
    └── SKILL.md
```

In split mode, one directory per leaf:

```
.claude/skills/
├── mytool/           # optional overview
│   └── SKILL.md
├── mytool-build/
│   └── SKILL.md
└── mytool-deploy/
    └── SKILL.md
```

## Quick start

Add a `skills` subcommand to your root command — same shape as cobra's `completion`:

```go
package main

import (
    "github.com/bueti/skillgen"
    "github.com/spf13/cobra"
)

func main() {
    root := &cobra.Command{
        Use:   "mytool",
        Short: "Build and deploy mytool services",
    }
    // ... register your subcommands ...

    root.AddCommand(skillgen.NewSkillsCmd(root))
    _ = root.Execute()
}
```

Then:

```sh
mytool skills print                       # write to stdout
mytool skills generate --dir .claude/skills  # write to disk
```

The generated file is deterministic, so you can check it into git and regenerate in CI.

## Programmatic use

```go
gen := skillgen.New(root,
    skillgen.WithFilenamePrefix("acme-"),
    skillgen.WithSkip(func(c *cobra.Command) bool {
        return c.Name() == "debug"
    }),
)
if err := gen.WriteTo("./.claude/skills"); err != nil {
    log.Fatal(err)
}
```

## Enriching generated skills

Any cobra command can carry annotations that shape its output:

| Annotation              | Effect                                                                             |
| ----------------------- | ---------------------------------------------------------------------------------- |
| `skill.trigger`         | Accepts a fragment (`"deploy, ship"`) or a full sentence (`"Use when…"`).          |
| `skill.description`     | Replaces the generated description entirely.                                       |
| `skill.name`            | Overrides the skill name. Validated against the spec regex.                        |
| `skill.skip`            | `"true"` to exclude the command and its subtree.                                   |
| `skill.examples`        | Free-form Markdown appended to the command's body section.                         |
| `skill.avoid`           | Renders as an **Avoid** section — tell the agent what *not* to do.                 |
| `skill.prefer-over`     | Renders as a **Prefer over** section — point the agent away from alternatives.     |
| `skill.license`         | Populates the spec `license` frontmatter field (e.g. `"Apache-2.0"`).              |
| `skill.compatibility`   | Populates the spec `compatibility` field — max 500 chars, environment reqs only.   |
| `skill.metadata.<key>`  | Prefix pattern that populates the spec `metadata:` map. Keys emitted sorted.       |
| `skill.allowed-tools`   | Only under `TargetClaudeCode` — populates the `allowed-tools` frontmatter field.   |

```go
deploy := &cobra.Command{
    Use:   "deploy <service>",
    Short: "Deploy a built artifact to an environment",
    Annotations: map[string]string{
        skillgen.AnnotationTrigger:  "deploy, promote, ship, or release a service",
        skillgen.AnnotationExamples: "Tip: pair with `--dry-run` to preview before applying.",
    },
}
```

## Options

| Option                  | Purpose                                                                |
| ----------------------- | ---------------------------------------------------------------------- |
| `WithSplit(mode)`       | `SplitNone` (default, single skill) or `SplitPerLeaf` (one per leaf).  |
| `WithOverview(bool)`    | In split mode, also emit an overview skill that lists the leaves.      |
| `WithTemplate(t)`       | Replace the single-mode body renderer with a `text/template.Template`. |
| `WithFilenamePrefix(p)` | Prepend a prefix to every generated filename.                          |
| `WithSkip(pred)`        | Custom predicate for excluding commands.                               |
| `WithIncludeBuiltins()` | Keep cobra's auto-injected `help` / `completion` in the output.        |
| `WithTarget(t)`         | `TargetGeneric` (default) or `TargetClaudeCode` for richer frontmatter.|

By default, cobra's built-in `help` and `completion` subcommands are filtered out because agents don't need them. User-defined commands with those names at deeper levels are _not_ filtered.

## Example output

From the example CLI in `./example`:

```markdown
---
name: "mytool"
description: "Build and deploy mytool services. Use when the user asks to build, deploy, promote, ship, or release a mytool service."
---

# mytool

mytool is a small example CLI used to demonstrate agent-skill generation via the skillgen library.

## When to use

Build and deploy mytool services. Use when the user asks to build, deploy, promote, ship, or release a mytool service.

## Commands

### `mytool build`

Build an artifact of a service

Usage: `mytool build <service> [flags]`

Flags:

- `--push` — push the built image after building
- `--tag` — image tag to apply (default `latest`)

…
```

Try it:

```sh
go run ./example skills print
```

## What's mined from cobra

Before consulting annotations, skillgen extracts free signal straight from the command tree so you don't have to duplicate it:

- `cmd.Aliases` → rendered as `Aliases: …` and, when `skill.trigger` is unset, auto-derives trigger phrases ("Use when the user asks to deploy, ship, or release").
- `cmd.Deprecated` → renders a prominent `> **Deprecated:** …` callout so agents know to avoid the command and see the replacement.
- `flag.Deprecated` → deprecated flags are filtered from the rendered flag list entirely; the author already said the flag shouldn't be suggested.

Annotations *augment* this mined signal rather than replace it — an alias list alone is often enough to skip writing `skill.trigger` yourself.

## Sibling collapse

When a parent command's visible children all accept exactly the same local flags (name, shorthand, type, required-ness, and usage text), skillgen hoists the flag table up to the parent and omits the per-child repetition:

```markdown
### `mytool actions`

Shared subcommand flags (apply to every subcommand below):

- `-p, --instances` (required) — target instances
- `-r, --reason` (required) — justification for the action

#### `mytool actions cycle`
Move past a bad node.

#### `mytool actions triage`
Preserve for investigation.
…
```

For a CLI with six siblings taking the same two flags, that's ~45 lines of duplication gone. A single differing flag across siblings disables the collapse — the agent shouldn't ever see a flag list that's subtly wrong. Split mode renders each leaf standalone, so collapse doesn't apply there.

## Targets

Default output is a minimal, interoperable frontmatter (`name` + `description`). Opt into a richer host-specific shape with `WithTarget`:

```go
gen := skillgen.New(root, skillgen.WithTarget(skillgen.TargetClaudeCode))
```

Under `TargetClaudeCode`, the `skill.allowed-tools` annotation populates the Claude Code `allowed-tools` frontmatter field:

```go
deploy.Annotations[skillgen.AnnotationAllowedTools] = "Bash, Read, Edit"
// → allowed-tools: "Bash, Read, Edit" in the generated frontmatter.
```

Targets other than the minimal default are additive — they never strip standard keys, only extend.

## Linting

`skills lint` runs two passes. The first walks the cobra command tree for checks keyed by *command path* (more actionable than a generated file path): missing descriptions, overly short `Short` text, leaves without trigger hints, deprecated commands without a helpful message, operator-suffix names, deep nesting, sibling-description variance. These rules are prefixed `cmd-` in the output.

The second pass delegates to [`skilllint`](https://github.com/bueti/skilllint) — skillgen generates each SKILL.md in memory and runs skilllint's built-in rules against it: spec hard limits (name format, description length, compatibility length) as errors, and spec soft limits + quality checks (body tokens, body lines, vague descriptions, heading-level jumps, trigger phrasing) as warnings. Errors produce exit code 1; `--strict` promotes warnings to errors too.

```sh
mytool skills lint                              # report, but don't fail on warnings
mytool skills lint --strict                     # CI-friendly: any finding is a hard fail
mytool skills lint --format=json                # machine-readable
mytool skills lint --format=github-actions      # inline annotations on GitHub PRs
```

A minimal GitHub Actions step:

```yaml
- name: Lint skills
  run: go run ./your-cli skills lint --strict
```

## Split mode

Default output is a single skill covering the whole CLI. Large tools with many independent subcommands may prefer **split mode**: one skill per leaf command so agents can load only the one that matches.

```go
gen := skillgen.New(root,
    skillgen.WithSplit(skillgen.SplitPerLeaf),
    skillgen.WithOverview(true), // optional: also emit a root overview
)
```

Each skill lands in its own directory — `mytool deploy` → `mytool-deploy/SKILL.md`, `mytool config set` → `mytool-config-set/SKILL.md`. The optional overview skill uses the root name (`mytool/SKILL.md`) and lists every leaf with a short description.

Rule of thumb: ≤ ~10 commands → single mode; dozens of commands or independent command groups → split mode.

## Releasing

Releases are cut by pushing a semver tag to `main`:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The `Release` workflow verifies the tag lives on `main`, runs the test suite, and drafts a GitHub Release with auto-generated notes. `proxy.golang.org` picks up the tag automatically, so `go get github.com/bueti/skillgen@v0.1.0` works immediately.

Pre-release tags (e.g. `v0.2.0-rc.1`) are marked as pre-releases on GitHub.

## Status

- **M1** — single-skill mode, cobra integration, Claude Code-compatible frontmatter ✅
- **M2** — annotations and template override hook ✅
- **M3** — `skills generate` / `skills print` subcommands, stable filenames, validation ✅
- **M4** — split mode (one skill per leaf + optional overview) ✅

See [`PRD.md`](./PRD.md) for the full design and [`CHANGELOG.md`](./CHANGELOG.md) for per-version changes.

## License

[MIT](./LICENSE)
