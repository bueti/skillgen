package skillgen

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/bueti/skilllint"
	_ "github.com/bueti/skilllint/rules" // register the built-in spec rules

	"github.com/spf13/cobra"
)

// Lint walks the command tree and lints the resulting skills. Findings
// combine two passes:
//
//  1. Cobra-tree rules ("cmd-*") that inspect cobra metadata — these produce
//     findings keyed by command path, which is more actionable for authors
//     than the generated skill path.
//  2. Spec-compliance rules from the skilllint library, run against each
//     generated skill's bytes. These cover name format, description length,
//     body size, vague descriptions, and so on.
//
// The generator's skip rules (Hidden, skill.skip, WithSkip predicate,
// cobra builtins) apply in both passes so lint stays in sync with what the
// generator would actually emit.
func (g *Generator) Lint() []skilllint.Issue {
	if g.root == nil {
		return []skilllint.Issue{{
			Rule:     "cmd-root",
			Severity: skilllint.SeverityError,
			Source:   "",
			Field:    "root",
			Message:  "Generator has a nil root command",
		}}
	}

	var issues []skilllint.Issue
	g.lintCommand(g.root, true, &issues)

	// Spec-compliance pass: run skilllint on every generated skill. The
	// library handles name regex, length caps, description quality, trigger
	// presence, and body size. We disable the filesystem-touching rules
	// because the skill bytes haven't been written to disk yet.
	if specIssues, err := g.lintSpec(); err != nil {
		issues = append(issues, skilllint.Issue{
			Rule:     "cmd-generation",
			Severity: skilllint.SeverityError,
			Source:   g.root.CommandPath(),
			Field:    "generation",
			Message:  fmt.Sprintf("skill generation failed during lint: %v", err),
		})
	} else {
		issues = append(issues, specIssues...)
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Source != issues[j].Source {
			return issues[i].Source < issues[j].Source
		}
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity > issues[j].Severity // errors before warnings
		}
		return issues[i].Rule < issues[j].Rule
	})
	return issues
}

// lintSpec renders the generator's skills and delegates validation to
// skilllint. The filesystem rules (script-exists, orphaned-file) are
// disabled because skillgen hasn't written the skill directory yet, and
// name-matches-dir is redundant (skillgen constructs both from the same
// slug).
func (g *Generator) lintSpec() ([]skilllint.Issue, error) {
	skills, err := g.Skills()
	if err != nil {
		return nil, err
	}
	off := skilllint.SeverityOff
	cfg := skilllint.Config{
		Rules: map[string]skilllint.RuleConfig{
			"script-exists":    {Severity: &off},
			"orphaned-file":    {Severity: &off},
			"name-matches-dir": {Severity: &off},
		},
	}
	linter := skilllint.New(skilllint.WithConfig(cfg))

	var out []skilllint.Issue
	for _, s := range skills {
		// Use a virtual path so Source in the output identifies the skill.
		// No file is read from disk.
		virtualSrc := path.Join(s.Name, "SKILL.md")
		found, lerr := linter.LintBytes(virtualSrc, s.Bytes())
		if lerr != nil {
			return nil, lerr
		}
		out = append(out, found...)
	}
	return out, nil
}

func (g *Generator) lintCommand(c *cobra.Command, isRoot bool, out *[]skilllint.Issue) {
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
		*out = append(*out, skilllint.Issue{
			Rule:     "cmd-description-missing",
			Severity: skilllint.SeverityError,
			Source:   path,
			Field:    "description",
			Message:  fmt.Sprintf("no Short, Long, or %q annotation — the generated skill will have an empty description", AnnotationDescription),
		})
	} else if short := strings.TrimSpace(c.Short); short != "" && len(short) < 12 {
		*out = append(*out, skilllint.Issue{
			Rule:     "cmd-short-brief",
			Severity: skilllint.SeverityWarning,
			Source:   path,
			Field:    "short",
			Message:  fmt.Sprintf("Short (%q) is very brief; agents match skills by description text, so a few more words help discoverability", short),
		})
	}

	// Trigger signal — recommended on root and every leaf so the agent has
	// keywords beyond whatever the description happens to contain.
	if isRoot || isLeaf {
		hasTrigger := strings.TrimSpace(c.Annotations[AnnotationTrigger]) != "" || len(c.Aliases) > 0
		if !hasTrigger {
			*out = append(*out, skilllint.Issue{
				Rule:     "cmd-trigger-missing",
				Severity: skilllint.SeverityWarning,
				Source:   path,
				Field:    "trigger",
				Message:  fmt.Sprintf("no %q annotation and no Aliases — agent matching relies solely on the description", AnnotationTrigger),
			})
		}
	}

	// Deprecated commands should carry a helpful message.
	if dep := strings.TrimSpace(c.Deprecated); dep != "" && len(dep) < 12 {
		*out = append(*out, skilllint.Issue{
			Rule:     "cmd-deprecated-short",
			Severity: skilllint.SeverityWarning,
			Source:   path,
			Field:    "deprecated",
			Message:  fmt.Sprintf("Deprecated message is very short (%q); agents benefit from knowing the replacement command", dep),
		})
	}

	// Operator / daemon subtree — names ending -operator, -daemon, or -runner
	// usually indicate internal server commands an agent shouldn't surface.
	if isOperatorName(c.Name()) {
		*out = append(*out, skilllint.Issue{
			Rule:     "cmd-operator-suffix",
			Severity: skilllint.SeverityWarning,
			Source:   path,
			Field:    "name",
			Message:  fmt.Sprintf("name ends in -operator/-daemon/-runner — if this is an internal server command, set the %q annotation to exclude it", AnnotationSkip),
		})
	}

	// Path depth — deep nesting is usually a smell.
	if depth := strings.Count(path, " "); depth >= maxRecommendedDepth {
		*out = append(*out, skilllint.Issue{
			Rule:     "cmd-depth",
			Severity: skilllint.SeverityWarning,
			Source:   path,
			Field:    "depth",
			Message:  fmt.Sprintf("%d levels deep; consider flattening — nested paths are harder for agents to match", depth),
		})
	}

	// Sibling description-length variance.
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

// maxRecommendedDepth is the path-space count at or above which depth is
// flagged. "tool a b c" has 3 spaces — 3 levels deep. We warn at 4.
const maxRecommendedDepth = 4

// lintSiblingVariance returns a warning when siblings have wildly different
// description lengths (max > 3x min), which indicates asymmetric quality.
func lintSiblingVariance(parentPath string, siblings []*cobra.Command) *skilllint.Issue {
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
	return &skilllint.Issue{
		Rule:     "cmd-sibling-variance",
		Severity: skilllint.SeverityWarning,
		Source:   parentPath,
		Field:    "sibling-variance",
		Message:  fmt.Sprintf("subcommand description lengths vary widely (%d..%d chars) — consider levelling up the shorter siblings so agents get symmetric detail", minLen, maxLen),
	}
}
