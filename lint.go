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

	// Operator / daemon subtree — names ending -operator, -daemon, or -runner
	// usually indicate internal server commands an agent shouldn't surface. A
	// false positive is trivial for the author to silence with skill.skip.
	if isOperatorName(c.Name()) {
		*out = append(*out, Issue{
			Level:   IssueWarning,
			Command: path,
			Field:   "operator-subtree",
			Message: fmt.Sprintf("name ends in -operator/-daemon/-runner — if this is an internal server command, set the %q annotation to exclude it", AnnotationSkip),
		})
	}

	// Path depth — deep nesting is usually a smell; agents have trouble
	// holding 4-level command paths in trigger descriptions.
	if depth := strings.Count(path, " "); depth >= maxRecommendedDepth {
		*out = append(*out, Issue{
			Level:   IssueWarning,
			Command: path,
			Field:   "depth",
			Message: fmt.Sprintf("%d levels deep; consider flattening — nested paths are harder for agents to match", depth),
		})
	}

	// Sibling Short-length variance — if one sibling has a 400-char Long and
	// another a 10-word Short, the skill quality is uneven. Warn the parent.
	if len(visibleChildren) >= 2 {
		if iss := lintSiblingVariance(path, visibleChildren); iss != nil {
			*out = append(*out, *iss)
		}
	}

	for _, sub := range visibleChildren {
		g.lintCommand(sub, false, out)
	}
}

// isOperatorName reports whether c's leaf name looks like an internal
// server/operator command that probably shouldn't be in skill output.
func isOperatorName(name string) bool {
	for _, suffix := range []string{"-operator", "-daemon", "-runner"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// maxRecommendedDepth is the path-space count at or above which depth is flagged.
// "tool a b c" has 3 spaces — 3 levels deep. We warn at 4.
const maxRecommendedDepth = 4

// lintSiblingVariance returns a warning when siblings have wildly different
// description lengths (max > 3x min), which tends to indicate some siblings
// got careful Longs and others got a perfunctory Short.
func lintSiblingVariance(parentPath string, siblings []*cobra.Command) *Issue {
	lengths := make([]int, 0, len(siblings))
	for _, s := range siblings {
		effective := strings.TrimSpace(s.Long)
		if effective == "" {
			effective = strings.TrimSpace(s.Short)
		}
		if effective == "" {
			continue
		}
		lengths = append(lengths, len(effective))
	}
	if len(lengths) < 2 {
		return nil
	}
	minLen, maxLen := lengths[0], lengths[0]
	for _, l := range lengths[1:] {
		if l < minLen {
			minLen = l
		}
		if l > maxLen {
			maxLen = l
		}
	}
	if minLen == 0 || maxLen < minLen*3 {
		return nil
	}
	// Skip noise when the "short" sibling is already a reasonable Short.
	if minLen >= 40 {
		return nil
	}
	return &Issue{
		Level:   IssueWarning,
		Command: parentPath,
		Field:   "sibling-variance",
		Message: fmt.Sprintf("subcommand description lengths vary widely (%d..%d chars) — consider levelling up the shorter siblings so agents get symmetric detail", minLen, maxLen),
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
