# skillgen

Generate agent skill files from a [cobra](https://github.com/spf13/cobra) command tree — the way cobra itself generates shell tab completions.

`skillgen` emits a Markdown file with YAML frontmatter that teaches an AI coding agent (Claude Code or compatible) when the CLI is relevant and how to invoke it. The output is derived from the command tree, so it stays in sync with the CLI instead of drifting like a hand-written skill would.

## Install

```sh
go get github.com/bueti/skillgen
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

| Annotation          | Effect                                                                         |
| ------------------- | ------------------------------------------------------------------------------ |
| `skill.trigger`     | Appends `"Use when the user asks to <trigger>."` to the description.           |
| `skill.description` | Replaces the generated description entirely.                                   |
| `skill.name`        | Overrides the skill name (root only in single-skill mode).                     |
| `skill.skip`        | `"true"` to exclude the command and its subtree.                               |
| `skill.examples`    | Free-form Markdown appended to the command's body section.                     |
| `skill.avoid`       | Renders as an **Avoid** section — tell the agent what *not* to do.             |
| `skill.prefer-over` | Renders as a **Prefer over** section — point the agent away from alternatives. |

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

## Linting

`skills lint` walks the same tree the generator walks and reports missing or low-quality signal — empty descriptions, overly short `Short` text, leaves without trigger hints, and deprecated commands without a helpful message. Errors produce exit code 1; `--strict` promotes warnings to errors too.

```sh
mytool skills lint                 # report, but don't fail on warnings
mytool skills lint --strict        # CI-friendly: any finding is a hard fail
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

Filenames are slugged from the full command path — `mytool deploy` becomes `mytool-deploy.md`, `mytool config set` becomes `mytool-config-set.md`. The optional overview skill uses the root name (`mytool.md`) and lists every leaf with a short description.

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
