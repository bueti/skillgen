package skillgen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/spf13/cobra"
)

func newTestRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A CLI for building and deploying services",
	}
	root.PersistentFlags().Bool("verbose", false, "enable verbose logging")

	build := &cobra.Command{
		Use:     "build <service>",
		Short:   "Build an artifact of a service",
		Example: "mytool build api --tag v1.2.3",
	}
	build.Flags().String("tag", "latest", "image tag to apply")
	build.Flags().Bool("push", false, "push the built image")

	deploy := &cobra.Command{
		Use:   "deploy <service>",
		Short: "Deploy a built artifact to an environment",
		Long:  "Deploy promotes an already-built artifact of a service into a named environment.",
		Annotations: map[string]string{
			AnnotationTrigger: "deploy, promote, ship, or release a service",
		},
	}
	deploy.Flags().String("env", "", "target environment")
	_ = deploy.MarkFlagRequired("env")
	deploy.Flags().Bool("dry-run", false, "print the plan without applying")

	hidden := &cobra.Command{Use: "secret", Short: "hidden", Hidden: true}
	skipped := &cobra.Command{
		Use:         "internal",
		Short:       "internal tooling",
		Annotations: map[string]string{AnnotationSkip: "true"},
	}

	root.AddCommand(build, deploy, hidden, skipped)
	return root
}

func TestSingleSkill(t *testing.T) {
	skills, err := New(newTestRoot()).Skills()
	if err != nil {
		t.Fatalf("Skills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("want 1 skill, got %d", len(skills))
	}
	s := skills[0]
	if s.Name != "mytool" {
		t.Errorf("Name = %q, want %q", s.Name, "mytool")
	}
	if s.Filename != "mytool.md" {
		t.Errorf("Filename = %q, want %q", s.Filename, "mytool.md")
	}
	if !strings.Contains(s.Body, "### `mytool build`") {
		t.Errorf("body missing build section:\n%s", s.Body)
	}
	if !strings.Contains(s.Body, "### `mytool deploy`") {
		t.Errorf("body missing deploy section:\n%s", s.Body)
	}
}

func TestHiddenAndSkipExcluded(t *testing.T) {
	skills, err := New(newTestRoot()).Skills()
	if err != nil {
		t.Fatal(err)
	}
	body := skills[0].Body
	if strings.Contains(body, "secret") {
		t.Errorf("hidden command leaked into body:\n%s", body)
	}
	if strings.Contains(body, "internal") {
		t.Errorf("skill.skip command leaked into body:\n%s", body)
	}
}

func TestTriggerAnnotation(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "Build and deploy services",
		Annotations: map[string]string{
			AnnotationTrigger: "build or deploy services",
		},
	}
	skills, err := New(root).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skills[0].Description, "Use when the user asks to build or deploy services") {
		t.Errorf("trigger not appended: %q", skills[0].Description)
	}
}

func TestDescriptionOverride(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "ignore me",
		Annotations: map[string]string{
			AnnotationDescription: "Custom description.",
		},
	}
	skills, _ := New(root).Skills()
	if skills[0].Description != "Custom description." {
		t.Errorf("description override not applied: %q", skills[0].Description)
	}
}

func TestNameOverride(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "x",
		Annotations: map[string]string{
			AnnotationName: "acme-toolkit",
		},
	}
	skills, _ := New(root).Skills()
	if skills[0].Name != "acme-toolkit" {
		t.Errorf("name override not applied: %q", skills[0].Name)
	}
	if skills[0].Filename != "acme-toolkit.md" {
		t.Errorf("filename not derived from name override: %q", skills[0].Filename)
	}
}

func TestRequiredFlagMarked(t *testing.T) {
	skills, _ := New(newTestRoot()).Skills()
	if !strings.Contains(skills[0].Body, "`--env` (required)") {
		t.Errorf("required flag not marked:\n%s", skills[0].Body)
	}
}

func TestFlagDefaultShown(t *testing.T) {
	skills, _ := New(newTestRoot()).Skills()
	if !strings.Contains(skills[0].Body, "(default `latest`)") {
		t.Errorf("default value not rendered:\n%s", skills[0].Body)
	}
}

func TestPersistentFlagInGlobals(t *testing.T) {
	skills, _ := New(newTestRoot()).Skills()
	if !strings.Contains(skills[0].Body, "## Global flags") {
		t.Errorf("global flags section missing:\n%s", skills[0].Body)
	}
	if !strings.Contains(skills[0].Body, "--verbose") {
		t.Errorf("persistent --verbose missing:\n%s", skills[0].Body)
	}
}

func TestDeterministic(t *testing.T) {
	a, err := New(newTestRoot()).Skills()
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(newTestRoot()).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a[0].Bytes(), b[0].Bytes()) {
		t.Errorf("output not deterministic between runs")
	}
}

func TestRootWithoutDescriptionErrors(t *testing.T) {
	root := &cobra.Command{Use: "empty"}
	_, err := New(root).Skills()
	if err == nil {
		t.Fatal("expected error for root without description")
	}
}

func TestNilRootErrors(t *testing.T) {
	_, err := New(nil).Skills()
	if err == nil {
		t.Fatal("expected error for nil root")
	}
}

func TestSkipPredicate(t *testing.T) {
	root := newTestRoot()
	skills, _ := New(root, WithSkip(func(c *cobra.Command) bool {
		return c.Name() == "deploy"
	})).Skills()
	if strings.Contains(skills[0].Body, "mytool deploy") {
		t.Errorf("predicate-excluded command leaked:\n%s", skills[0].Body)
	}
	if !strings.Contains(skills[0].Body, "mytool build") {
		t.Errorf("non-excluded command missing:\n%s", skills[0].Body)
	}
}

func TestSplitPerLeafNotImplemented(t *testing.T) {
	_, err := New(newTestRoot(), WithSplit(SplitPerLeaf)).Skills()
	if err == nil {
		t.Fatal("expected not-implemented error")
	}
}

func TestWriteToCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := New(newTestRoot()).WriteTo(filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "out", "mytool.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("---\nname: \"mytool\"\n")) {
		t.Errorf("unexpected file prefix:\n%s", data)
	}
}

func TestBytesFrontmatter(t *testing.T) {
	s := Skill{
		Name:        "my-tool",
		Description: `A "tricky" description: has quotes and colons.`,
		Body:        "# body\n",
	}
	out := string(s.Bytes())
	wantPrefix := "---\nname: \"my-tool\"\ndescription: \"A \\\"tricky\\\" description: has quotes and colons.\"\n---\n\n"
	if !strings.HasPrefix(out, wantPrefix) {
		t.Errorf("frontmatter malformed:\n%s", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("missing trailing newline")
	}
}

func TestCustomTemplate(t *testing.T) {
	tmpl := template.Must(template.New("x").Parse("ROOT={{.Root.Path}}\nCOUNT={{len .Commands}}\n"))
	skills, err := New(newTestRoot(), WithTemplate(tmpl)).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skills[0].Body, "ROOT=mytool") {
		t.Errorf("template not applied:\n%s", skills[0].Body)
	}
	if !strings.Contains(skills[0].Body, "COUNT=2") {
		t.Errorf("expected 2 visible descendants:\n%s", skills[0].Body)
	}
}

func TestFilenamePrefix(t *testing.T) {
	skills, _ := New(newTestRoot(), WithFilenamePrefix("acme-")).Skills()
	if skills[0].Filename != "acme-mytool.md" {
		t.Errorf("prefix not applied: %q", skills[0].Filename)
	}
}

func TestCobraBuiltinsExcludedByDefault(t *testing.T) {
	root := newTestRoot()
	// Attach the skills subcommand (which cobra forces "help" + "completion" to coexist with).
	root.AddCommand(NewSkillsCmd(root))
	// Force cobra to realize its auto-injected commands.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	skills, err := New(root).Skills()
	if err != nil {
		t.Fatal(err)
	}
	body := skills[0].Body
	if strings.Contains(body, "`mytool help`") {
		t.Errorf("help command leaked into body:\n%s", body)
	}
	if strings.Contains(body, "`mytool completion`") {
		t.Errorf("completion command leaked into body:\n%s", body)
	}
	if strings.Contains(body, "`mytool skills`") {
		t.Errorf("skills command leaked into body:\n%s", body)
	}
}

func TestIncludeBuiltinsOptIn(t *testing.T) {
	root := newTestRoot()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	skills, err := New(root, WithIncludeBuiltins()).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skills[0].Body, "`mytool help`") {
		t.Errorf("help command should be included with opt-in:\n%s", skills[0].Body)
	}
}

func TestUserSubcommandNamedHelpNotSkipped(t *testing.T) {
	// A non-root command that happens to be named "help" should NOT be filtered.
	root := &cobra.Command{Use: "mytool", Short: "x"}
	docs := &cobra.Command{Use: "docs", Short: "documentation"}
	help := &cobra.Command{Use: "help", Short: "show docs help"}
	docs.AddCommand(help)
	root.AddCommand(docs)

	skills, err := New(root).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skills[0].Body, "mytool docs help") {
		t.Errorf("user-defined nested 'help' was wrongly skipped:\n%s", skills[0].Body)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"mytool":       "mytool",
		"My Tool":      "my-tool",
		"foo_bar-baz":  "foo-bar-baz",
		"  spaced   ":  "spaced",
		"123go":        "123go",
		"weird!!name":  "weird-name",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
