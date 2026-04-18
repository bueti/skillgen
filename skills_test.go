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
	if s.Path != "mytool/SKILL.md" {
		t.Errorf("Path = %q, want %q", s.Path, "mytool/SKILL.md")
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
	if skills[0].Path != "acme-toolkit/SKILL.md" {
		t.Errorf("path not derived from name override: %q", skills[0].Path)
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

func TestSplitPerLeafBasic(t *testing.T) {
	skills, err := New(newTestRoot(), WithSplit(SplitPerLeaf)).Skills()
	if err != nil {
		t.Fatal(err)
	}
	// test root has: build (leaf), deploy (leaf), secret (hidden), internal (skipped)
	if len(skills) != 2 {
		t.Fatalf("want 2 skills, got %d", len(skills))
	}
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	if !names["mytool-build"] {
		t.Errorf("missing mytool-build skill")
	}
	if !names["mytool-deploy"] {
		t.Errorf("missing mytool-deploy skill")
	}
}

func TestSplitPerLeafLeafContent(t *testing.T) {
	skills, _ := New(newTestRoot(), WithSplit(SplitPerLeaf)).Skills()
	var deploy Skill
	for _, s := range skills {
		if s.Name == "mytool-deploy" {
			deploy = s
			break
		}
	}
	if deploy.Name == "" {
		t.Fatal("no mytool-deploy skill")
	}
	if !strings.Contains(deploy.Body, "# mytool deploy") {
		t.Errorf("leaf body missing heading:\n%s", deploy.Body)
	}
	if !strings.Contains(deploy.Body, "`--env` (required)") {
		t.Errorf("required flag not rendered:\n%s", deploy.Body)
	}
	if !strings.Contains(deploy.Description, "Use when the user asks to deploy, promote, ship, or release a service") {
		t.Errorf("trigger not appended to leaf description: %q", deploy.Description)
	}
	if deploy.Path != "mytool-deploy/SKILL.md" {
		t.Errorf("path: got %q, want mytool-deploy/SKILL.md", deploy.Path)
	}
}

func TestSplitPerLeafNested(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "mytool does things"}
	config := &cobra.Command{Use: "config", Short: "config management"}
	get := &cobra.Command{Use: "get <key>", Short: "get a config value"}
	set := &cobra.Command{Use: "set <key> <val>", Short: "set a config value"}
	config.AddCommand(get, set)
	root.AddCommand(config)

	skills, err := New(root, WithSplit(SplitPerLeaf)).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("want 2 leaf skills, got %d", len(skills))
	}
	want := []string{"mytool-config-get", "mytool-config-set"}
	for i, s := range skills {
		if s.Name != want[i] {
			t.Errorf("skill %d: got %q, want %q", i, s.Name, want[i])
		}
	}
}

func TestSplitPerLeafRootOnly(t *testing.T) {
	root := &cobra.Command{Use: "solo", Short: "a single-command CLI"}
	skills, err := New(root, WithSplit(SplitPerLeaf)).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("want 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "solo" {
		t.Errorf("name: got %q, want %q", skills[0].Name, "solo")
	}
}

func TestSplitPerLeafWithOverview(t *testing.T) {
	skills, err := New(newTestRoot(), WithSplit(SplitPerLeaf), WithOverview(true)).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 3 {
		t.Fatalf("want 3 skills (overview + 2 leaves), got %d", len(skills))
	}
	if skills[0].Name != "mytool" {
		t.Errorf("overview should be first, got %q", skills[0].Name)
	}
	if !strings.Contains(skills[0].Body, "## Commands") {
		t.Errorf("overview missing ## Commands section:\n%s", skills[0].Body)
	}
	if !strings.Contains(skills[0].Body, "- `mytool build`") {
		t.Errorf("overview missing build entry:\n%s", skills[0].Body)
	}
	if !strings.Contains(skills[0].Body, "- `mytool deploy`") {
		t.Errorf("overview missing deploy entry:\n%s", skills[0].Body)
	}
}

func TestSplitPerLeafOverviewSuppressedForSingleLeaf(t *testing.T) {
	root := &cobra.Command{Use: "solo", Short: "a single-command CLI"}
	skills, err := New(root, WithSplit(SplitPerLeaf), WithOverview(true)).Skills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("overview must be suppressed when only one leaf exists; got %d skills", len(skills))
	}
}

func TestSplitPerLeafRespectsHiddenAndSkip(t *testing.T) {
	skills, _ := New(newTestRoot(), WithSplit(SplitPerLeaf)).Skills()
	for _, s := range skills {
		if strings.Contains(s.Name, "secret") {
			t.Errorf("hidden command leaked: %q", s.Name)
		}
		if strings.Contains(s.Name, "internal") {
			t.Errorf("skill.skip command leaked: %q", s.Name)
		}
	}
}

func TestSplitPerLeafDeterministic(t *testing.T) {
	a, _ := New(newTestRoot(), WithSplit(SplitPerLeaf), WithOverview(true)).Skills()
	b, _ := New(newTestRoot(), WithSplit(SplitPerLeaf), WithOverview(true)).Skills()
	if len(a) != len(b) {
		t.Fatalf("length mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if !bytes.Equal(a[i].Bytes(), b[i].Bytes()) {
			t.Errorf("skill %d (%q) not deterministic", i, a[i].Name)
		}
	}
}

func TestSplitPerLeafWriteTo(t *testing.T) {
	dir := t.TempDir()
	if err := New(newTestRoot(), WithSplit(SplitPerLeaf), WithOverview(true)).WriteTo(dir); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mytool/SKILL.md", "mytool-build/SKILL.md", "mytool-deploy/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

func TestWriteToCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := New(newTestRoot()).WriteTo(filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "out", "mytool", "SKILL.md"))
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
	if skills[0].Path != "acme-mytool/SKILL.md" {
		t.Errorf("prefix not applied: %q", skills[0].Path)
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
