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

| Annotation          | Effect                                                               |
| ------------------- | -------------------------------------------------------------------- |
| `skill.trigger`     | Appends `"Use when the user asks to <trigger>."` to the description. |
| `skill.description` | Replaces the generated description entirely.                         |
| `skill.name`        | Overrides the skill name (root only in single-skill mode).           |
| `skill.skip`        | `"true"` to exclude the command and its subtree.                     |
| `skill.examples`    | Free-form Markdown appended to the command's body section.           |

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

| Option                  | Purpose                                                            |
| ----------------------- | ------------------------------------------------------------------ |
| `WithSplit(mode)`       | `SplitNone` (default) or `SplitPerLeaf` (planned).                 |
| `WithOverview(bool)`    | Emit an overview skill in split mode.                              |
| `WithTemplate(t)`       | Replace the default body renderer with a `text/template.Template`. |
| `WithFilenamePrefix(p)` | Prepend a prefix to every generated filename.                      |
| `WithSkip(pred)`        | Custom predicate for excluding commands.                           |
| `WithIncludeBuiltins()` | Keep cobra's auto-injected `help` / `completion` in the output.    |

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

## Status

- **M1** — single-skill mode, cobra integration, Claude Code-compatible frontmatter ✅
- **M2** — annotations and template override hook ✅
- **M3** — `skills generate` / `skills print` subcommands, stable filenames, validation ✅
- **M4** — split mode (one skill per leaf) — planned

See [`PRD.md`](./PRD.md) for the full design.

## License

[MIT](./LICENSE)
