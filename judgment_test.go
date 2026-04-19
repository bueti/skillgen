package skillgen

import (
	"strings"
	"testing"

	"github.com/bueti/skilllint"
	"github.com/spf13/cobra"
)

func TestAliasesRenderedAndDriveTrigger(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "Mytool ships services"}
	deploy := &cobra.Command{
		Use:     "deploy <service>",
		Short:   "Deploy a service",
		Aliases: []string{"ship", "release"},
	}
	root.AddCommand(deploy)

	skills, err := New(root).Skills()
	if err != nil {
		t.Fatal(err)
	}
	body := skills[0].Body
	if !strings.Contains(body, "Aliases: `ship`, `release`") {
		t.Errorf("aliases line missing:\n%s", body)
	}

	// Leaf mode: description on the per-leaf skill should auto-derive a trigger
	// from command name + aliases when skill.trigger isn't set.
	leafSkills, err := New(root, WithSplit(SplitPerLeaf)).Skills()
	if err != nil {
		t.Fatal(err)
	}
	var deploySkill Skill
	for _, s := range leafSkills {
		if s.Name == "mytool-deploy" {
			deploySkill = s
			break
		}
	}
	if !strings.Contains(deploySkill.Description, "Use when the user asks to deploy, ship, or release") {
		t.Errorf("aliases did not produce an auto-trigger:\n%q", deploySkill.Description)
	}
}

func TestExplicitTriggerBeatsAliases(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "mytool",
		Short:   "x",
		Aliases: []string{"alt"},
		Annotations: map[string]string{
			AnnotationTrigger: "ship",
		},
	}
	skills, _ := New(cmd).Skills()
	if strings.Contains(skills[0].Description, "mytool or alt") {
		t.Errorf("alias trigger leaked when skill.trigger was set: %q", skills[0].Description)
	}
	if !strings.Contains(skills[0].Description, "Use when the user asks to ship") {
		t.Errorf("explicit trigger not honored: %q", skills[0].Description)
	}
}

func TestDeprecatedCommandRendersCallout(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "x"}
	old := &cobra.Command{
		Use:        "old-deploy",
		Short:      "Legacy deploy",
		Deprecated: "use `mytool deploy` instead",
	}
	root.AddCommand(old)

	skills, _ := New(root).Skills()
	if !strings.Contains(skills[0].Body, "> **Deprecated:** use `mytool deploy` instead") {
		t.Errorf("deprecated callout missing:\n%s", skills[0].Body)
	}
}

func TestDeprecatedFlagFiltered(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "x"}
	root.Flags().String("old-flag", "", "legacy option")
	_ = root.Flags().MarkDeprecated("old-flag", "use --new-flag")

	skills, _ := New(root).Skills()
	if strings.Contains(skills[0].Body, "old-flag") {
		t.Errorf("deprecated flag leaked into body:\n%s", skills[0].Body)
	}
}

func TestAvoidAnnotationRendered(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationAvoid: "Do not run with `--force` on production.",
		},
	}
	skills, _ := New(root).Skills()
	if !strings.Contains(skills[0].Body, "## Avoid") {
		t.Errorf("Avoid heading missing:\n%s", skills[0].Body)
	}
	if !strings.Contains(skills[0].Body, "Do not run with `--force` on production.") {
		t.Errorf("Avoid content missing:\n%s", skills[0].Body)
	}
}

func TestPreferOverAnnotationRendered(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationPreferOver: "Use instead of raw `kubectl delete`.",
		},
	}
	skills, _ := New(root).Skills()
	if !strings.Contains(skills[0].Body, "## Prefer over") {
		t.Errorf("Prefer over heading missing:\n%s", skills[0].Body)
	}
	if !strings.Contains(skills[0].Body, "Use instead of raw `kubectl delete`.") {
		t.Errorf("Prefer over content missing:\n%s", skills[0].Body)
	}
}

func TestSubcommandAvoidRendersInline(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "x"}
	deploy := &cobra.Command{
		Use:   "deploy",
		Short: "deploy the service",
		Annotations: map[string]string{
			AnnotationAvoid: "Don't skip --dry-run in prod.",
		},
	}
	root.AddCommand(deploy)

	skills, _ := New(root).Skills()
	if !strings.Contains(skills[0].Body, "**Avoid:** Don't skip --dry-run in prod.") {
		t.Errorf("inline Avoid label missing:\n%s", skills[0].Body)
	}
}

func TestLintFlagsMissingDescription(t *testing.T) {
	root := &cobra.Command{Use: "mytool"}
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Source == "mytool" && iss.Field == "description" && iss.Severity == skilllint.SeverityError {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-description error:\n%v", issues)
	}
}

func TestLintFlagsShortShort(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "tiny"}
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Field == "short" && iss.Severity == skilllint.SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("expected short-too-brief warning:\n%v", issues)
	}
}

func TestLintFlagsMissingTrigger(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Field == "trigger" && iss.Severity == skilllint.SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-trigger warning:\n%v", issues)
	}
}

func TestLintAliasesSatisfyTrigger(t *testing.T) {
	root := &cobra.Command{
		Use:     "mytool",
		Short:   "A reasonable description here",
		Aliases: []string{"mt"},
	}
	issues := New(root).Lint()
	for _, iss := range issues {
		if iss.Field == "trigger" {
			t.Errorf("aliases should satisfy trigger check: %v", iss)
		}
	}
}

func TestLintDeprecatedWithoutReplacement(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	old := &cobra.Command{
		Use:        "old",
		Short:      "Legacy command that should be removed",
		Deprecated: "yes",
	}
	root.AddCommand(old)
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Source == "mytool old" && iss.Field == "deprecated" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected deprecated-too-short warning on child:\n%v", issues)
	}
}

func TestLintRespectsSkip(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	skipped := &cobra.Command{
		Use:         "internal",
		Annotations: map[string]string{AnnotationSkip: "true"},
	}
	root.AddCommand(skipped)
	issues := New(root).Lint()
	for _, iss := range issues {
		if strings.HasPrefix(iss.Source, "mytool internal") {
			t.Errorf("skipped command %q should not produce lint findings: %v", iss.Source, iss)
		}
	}
}

func TestLintSortOrder(t *testing.T) {
	root := &cobra.Command{Use: "mytool"} // missing description → error on root
	sub := &cobra.Command{Use: "sub"}     // missing description → error on sub
	root.AddCommand(sub)

	issues := New(root).Lint()
	if len(issues) < 2 {
		t.Fatalf("expected multiple issues, got %d", len(issues))
	}
	// root sorts before "mytool sub"
	if issues[0].Source != "mytool" {
		t.Errorf("want first issue on root, got %q", issues[0].Source)
	}
}

// FormatIssues moved to skilllint.Write — covered by skilllint's own tests.
