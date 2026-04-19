package skillgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bueti/skilllint"
	"github.com/bueti/skilllint/rules"
	"github.com/spf13/cobra"
)

// --- directory layout ---

func TestSkillPathIsSkillMdInsideNameDir(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	skills, _ := New(root).Skills()
	if skills[0].Path != "mytool/SKILL.md" {
		t.Errorf("Path = %q, want %q", skills[0].Path, "mytool/SKILL.md")
	}
	if skills[0].Dir() != "mytool" {
		t.Errorf("Dir() = %q, want %q", skills[0].Dir(), "mytool")
	}
}

func TestWriteToCreatesSkillMdInSubdir(t *testing.T) {
	dir := t.TempDir()
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	if err := New(root).WriteTo(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mytool", "SKILL.md")); err != nil {
		t.Errorf("expected SKILL.md inside mytool/: %v", err)
	}
}

func TestWriteToSplitModeCreatesOneDirPerLeaf(t *testing.T) {
	dir := t.TempDir()
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	root.AddCommand(&cobra.Command{Use: "build", Short: "build a service"})
	root.AddCommand(&cobra.Command{Use: "deploy", Short: "deploy a service"})

	if err := New(root, WithSplit(SplitPerLeaf), WithOverview(true)).WriteTo(dir); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mytool/SKILL.md", "mytool-build/SKILL.md", "mytool-deploy/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

func TestFilenamePrefixAppliesToDir(t *testing.T) {
	dir := t.TempDir()
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	if err := New(root, WithFilenamePrefix("acme-")).WriteTo(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "acme-mytool", "SKILL.md")); err != nil {
		t.Errorf("prefixed directory missing: %v", err)
	}
}

// --- spec frontmatter fields ---

func TestLicenseFrontmatter(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A reasonable description here",
		Annotations: map[string]string{
			AnnotationLicense: "Apache-2.0",
		},
	}
	skills, _ := New(root).Skills()
	body := string(skills[0].Bytes())
	if !strings.Contains(body, "license: \"Apache-2.0\"") {
		t.Errorf("license frontmatter missing:\n%s", body)
	}
}

func TestCompatibilityFrontmatter(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A reasonable description here",
		Annotations: map[string]string{
			AnnotationCompatibility: "Requires git, docker, jq, internet",
		},
	}
	skills, _ := New(root).Skills()
	body := string(skills[0].Bytes())
	if !strings.Contains(body, "compatibility: \"Requires git, docker, jq, internet\"") {
		t.Errorf("compatibility frontmatter missing:\n%s", body)
	}
}

func TestMetadataFrontmatterMap(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A reasonable description here",
		Annotations: map[string]string{
			AnnotationMetadataPrefix + "author":  "example-org",
			AnnotationMetadataPrefix + "version": "1.0",
		},
	}
	skills, _ := New(root).Skills()
	body := string(skills[0].Bytes())
	if !strings.Contains(body, "metadata:\n") {
		t.Errorf("metadata map header missing:\n%s", body)
	}
	// Keys should be sorted for determinism.
	author := strings.Index(body, "author:")
	version := strings.Index(body, "version:")
	if author < 0 || version < 0 || author > version {
		t.Errorf("metadata keys not in sorted order:\n%s", body)
	}
	if !strings.Contains(body, "  author: \"example-org\"") {
		t.Errorf("author metadata missing or malformed:\n%s", body)
	}
	if !strings.Contains(body, "  version: \"1.0\"") {
		t.Errorf("version metadata missing or malformed:\n%s", body)
	}
}

func TestEmptyMetadataAnnotationsIgnored(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A reasonable description here",
		Annotations: map[string]string{
			AnnotationMetadataPrefix + "author": "  ",
		},
	}
	skills, _ := New(root).Skills()
	if strings.Contains(string(skills[0].Bytes()), "metadata:") {
		t.Errorf("empty metadata should not produce metadata header:\n%s", skills[0].Bytes())
	}
}

// --- spec-limit lint rules ---

func TestLintNameTooLong(t *testing.T) {
	longName := strings.Repeat("a", rules.MaxNameLength+1)
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A reasonable description here",
		Annotations: map[string]string{
			AnnotationName: longName,
		},
	}
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Field == "name" && iss.Severity == skilllint.SeverityError && strings.Contains(iss.Message, "spec limit") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected name-too-long error:\n%v", issues)
	}
}

func TestLintNameInvalidFormat(t *testing.T) {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A reasonable description here",
		Annotations: map[string]string{
			AnnotationName: "My--Tool", // uppercase + consecutive hyphens
		},
	}
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Field == "name" && iss.Severity == skilllint.SeverityError && strings.Contains(iss.Message, "spec regex") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected name-format error:\n%v", issues)
	}
}

func TestLintNameValidLowercase(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	issues := New(root).Lint()
	for _, iss := range issues {
		if iss.Field == "name" && iss.Severity == skilllint.SeverityError {
			t.Errorf("did not expect name error on valid name: %v", iss)
		}
	}
}

func TestLintDescriptionTooLong(t *testing.T) {
	longDesc := strings.Repeat("x", rules.MaxDescriptionLength+1)
	root := &cobra.Command{
		Use:   "mytool",
		Short: "short",
		Annotations: map[string]string{
			AnnotationDescription: longDesc,
		},
	}
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Field == "description" && iss.Severity == skilllint.SeverityError && strings.Contains(iss.Message, "spec limit") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected description-too-long error:\n%v", issues)
	}
}

func TestLintCompatibilityTooLong(t *testing.T) {
	long := strings.Repeat("x", rules.MaxCompatibilityLength+1)
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A reasonable description here",
		Annotations: map[string]string{
			AnnotationCompatibility: long,
		},
	}
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Field == "compatibility" && iss.Severity == skilllint.SeverityError {
			found = true
		}
	}
	if !found {
		t.Errorf("expected compatibility-too-long error:\n%v", issues)
	}
}

func TestLintBodyOverTokens(t *testing.T) {
	// Build a Long that balloons the body past the spec's 5000-token limit.
	longText := strings.Repeat("word ", 6000) // ~7500 tokens before rendering
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A reasonable description here",
		Long:  longText,
	}
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Rule == "body-tokens" && iss.Severity == skilllint.SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("expected body-tokens warning:\n%v", issues)
	}
}

func TestLintBodyOverLines(t *testing.T) {
	long := strings.Repeat("a line\n", rules.MaxBodyLines+50)
	root := &cobra.Command{
		Use:   "mytool",
		Short: "A reasonable description here",
		Long:  long,
	}
	issues := New(root).Lint()
	var found bool
	for _, iss := range issues {
		if iss.Rule == "body-lines" && iss.Severity == skilllint.SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("expected body-lines warning:\n%v", issues)
	}
}
