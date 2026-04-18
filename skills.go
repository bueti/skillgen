// Package skillgen generates agent skill files from a cobra command tree.
//
// The default output is a single Markdown file with YAML frontmatter that
// describes the whole CLI — name, description, and a section per subcommand.
// A large CLI can opt into split mode (one skill per leaf command) but that is
// not yet implemented.
//
// Typical integration mirrors cobra's own completion command:
//
//	root.AddCommand(skillgen.NewSkillsCmd(root))
//
// See PRD.md in the repository root for the full design.
package skillgen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

// Annotation keys an author can set on a cobra.Command to shape its skill output.
const (
	// AnnotationTrigger adds trigger phrases to the skill's description.
	//
	// Two forms are accepted:
	//
	//   - Fragment (preferred): "deploy, promote, ship a service". The
	//     library wraps it into "Use when the user asks to <fragment>."
	//   - Full sentence: "Use when the user asks to deploy the service."
	//     (case-insensitive prefix detection). Used as-is.
	//
	// Trailing punctuation is normalized so both forms produce a single
	// terminating period.
	AnnotationTrigger = "skill.trigger"

	// AnnotationDescription replaces the generated description wholesale.
	AnnotationDescription = "skill.description"

	// AnnotationName overrides the skill's name. In single-skill mode, only
	// the root command's annotation is consulted.
	AnnotationName = "skill.name"

	// AnnotationSkip, when set to "true", excludes the command and its subtree.
	AnnotationSkip = "skill.skip"

	// AnnotationExamples appends free-form example text to a command's body section.
	AnnotationExamples = "skill.examples"

	// AnnotationAvoid is free-form Markdown that appears under an "Avoid"
	// heading. Use it to tell the agent what not to do — the single highest-
	// value content in most real skills.
	AnnotationAvoid = "skill.avoid"

	// AnnotationPreferOver is free-form Markdown that appears under a "Prefer
	// over" heading. Use it to point the agent away from alternative tools or
	// commands this one supersedes (e.g. "Use instead of raw `kubectl delete`").
	AnnotationPreferOver = "skill.prefer-over"

	// AnnotationAllowedTools populates the Claude Code `allowed-tools`
	// frontmatter field (comma- or space-separated list of tool names, e.g.
	// "Bash, Read, Edit"). Only emitted under TargetClaudeCode.
	AnnotationAllowedTools = "skill.allowed-tools"
)

// SplitMode selects how many skill files are emitted.
type SplitMode int

const (
	// SplitNone writes a single skill describing the whole CLI (the default).
	SplitNone SplitMode = iota
	// SplitPerLeaf writes one skill per leaf command. Enable WithOverview to
	// additionally emit a single overview skill that lists all leaves.
	SplitPerLeaf
)

// Target controls which host's conventions the generated frontmatter follows.
// Hosts vary in which optional fields they support; TargetGeneric stays strict
// (name + description only) while richer targets opt in to host-specific keys.
type Target int

const (
	// TargetGeneric emits the minimal, interoperable frontmatter: name + description.
	TargetGeneric Target = iota
	// TargetClaudeCode emits Claude Code-specific keys like `allowed-tools`
	// (from the skill.allowed-tools annotation, comma-separated).
	TargetClaudeCode
)

// FrontmatterField is a target-specific frontmatter key/value emitted
// alongside name + description. Value should be YAML-safe.
type FrontmatterField struct {
	Key   string
	Value string
}

// Skill is a single generated skill file.
type Skill struct {
	Name        string
	Description string
	Body        string
	Filename    string // basename, e.g. "mytool.md"
	// Frontmatter is extra target-specific frontmatter fields emitted after
	// name + description. Empty for TargetGeneric.
	Frontmatter []FrontmatterField
}

// Bytes returns the full file contents (frontmatter + body, trailing newline).
func (s Skill) Bytes() []byte {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	fmt.Fprintf(&buf, "name: %s\n", yamlString(s.Name))
	fmt.Fprintf(&buf, "description: %s\n", yamlString(s.Description))
	for _, f := range s.Frontmatter {
		fmt.Fprintf(&buf, "%s: %s\n", f.Key, yamlString(f.Value))
	}
	buf.WriteString("---\n\n")
	body := strings.TrimLeft(s.Body, "\n")
	buf.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// Option configures a Generator.
type Option func(*Generator)

// WithSplit sets the output mode. Default is SplitNone.
func WithSplit(m SplitMode) Option { return func(g *Generator) { g.split = m } }

// WithOverview controls whether an overview skill is emitted in split mode.
// Has no effect in SplitNone.
func WithOverview(b bool) Option { return func(g *Generator) { g.overview = b } }

// WithTemplate replaces the default body renderer with a text/template.
// The template receives a TemplateData value.
func WithTemplate(t *template.Template) Option { return func(g *Generator) { g.tmpl = t } }

// WithFilenamePrefix prepends a prefix to every generated filename.
func WithFilenamePrefix(p string) Option { return func(g *Generator) { g.prefix = p } }

// WithSkip installs a predicate that excludes commands (and their subtrees).
// Runs in addition to Hidden and the skill.skip annotation.
func WithSkip(pred func(*cobra.Command) bool) Option {
	return func(g *Generator) { g.skipFn = pred }
}

// WithIncludeBuiltins keeps cobra's auto-injected `help` and `completion`
// commands in the output. They are excluded by default because agents don't
// need them to use the tool.
func WithIncludeBuiltins() Option {
	return func(g *Generator) { g.includeBuiltins = true }
}

// WithTarget selects which host's frontmatter conventions to follow. Default
// is TargetGeneric (strict name + description).
func WithTarget(t Target) Option { return func(g *Generator) { g.target = t } }

// Generator emits skills for a cobra command tree.
type Generator struct {
	root            *cobra.Command
	split           SplitMode
	overview        bool
	tmpl            *template.Template
	prefix          string
	skipFn          func(*cobra.Command) bool
	includeBuiltins bool
	target          Target
}

// New returns a Generator for the given root command.
func New(root *cobra.Command, opts ...Option) *Generator {
	g := &Generator{root: root}
	for _, o := range opts {
		o(g)
	}
	return g
}

// Skills returns the generated skills without touching the filesystem.
func (g *Generator) Skills() ([]Skill, error) {
	if g.root == nil {
		return nil, fmt.Errorf("skillgen: root command is nil")
	}
	switch g.split {
	case SplitNone:
		s, err := g.singleSkill()
		if err != nil {
			return nil, err
		}
		return []Skill{s}, nil
	case SplitPerLeaf:
		return g.splitSkills()
	default:
		return nil, fmt.Errorf("skillgen: unknown split mode %d", g.split)
	}
}

// WriteTo renders skills and writes them into dir, creating it if needed.
func (g *Generator) WriteTo(dir string) error {
	skills, err := g.Skills()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("skillgen: create %s: %w", dir, err)
	}
	for _, s := range skills {
		path := filepath.Join(dir, s.Filename)
		if err := os.WriteFile(path, s.Bytes(), 0o644); err != nil {
			return fmt.Errorf("skillgen: write %s: %w", path, err)
		}
	}
	return nil
}

// targetFrontmatter returns the target-specific frontmatter fields for a
// command under the generator's current target. Returns nil when the target
// is TargetGeneric or when no target-specific annotations apply.
func (g *Generator) targetFrontmatter(c *cobra.Command) []FrontmatterField {
	switch g.target {
	case TargetClaudeCode:
		var out []FrontmatterField
		if tools := strings.TrimSpace(c.Annotations[AnnotationAllowedTools]); tools != "" {
			out = append(out, FrontmatterField{Key: "allowed-tools", Value: tools})
		}
		return out
	default:
		return nil
	}
}

func (g *Generator) shouldSkip(c *cobra.Command) bool {
	if c.Hidden {
		return true
	}
	if v, ok := c.Annotations[AnnotationSkip]; ok && strings.EqualFold(strings.TrimSpace(v), "true") {
		return true
	}
	if !g.includeBuiltins && isCobraBuiltin(c) {
		return true
	}
	if g.skipFn != nil && g.skipFn(c) {
		return true
	}
	return false
}

// isCobraBuiltin reports whether c is one of cobra's auto-injected commands
// (`help`, `completion`). These only auto-register as direct children of the
// root, so we gate on depth to avoid mis-skipping an author's own subcommand
// that happens to be named "help" or "completion".
func isCobraBuiltin(c *cobra.Command) bool {
	if c.Parent() == nil || c.Parent().Parent() != nil {
		return false
	}
	switch c.Name() {
	case "help", "completion":
		return true
	}
	return false
}

func (g *Generator) singleSkill() (Skill, error) {
	data := TemplateData{
		Root:     commandData(g.root),
		Commands: g.collectDescendants(g.root),
	}

	name := firstNonEmpty(g.root.Annotations[AnnotationName], slug(g.root.Name()))
	if name == "" {
		return Skill{}, fmt.Errorf("skillgen: root command has no usable name")
	}

	desc, trig := g.deriveDescription(g.root)
	if desc == "" {
		return Skill{}, fmt.Errorf("skillgen: root command %q has no description — set Short, Long, or the %q annotation", g.root.Name(), AnnotationDescription)
	}

	data.Name = name
	data.Description = desc
	data.TriggerClause = trig

	body, err := g.renderBody(data)
	if err != nil {
		return Skill{}, err
	}

	return Skill{
		Name:        name,
		Description: desc,
		Body:        body,
		Filename:    g.prefix + name + ".md",
		Frontmatter: g.targetFrontmatter(g.root),
	}, nil
}

func (g *Generator) renderBody(d TemplateData) (string, error) {
	if g.tmpl == nil {
		return defaultRender(d), nil
	}
	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, d); err != nil {
		return "", fmt.Errorf("skillgen: render template: %w", err)
	}
	return buf.String(), nil
}

// splitSkills renders one skill per leaf command, optionally prefixed by an
// overview skill that lists the leaves.
func (g *Generator) splitSkills() ([]Skill, error) {
	leaves := g.collectLeafCommands(g.root)
	if len(leaves) == 0 {
		return nil, fmt.Errorf("skillgen: no visible commands to emit skills for")
	}

	var out []Skill

	// Overview only makes sense when there's more than one leaf — otherwise it
	// would be a second copy of the same skill (or worse, collide on filename).
	if g.overview && len(leaves) > 1 {
		ov, err := g.overviewSkill(leaves)
		if err != nil {
			return nil, err
		}
		out = append(out, ov)
	}

	for _, c := range leaves {
		s, err := g.leafSkill(c)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}

	return out, nil
}

// collectLeafCommands returns the tree's leaves — commands with no visible
// children. Hidden / skill.skip / predicate-excluded commands are pruned
// before leafness is decided, so a parent whose only children are all skipped
// is itself treated as a leaf.
func (g *Generator) collectLeafCommands(root *cobra.Command) []*cobra.Command {
	if root == nil || g.shouldSkip(root) {
		return nil
	}

	var leaves []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		var visible []*cobra.Command
		for _, sub := range c.Commands() {
			if !g.shouldSkip(sub) {
				visible = append(visible, sub)
			}
		}
		if len(visible) == 0 {
			leaves = append(leaves, c)
			return
		}
		for _, sub := range visible {
			walk(sub)
		}
	}
	walk(root)

	sort.SliceStable(leaves, func(i, j int) bool {
		return leaves[i].CommandPath() < leaves[j].CommandPath()
	})
	return leaves
}

func (g *Generator) leafSkill(c *cobra.Command) (Skill, error) {
	name := firstNonEmpty(c.Annotations[AnnotationName], slug(c.CommandPath()))
	if name == "" {
		return Skill{}, fmt.Errorf("skillgen: leaf command at %q has no usable name", c.CommandPath())
	}

	desc, trig := g.deriveDescription(c)
	if desc == "" {
		return Skill{}, fmt.Errorf("skillgen: leaf command %q has no description — set Short, Long, or the %q annotation", c.CommandPath(), AnnotationDescription)
	}

	body := renderLeafBody(commandData(c), trig)
	return Skill{
		Name:        name,
		Description: desc,
		Body:        body,
		Filename:    g.prefix + name + ".md",
		Frontmatter: g.targetFrontmatter(c),
	}, nil
}

func (g *Generator) overviewSkill(leaves []*cobra.Command) (Skill, error) {
	name := firstNonEmpty(g.root.Annotations[AnnotationName], slug(g.root.Name()))
	if name == "" {
		return Skill{}, fmt.Errorf("skillgen: root command has no usable name for the overview skill")
	}

	desc, trig := g.deriveDescription(g.root)
	if desc == "" {
		return Skill{}, fmt.Errorf("skillgen: root command %q has no description for the overview — set Short, Long, or the %q annotation", g.root.Name(), AnnotationDescription)
	}

	leafData := make([]CommandData, len(leaves))
	for i, c := range leaves {
		leafData[i] = commandData(c)
	}
	body := renderOverviewBody(commandData(g.root), leafData, trig)

	return Skill{
		Name:        name,
		Description: desc,
		Body:        body,
		Filename:    g.prefix + name + ".md",
		Frontmatter: g.targetFrontmatter(g.root),
	}, nil
}
