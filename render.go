package skillgen

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TemplateData is the value passed to a custom template via WithTemplate.
type TemplateData struct {
	Name        string
	Description string
	Root        CommandData
	Commands    []CommandData // flattened, depth-first, sorted by name at each level; root not included
}

// CommandData describes one cobra command for rendering.
type CommandData struct {
	Name     string // leaf name, e.g. "deploy"
	Path     string // full path, e.g. "mytool deploy"
	UseLine  string // cobra's UseLine, e.g. "mytool deploy <service>"
	Short    string
	Long     string
	Example  string
	Flags    []FlagData
	Extra    string // skill.examples annotation contents
	Depth    int    // 1 for direct child of root, 2 for grandchild, ...
	HasSubs  bool   // true if this command has at least one visible subcommand
}

// FlagData describes one pflag.Flag.
type FlagData struct {
	Name       string
	Shorthand  string
	Type       string
	DefValue   string
	Usage      string
	Required   bool
	Persistent bool
}

// Ref returns a rendered flag reference, e.g. "--env" or "-n, --name".
func (f FlagData) Ref() string {
	if f.Shorthand != "" {
		return fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name)
	}
	return "--" + f.Name
}

func commandData(c *cobra.Command) CommandData {
	d := CommandData{
		Name:    c.Name(),
		Path:    c.CommandPath(),
		UseLine: c.UseLine(),
		Short:   strings.TrimSpace(c.Short),
		Long:    strings.TrimSpace(c.Long),
		Example: strings.TrimRight(c.Example, "\n"),
		Extra:   strings.TrimSpace(c.Annotations[AnnotationExamples]),
		Flags:   collectFlags(c),
		HasSubs: hasVisibleSubcommands(c),
	}
	// Depth relative to root = number of spaces in the command path.
	d.Depth = strings.Count(d.Path, " ")
	return d
}

func hasVisibleSubcommands(c *cobra.Command) bool {
	for _, sub := range c.Commands() {
		if !sub.Hidden {
			return true
		}
	}
	return false
}

func collectFlags(c *cobra.Command) []FlagData {
	seen := map[string]bool{}
	var out []FlagData
	add := func(f *pflag.Flag, persistent bool) {
		if f.Hidden || seen[f.Name] {
			return
		}
		seen[f.Name] = true
		_, required := f.Annotations[cobra.BashCompOneRequiredFlag]
		out = append(out, FlagData{
			Name:       f.Name,
			Shorthand:  f.Shorthand,
			Type:       f.Value.Type(),
			DefValue:   f.DefValue,
			Usage:      strings.TrimSpace(f.Usage),
			Required:   required,
			Persistent: persistent,
		})
	}
	c.LocalFlags().VisitAll(func(f *pflag.Flag) { add(f, false) })
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) { add(f, true) })
	sort.SliceStable(out, func(i, j int) bool {
		// Required flags first, then alphabetical.
		if out[i].Required != out[j].Required {
			return out[i].Required
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (g *Generator) collectDescendants(root *cobra.Command) []CommandData {
	var out []CommandData
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		subs := append([]*cobra.Command(nil), c.Commands()...)
		sort.SliceStable(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
		for _, sub := range subs {
			if g.shouldSkip(sub) {
				continue
			}
			out = append(out, commandData(sub))
			walk(sub)
		}
	}
	walk(root)
	return out
}

// defaultRender produces the built-in single-skill Markdown body.
func defaultRender(d TemplateData) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", d.Root.Path)

	if intro := firstNonEmpty(d.Root.Long, d.Root.Short); intro != "" {
		b.WriteString(intro)
		b.WriteString("\n\n")
	}

	b.WriteString("## When to use\n\n")
	b.WriteString(d.Description)
	b.WriteString("\n\n")

	if d.Root.UseLine != "" {
		b.WriteString("## Usage\n\n")
		fmt.Fprintf(&b, "```\n%s\n```\n\n", d.Root.UseLine)
	}

	if d.Root.Example != "" {
		b.WriteString("## Example\n\n")
		fmt.Fprintf(&b, "```\n%s\n```\n\n", d.Root.Example)
	}

	if rootFlags := nonEmptyFlags(d.Root.Flags); len(rootFlags) > 0 {
		b.WriteString("## Global flags\n\n")
		writeFlagList(&b, rootFlags)
		b.WriteString("\n")
	}

	if len(d.Commands) > 0 {
		b.WriteString("## Commands\n\n")
		for _, c := range d.Commands {
			writeCommandSection(&b, c)
		}
	}

	return b.String()
}

func writeCommandSection(b *strings.Builder, c CommandData) {
	// Heading level grows with depth but caps at h4 to stay readable.
	level := min(2+c.Depth, 4)
	fmt.Fprintf(b, "%s `%s`\n\n", strings.Repeat("#", level), c.Path)

	if body := firstNonEmpty(c.Long, c.Short); body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	}

	if c.UseLine != "" && c.UseLine != c.Path {
		fmt.Fprintf(b, "Usage: `%s`\n\n", c.UseLine)
	}

	local := localOnly(c.Flags)
	if len(local) > 0 {
		b.WriteString("Flags:\n\n")
		writeFlagList(b, local)
		b.WriteString("\n")
	}

	if c.Example != "" {
		b.WriteString("Example:\n\n")
		fmt.Fprintf(b, "```\n%s\n```\n\n", c.Example)
	}

	if c.Extra != "" {
		b.WriteString(c.Extra)
		b.WriteString("\n\n")
	}
}

// renderLeafBody produces the body of a split-mode leaf skill — a standalone
// Markdown document covering a single command.
func renderLeafBody(c CommandData, desc string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", c.Path)

	if intro := firstNonEmpty(c.Long, c.Short); intro != "" {
		b.WriteString(intro)
		b.WriteString("\n\n")
	}

	b.WriteString("## When to use\n\n")
	b.WriteString(desc)
	b.WriteString("\n\n")

	if c.UseLine != "" {
		b.WriteString("## Usage\n\n")
		fmt.Fprintf(&b, "```\n%s\n```\n\n", c.UseLine)
	}

	if len(c.Flags) > 0 {
		b.WriteString("## Flags\n\n")
		writeFlagList(&b, c.Flags)
		b.WriteString("\n")
	}

	if c.Example != "" {
		b.WriteString("## Example\n\n")
		fmt.Fprintf(&b, "```\n%s\n```\n\n", c.Example)
	}

	if c.Extra != "" {
		b.WriteString(c.Extra)
		b.WriteString("\n\n")
	}

	return b.String()
}

// renderOverviewBody produces the body of a split-mode overview skill — a
// short index that points the agent at each per-leaf skill.
func renderOverviewBody(root CommandData, leaves []CommandData, desc string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", root.Path)

	if intro := firstNonEmpty(root.Long, root.Short); intro != "" {
		b.WriteString(intro)
		b.WriteString("\n\n")
	}

	b.WriteString("## When to use\n\n")
	b.WriteString(desc)
	b.WriteString("\n\n")

	if rootFlags := nonEmptyFlags(root.Flags); len(rootFlags) > 0 {
		b.WriteString("## Global flags\n\n")
		writeFlagList(&b, rootFlags)
		b.WriteString("\n")
	}

	b.WriteString("## Commands\n\n")
	b.WriteString("Each subcommand has its own skill:\n\n")
	for _, leaf := range leaves {
		summary := firstNonEmpty(leaf.Short, firstLine(leaf.Long))
		if summary != "" {
			fmt.Fprintf(&b, "- `%s` — %s\n", leaf.Path, summary)
		} else {
			fmt.Fprintf(&b, "- `%s`\n", leaf.Path)
		}
	}
	b.WriteString("\n")

	return b.String()
}

func writeFlagList(b *strings.Builder, flags []FlagData) {
	for _, f := range flags {
		fmt.Fprintf(b, "- `%s`", f.Ref())
		if f.Required {
			b.WriteString(" (required)")
		}
		if f.Usage != "" {
			fmt.Fprintf(b, " — %s", f.Usage)
		}
		if !f.Required && f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
			fmt.Fprintf(b, " (default `%s`)", f.DefValue)
		}
		b.WriteString("\n")
	}
}

func localOnly(flags []FlagData) []FlagData {
	var out []FlagData
	for _, f := range flags {
		if !f.Persistent {
			out = append(out, f)
		}
	}
	return out
}

func nonEmptyFlags(flags []FlagData) []FlagData {
	var out []FlagData
	for _, f := range flags {
		// Drop cobra's injected help flag from global listings; it's noise.
		if f.Name == "help" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// --- small string helpers ---

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return strings.TrimSpace(before)
	}
	return s
}

func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
			}
			prevSpace = true
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// yamlString renders s as a YAML double-quoted scalar (always quoted for safety).
// Newlines are collapsed to spaces so the frontmatter stays single-line.
func yamlString(s string) string {
	s = strings.ReplaceAll(s, "\\", `\\`)
	s = strings.ReplaceAll(s, "\"", `\"`)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return "\"" + s + "\""
}
