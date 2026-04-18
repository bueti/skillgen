package skillgen

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// --- #1: "When to use" section honesty ---

func TestWhenToUseOmittedWithoutTriggerSignal(t *testing.T) {
	// Root with Short but no trigger, no aliases, no visible children.
	root := &cobra.Command{Use: "solo", Short: "A solitary CLI"}
	skills, err := New(root).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(skills[0].Body, "## When to use") {
		t.Errorf("When-to-use section should be omitted without trigger signal:\n%s", skills[0].Body)
	}
	// Frontmatter description should still carry the Short.
	if !strings.Contains(skills[0].Description, "A solitary CLI") {
		t.Errorf("description missing Short: %q", skills[0].Description)
	}
}

func TestWhenToUseContainsOnlyTriggerClause(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "Mytool runs things",
		Annotations: map[string]string{
			AnnotationTrigger: "run, execute, or launch something",
		},
	}
	skills, _ := New(root).Skills()
	body := skills[0].Body
	// Section present.
	if !strings.Contains(body, "## When to use") {
		t.Fatalf("When-to-use section missing with trigger:\n%s", body)
	}
	// Section contains ONLY the "Use when…" clause, not the Short too.
	after := body[strings.Index(body, "## When to use"):]
	if strings.Count(after, "Mytool runs things") > 0 {
		t.Errorf("When-to-use section duplicated the Short:\n%s", after)
	}
	if !strings.Contains(after, "Use when the user asks to run, execute, or launch something.") {
		t.Errorf("When-to-use missing trigger clause:\n%s", after)
	}
}

func TestRootTriggerSynthesizedFromChildren(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "mytool does things"}
	root.AddCommand(&cobra.Command{Use: "dev", Short: "dev stuff"})
	root.AddCommand(&cobra.Command{Use: "gpu", Short: "gpu stuff"})
	root.AddCommand(&cobra.Command{Use: "k8s", Short: "k8s stuff"})

	skills, err := New(root).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skills[0].Description, "Use when the user asks about dev, gpu, or k8s.") {
		t.Errorf("child-name synthesis missing: %q", skills[0].Description)
	}
}

func TestLeafDoesNotSynthesizeFromOwnName(t *testing.T) {
	// In split mode, a leaf with no trigger/aliases must NOT get a synthesized
	// trigger — there are no children to derive from, and echoing the command
	// name would be noise.
	root := &cobra.Command{Use: "mytool", Short: "x"}
	leaf := &cobra.Command{Use: "deploy", Short: "Deploy it"}
	root.AddCommand(leaf)

	skills, _ := New(root, WithSplit(SplitPerLeaf)).Skills()
	var deploy Skill
	for _, s := range skills {
		if s.Name == "mytool-deploy" {
			deploy = s
		}
	}
	if deploy.Name == "" {
		t.Fatal("no mytool-deploy skill")
	}
	if strings.Contains(deploy.Description, "Use when the user asks") {
		t.Errorf("leaf description should not have auto-trigger: %q", deploy.Description)
	}
	if strings.Contains(deploy.Body, "## When to use") {
		t.Errorf("leaf body should omit When-to-use without trigger signal:\n%s", deploy.Body)
	}
}

// --- trigger normalization (bug: undocumented template contract) ---

func TestTriggerFragmentWrapped(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationTrigger: "deploy, ship, or release",
		},
	}
	skills, _ := New(root).Skills()
	if !strings.Contains(skills[0].Description, "Use when the user asks to deploy, ship, or release.") {
		t.Errorf("fragment trigger not wrapped correctly: %q", skills[0].Description)
	}
}

func TestTriggerFullSentenceUsedAsIs(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationTrigger: "Use when the user asks to deploy the service.",
		},
	}
	skills, _ := New(root).Skills()
	// Must not double-prefix.
	if strings.Contains(skills[0].Description, "Use when the user asks to Use when") {
		t.Errorf("full-sentence trigger was double-prefixed: %q", skills[0].Description)
	}
	// Must preserve the sentence.
	if !strings.Contains(skills[0].Description, "Use when the user asks to deploy the service.") {
		t.Errorf("full-sentence trigger not preserved: %q", skills[0].Description)
	}
}

func TestTriggerFullSentenceWithoutPeriodGetsOne(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationTrigger: "Use when the user asks to deploy the service",
		},
	}
	skills, _ := New(root).Skills()
	if !strings.Contains(skills[0].Description, "deploy the service.") {
		t.Errorf("full-sentence trigger missing trailing period: %q", skills[0].Description)
	}
}

func TestTriggerFragmentWithTrailingPeriod(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationTrigger: "deploy the service.",
		},
	}
	skills, _ := New(root).Skills()
	// No "...". double period.
	if strings.Contains(skills[0].Description, "..") {
		t.Errorf("fragment with trailing period produced double period: %q", skills[0].Description)
	}
	if !strings.Contains(skills[0].Description, "Use when the user asks to deploy the service.") {
		t.Errorf("fragment with trailing period not rewrapped cleanly: %q", skills[0].Description)
	}
}

func TestTriggerCaseInsensitiveSentinel(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationTrigger: "USE WHEN THE USER ASKS TO YELL.",
		},
	}
	skills, _ := New(root).Skills()
	if strings.Contains(skills[0].Description, "Use when the user asks to USE WHEN") {
		t.Errorf("case-insensitive sentinel not detected: %q", skills[0].Description)
	}
}

// --- #7: target-aware frontmatter ---

func TestTargetGenericOmitsExtraFields(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationAllowedTools: "Bash, Read",
		},
	}
	skills, _ := New(root).Skills() // default target = generic
	if bytes.Contains(skills[0].Bytes(), []byte("allowed-tools")) {
		t.Errorf("generic target should not emit allowed-tools:\n%s", skills[0].Bytes())
	}
}

func TestTargetClaudeCodeEmitsAllowedTools(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationAllowedTools: "Bash, Read",
		},
	}
	skills, _ := New(root, WithTarget(TargetClaudeCode)).Skills()
	body := string(skills[0].Bytes())
	if !strings.Contains(body, "allowed-tools: \"Bash, Read\"") {
		t.Errorf("claude-code target missing allowed-tools:\n%s", body)
	}
}

func TestTargetClaudeCodeNoAnnotationSkipsField(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "x"}
	skills, _ := New(root, WithTarget(TargetClaudeCode)).Skills()
	if bytes.Contains(skills[0].Bytes(), []byte("allowed-tools")) {
		t.Errorf("allowed-tools should not appear without the annotation:\n%s", skills[0].Bytes())
	}
}

func TestTargetClaudeCodePerLeafAnnotation(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "root desc here"}
	leaf := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a service",
		Annotations: map[string]string{
			AnnotationAllowedTools: "Bash",
		},
	}
	root.AddCommand(leaf)

	skills, _ := New(root, WithSplit(SplitPerLeaf), WithTarget(TargetClaudeCode)).Skills()
	var found bool
	for _, s := range skills {
		if s.Name == "mytool-deploy" && strings.Contains(string(s.Bytes()), "allowed-tools: \"Bash\"") {
			found = true
		}
	}
	if !found {
		t.Error("per-leaf allowed-tools annotation not emitted under claude-code target")
	}
}

// --- #4: output budget helpers ---

func TestEstimateTokensEmpty(t *testing.T) {
	if got := EstimateTokens(nil); got != 0 {
		t.Errorf("empty input should be 0 tokens, got %d", got)
	}
}

func TestEstimateTokensRoughly(t *testing.T) {
	// 400 bytes → ~100 tokens under the bytes/4 heuristic.
	got := EstimateTokens(bytes.Repeat([]byte("x"), 400))
	if got < 95 || got > 105 {
		t.Errorf("expected ~100 tokens for 400 bytes, got %d", got)
	}
}

// --- #6: forest lint rules ---

func TestLintOperatorSubtree(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description"}
	root.AddCommand(&cobra.Command{Use: "api-operator", Short: "internal operator"})

	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Field == "operator-subtree" && iss.Level == IssueWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("expected operator-subtree warning:\n%v", issues)
	}
}

func TestLintPathDepth(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description"}
	a := &cobra.Command{Use: "a", Short: "a-short-description"}
	b := &cobra.Command{Use: "b", Short: "b-short-description"}
	c := &cobra.Command{Use: "c", Short: "c-short-description"}
	d := &cobra.Command{Use: "d", Short: "d-short-description"}
	root.AddCommand(a)
	a.AddCommand(b)
	b.AddCommand(c)
	c.AddCommand(d) // mytool a b c d → 4 spaces → depth warning

	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Field == "depth" && iss.Command == "mytool a b c d" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected path-depth warning on the leaf:\n%v", issues)
	}
}

func TestLintSiblingVariance(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	deep := &cobra.Command{Use: "deep", Short: "d", Long: strings.Repeat("detailed narrative ", 40)}
	shallow := &cobra.Command{Use: "shallow", Short: "x"}
	root.AddCommand(deep, shallow)

	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Field == "sibling-variance" && iss.Command == "mytool" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sibling-variance warning on parent:\n%v", issues)
	}
}

func TestLintSiblingVarianceIgnoresBalancedTrees(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		root.AddCommand(&cobra.Command{
			Use:   name,
			Short: "A reasonable short description for " + name,
		})
	}
	issues := New(root).Lint()
	for _, iss := range issues {
		if iss.Field == "sibling-variance" {
			t.Errorf("did not expect sibling-variance on balanced siblings: %v", iss)
		}
	}
}
