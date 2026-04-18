package skillgen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// IssueLevel classifies a lint finding.
type IssueLevel int

const (
	// IssueWarning marks a finding the author may want to fix for skill quality
	// but that doesn't break generation.
	IssueWarning IssueLevel = iota

	// IssueError marks a finding that would produce a broken or useless skill.
	IssueError
)

// String returns a short label for the level.
func (l IssueLevel) String() string {
	switch l {
	case IssueError:
		return "error"
	case IssueWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// Issue is a single finding from Lint.
type Issue struct {
	Level   IssueLevel
	Command string // command path, e.g. "mytool deploy"
	Field   string // "description", "trigger", "short", "deprecated", ...
	Message string
}

// String formats the issue for text output.
func (i Issue) String() string {
	where := i.Command
	if where == "" {
		where = "(root)"
	}
	return fmt.Sprintf("%s\t%s\t%s: %s", i.Level, where, i.Field, i.Message)
}

// Lint walks the command tree and returns quality findings. The Generator's
// skip rules (Hidden, skill.skip, WithSkip predicate, cobra builtins) apply
// so linting and generation stay in sync.
func (g *Generator) Lint() []Issue {
	if g.root == nil {
		return []Issue{{
			Level:   IssueError,
			Command: "",
			Field:   "root",
			Message: "Generator has a nil root command",
		}}
	}

	var issues []Issue
	g.lintCommand(g.root, true, &issues)

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Command != issues[j].Command {
			return issues[i].Command < issues[j].Command
		}
		if issues[i].Level != issues[j].Level {
			return issues[i].Level > issues[j].Level // errors before warnings
		}
		return issues[i].Field < issues[j].Field
	})
	return issues
}

func (g *Generator) lintCommand(c *cobra.Command, isRoot bool, out *[]Issue) {
	if !isRoot && g.shouldSkip(c) {
		return
	}

	var visibleChildren []*cobra.Command
	for _, sub := range c.Commands() {
		if !g.shouldSkip(sub) {
			visibleChildren = append(visibleChildren, sub)
		}
	}
	isLeaf := len(visibleChildren) == 0

	path := c.CommandPath()

	// Description — every command that will end up in a skill needs one.
	hasDesc := strings.TrimSpace(c.Annotations[AnnotationDescription]) != "" ||
		strings.TrimSpace(c.Short) != "" ||
		strings.TrimSpace(c.Long) != ""
	if !hasDesc {
		*out = append(*out, Issue{
			Level:   IssueError,
			Command: path,
			Field:   "description",
			Message: fmt.Sprintf("no Short, Long, or %q annotation — the generated skill will have an empty description", AnnotationDescription),
		})
	} else if short := strings.TrimSpace(c.Short); short != "" && len(short) < 12 {
		*out = append(*out, Issue{
			Level:   IssueWarning,
			Command: path,
			Field:   "short",
			Message: fmt.Sprintf("Short (%q) is very brief; agents match skills by description text, so a few more words help discoverability", short),
		})
	}

	// Trigger signal — recommended on root and on every leaf so the agent has
	// keywords beyond whatever the description happens to contain.
	if isRoot || isLeaf {
		hasTrigger := strings.TrimSpace(c.Annotations[AnnotationTrigger]) != "" || len(c.Aliases) > 0
		if !hasTrigger {
			*out = append(*out, Issue{
				Level:   IssueWarning,
				Command: path,
				Field:   "trigger",
				Message: fmt.Sprintf("no %q annotation and no Aliases — agent matching relies solely on the description", AnnotationTrigger),
			})
		}
	}

	// Deprecated commands should carry a helpful message.
	if dep := strings.TrimSpace(c.Deprecated); dep != "" && len(dep) < 12 {
		*out = append(*out, Issue{
			Level:   IssueWarning,
			Command: path,
			Field:   "deprecated",
			Message: fmt.Sprintf("Deprecated message is very short (%q); agents benefit from knowing the replacement command", dep),
		})
	}

	for _, sub := range visibleChildren {
		g.lintCommand(sub, false, out)
	}
}

// FormatIssues returns a human-readable, multi-line summary of issues.
func FormatIssues(issues []Issue) string {
	if len(issues) == 0 {
		return "no issues\n"
	}
	var b strings.Builder
	var errs, warns int
	for _, iss := range issues {
		b.WriteString(iss.String())
		b.WriteByte('\n')
		switch iss.Level {
		case IssueError:
			errs++
		case IssueWarning:
			warns++
		}
	}
	fmt.Fprintf(&b, "\n%d error(s), %d warning(s)\n", errs, warns)
	return b.String()
}
