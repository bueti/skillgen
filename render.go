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
	// TriggerClause is the "Use when the user asks to …" sentence alone,
	// separate from Description (which embeds it). Empty when no trigger
	// signal is available — renderers must omit the "When to use" section
	// in that case rather than restating Description.
	TriggerClause string
	Root          CommandData
	Commands      []CommandData // flattened, depth-first, sorted by name at each level; root not included
}

// CommandData describes one cobra command for rendering.
type CommandData struct {
	Name       string   // leaf name, e.g. "deploy"
	Path       string   // full path, e.g. "mytool deploy"
	UseLine    string   // cobra's UseLine, e.g. "mytool deploy <service>"
	Short      string
	Long       string
	Example    string
	Aliases    []string // cobra.Command.Aliases
	Deprecated string   // cobra.Command.Deprecated (empty if not deprecated)
	Flags      []FlagData
	Avoid      string // skill.avoid annotation contents
	PreferOver string // skill.prefer-over annotation contents
	Extra      string // skill.examples annotation contents
	Depth      int    // 1 for direct child of root, 2 for grandchild, ...
	HasSubs    bool   // true if this command has at least one visible subcommand

	// SharedChildrenFlags is set on a parent whose visible children all have
	// identical local flag sets. The parent's section emits these once and
	// each child's section skips its local-flags block to avoid duplication.
	SharedChildrenFlags []FlagData
	// SkipFlagsInRender is set on children whose parent owns the shared-flags
	// block — render suppresses the per-child "Flags:" list in that case.
	SkipFlagsInRender bool
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
		Name:       c.Name(),
		Path:       c.CommandPath(),
		UseLine:    c.UseLine(),
		Short:      strings.TrimSpace(c.Short),
		Long:       strings.TrimSpace(c.Long),
		Example:    strings.TrimRight(c.Example, "\n"),
		Aliases:    append([]string(nil), c.Aliases...),
		Deprecated: strings.TrimSpace(c.Deprecated),
		Avoid:      strings.TrimSpace(c.Annotations[AnnotationAvoid]),
		PreferOver: strings.TrimSpace(c.Annotations[AnnotationPreferOver]),
		Extra:      strings.TrimSpace(c.Annotations[AnnotationExamples]),
		Flags:      collectFlags(c),
		HasSubs:    hasVisibleSubcommands(c),
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
		// Skip hidden and deprecated flags — deprecated is an explicit author
		// signal that the flag should not be suggested.
		if f.Hidden || f.Deprecated != "" || seen[f.Name] {
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

func (g *Generator) collectDescendants(root *cobra.Command) (descendants []CommandData, rootShared []FlagData) {
	// Walk the tree depth-first, stable-sorted. For each parent, check whether
	// its visible children all have the same local-flag fingerprint; if so,
	// hoist the flag set up to the parent so the children don't re-render it.
	//
	// The Root slot of TemplateData covers the top-level command, so the
	// flattened slice contains only descendants. rootShared returns separately
	// because the root isn't in that slice but can still anchor a sibling
	// group (the visible children of the top-level command).
	out := make([]CommandData, 0)

	var walk func(parent *cobra.Command, parentIdx int)
	walk = func(parent *cobra.Command, parentIdx int) {
		subs := g.visibleSubcommands(parent)
		if len(subs) == 0 {
			return
		}

		siblingGroup := make([]int, 0, len(subs))
		for _, sub := range subs {
			out = append(out, commandData(sub))
			idx := len(out) - 1
			siblingGroup = append(siblingGroup, idx)
			walk(sub, idx)
		}

		// After children recurse, decide whether to collapse this sibling
		// group. Collapsing requires ≥ 2 siblings with identical local-flag
		// fingerprints and at least one flag to share.
		shared := sharedSiblingFlags(out, siblingGroup)
		if shared == nil {
			return
		}
		for _, idx := range siblingGroup {
			out[idx].SkipFlagsInRender = true
		}
		if parentIdx >= 0 {
			out[parentIdx].SharedChildrenFlags = shared
		} else {
			rootShared = shared
		}
	}

	walk(root, -1)
	return out, rootShared
}

// visibleSubcommands returns c's direct subcommands that pass shouldSkip,
// sorted by name for deterministic output.
func (g *Generator) visibleSubcommands(c *cobra.Command) []*cobra.Command {
	subs := append([]*cobra.Command(nil), c.Commands()...)
	sort.SliceStable(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
	out := subs[:0]
	for _, sub := range subs {
		if !g.shouldSkip(sub) {
			out = append(out, sub)
		}
	}
	return out
}

// sharedSiblingFlags returns the shared local flag set if every index in
// siblings has the same local-flag fingerprint and there is at least one
// flag to share; otherwise nil.
func sharedSiblingFlags(all []CommandData, siblings []int) []FlagData {
	if len(siblings) < 2 {
		return nil
	}
	first := localOnly(all[siblings[0]].Flags)
	if len(first) == 0 {
		return nil
	}
	fp := flagFingerprint(first)
	for _, idx := range siblings[1:] {
		if flagFingerprint(localOnly(all[idx].Flags)) != fp {
			return nil
		}
	}
	return first
}

// flagFingerprint returns a string that uniquely identifies a flag set for
// sibling-collapse comparison. Includes name, shorthand, type, required
// status, and usage — siblings that differ in any of these shouldn't be
// collapsed because the agent-visible detail differs.
func flagFingerprint(flags []FlagData) string {
	var b strings.Builder
	for _, f := range flags {
		fmt.Fprintf(&b, "%s\x00%s\x00%s\x00%t\x00%s\x01",
			f.Name, f.Shorthand, f.Type, f.Required, f.Usage)
	}
	return b.String()
}

// defaultRender produces the built-in single-skill Markdown body.
func defaultRender(d TemplateData) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", d.Root.Path)

	writeDeprecatedCallout(&b, d.Root.Deprecated)

	if intro := firstNonEmpty(d.Root.Long, d.Root.Short); intro != "" {
		b.WriteString(intro)
		b.WriteString("\n\n")
	}

	writeAliasesLine(&b, d.Root.Aliases)

	if d.TriggerClause != "" {
		b.WriteString("## When to use\n\n")
		b.WriteString(d.TriggerClause)
		b.WriteString("\n\n")
	}

	if d.Root.UseLine != "" {
		b.WriteString("## Usage\n\n")
		fmt.Fprintf(&b, "```\n%s\n```\n\n", d.Root.UseLine)
	}

	if d.Root.Example != "" {
		b.WriteString("## Example\n\n")
		fmt.Fprintf(&b, "```\n%s\n```\n\n", d.Root.Example)
	}

	writeNamedSection(&b, "Avoid", d.Root.Avoid)
	writeNamedSection(&b, "Prefer over", d.Root.PreferOver)

	if rootFlags := nonEmptyFlags(d.Root.Flags); len(rootFlags) > 0 {
		b.WriteString("## Global flags\n\n")
		writeFlagList(&b, rootFlags)
		b.WriteString("\n")
	}

	if len(d.Root.SharedChildrenFlags) > 0 {
		b.WriteString("## Shared subcommand flags\n\n")
		b.WriteString("Every subcommand below accepts the same flags:\n\n")
		writeFlagList(&b, d.Root.SharedChildrenFlags)
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

	writeDeprecatedCallout(b, c.Deprecated)

	if body := firstNonEmpty(c.Long, c.Short); body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	}

	writeAliasesLine(b, c.Aliases)

	if c.UseLine != "" && c.UseLine != c.Path {
		fmt.Fprintf(b, "Usage: `%s`\n\n", c.UseLine)
	}

	if len(c.SharedChildrenFlags) > 0 {
		b.WriteString("Shared subcommand flags (apply to every subcommand below):\n\n")
		writeFlagList(b, c.SharedChildrenFlags)
		b.WriteString("\n")
	}

	if !c.SkipFlagsInRender {
		local := localOnly(c.Flags)
		if len(local) > 0 {
			b.WriteString("Flags:\n\n")
			writeFlagList(b, local)
			b.WriteString("\n")
		}
	}

	if c.Example != "" {
		b.WriteString("Example:\n\n")
		fmt.Fprintf(b, "```\n%s\n```\n\n", c.Example)
	}

	writeInlineLabel(b, "Avoid", c.Avoid)
	writeInlineLabel(b, "Prefer over", c.PreferOver)

	if c.Extra != "" {
		b.WriteString(c.Extra)
		b.WriteString("\n\n")
	}
}

// renderLeafBody produces the body of a split-mode leaf skill — a standalone
// Markdown document covering a single command.
func renderLeafBody(c CommandData, triggerClause string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", c.Path)

	writeDeprecatedCallout(&b, c.Deprecated)

	if intro := firstNonEmpty(c.Long, c.Short); intro != "" {
		b.WriteString(intro)
		b.WriteString("\n\n")
	}

	writeAliasesLine(&b, c.Aliases)

	if triggerClause != "" {
		b.WriteString("## When to use\n\n")
		b.WriteString(triggerClause)
		b.WriteString("\n\n")
	}

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

	writeNamedSection(&b, "Avoid", c.Avoid)
	writeNamedSection(&b, "Prefer over", c.PreferOver)

	if c.Extra != "" {
		b.WriteString(c.Extra)
		b.WriteString("\n\n")
	}

	return b.String()
}

// renderOverviewBody produces the body of a split-mode overview skill — a
// short index that points the agent at each per-leaf skill.
func renderOverviewBody(root CommandData, leaves []CommandData, triggerClause string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", root.Path)

	writeDeprecatedCallout(&b, root.Deprecated)

	if intro := firstNonEmpty(root.Long, root.Short); intro != "" {
		b.WriteString(intro)
		b.WriteString("\n\n")
	}

	writeAliasesLine(&b, root.Aliases)

	if triggerClause != "" {
		b.WriteString("## When to use\n\n")
		b.WriteString(triggerClause)
		b.WriteString("\n\n")
	}

	writeNamedSection(&b, "Avoid", root.Avoid)
	writeNamedSection(&b, "Prefer over", root.PreferOver)

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

// writeDeprecatedCallout renders a visible callout when a command is deprecated.
func writeDeprecatedCallout(b *strings.Builder, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	fmt.Fprintf(b, "> **Deprecated:** %s\n\n", msg)
}

// writeAliasesLine renders a one-line aliases note when a command has any.
func writeAliasesLine(b *strings.Builder, aliases []string) {
	if len(aliases) == 0 {
		return
	}
	refs := make([]string, len(aliases))
	for i, a := range aliases {
		refs[i] = "`" + a + "`"
	}
	fmt.Fprintf(b, "Aliases: %s\n\n", strings.Join(refs, ", "))
}

// writeNamedSection renders a "## <heading>" section with free-form content,
// skipping cleanly when content is empty.
func writeNamedSection(b *strings.Builder, heading, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	fmt.Fprintf(b, "## %s\n\n%s\n\n", heading, content)
}

// writeInlineLabel renders a bold-labeled paragraph inside a nested section,
// for use within writeCommandSection where an h2 heading would be too loud.
func writeInlineLabel(b *strings.Builder, label, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	fmt.Fprintf(b, "**%s:** %s\n\n", label, content)
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

// deriveDescription returns both the full composed description (for the
// frontmatter) and, separately, the trigger clause alone (for the body's
// "When to use" section). The clause is empty when no trigger signal is
// available — in that case the body must omit the section instead of
// restating the description.
//
// Trigger sources, in priority order:
//  1. skill.trigger annotation — "Use when the user asks to <trig>."
//  2. cobra.Command.Aliases — verbs; same phrasing as (1) with name + aliases.
//  3. Visible child names — topics; "Use when the user asks about <kids>."
//
// (3) only fires for parents (commands with visible children), never for
// leaves — a leaf's own name would just echo the description.
func (g *Generator) deriveDescription(c *cobra.Command) (full, triggerClause string) {
	base := firstNonEmpty(
		c.Annotations[AnnotationDescription],
		c.Short,
		firstLine(c.Long),
	)

	switch {
	case strings.TrimSpace(c.Annotations[AnnotationTrigger]) != "":
		triggerClause = formatTriggerClause(c.Annotations[AnnotationTrigger])
	case len(c.Aliases) > 0:
		names := append([]string{c.Name()}, c.Aliases...)
		triggerClause = "Use when the user asks to " + joinOr(names) + "."
	default:
		if kids := g.visibleChildNames(c); len(kids) > 0 {
			triggerClause = "Use when the user asks about " + joinOr(kids) + "."
		}
	}

	full = collapseSpace(base)
	if triggerClause != "" {
		if full != "" {
			full = strings.TrimRight(full, ".") + ". " + triggerClause
		} else {
			full = triggerClause
		}
	}
	return full, triggerClause
}

// visibleChildNames returns the sorted names of c's subcommands that would be
// included in skill output (not Hidden, not skill.skip, not cobra builtins,
// not excluded by WithSkip).
func (g *Generator) visibleChildNames(c *cobra.Command) []string {
	var names []string
	for _, sub := range c.Commands() {
		if g.shouldSkip(sub) {
			continue
		}
		names = append(names, sub.Name())
	}
	sort.Strings(names)
	return names
}

// formatTriggerClause normalizes a skill.trigger annotation into a single
// "Use when the user asks to …" sentence. Accepts both forms:
//
//   - Fragment: "deploy, ship, or release a service" → wrapped into a full
//     sentence with the standard prefix and a trailing period.
//   - Full sentence: "Use when the user asks to deploy the service." → used
//     as-is (only the trailing period is normalized).
//
// Detection is case-insensitive. The string "use when the user asks to" is
// treated as the sentinel prefix; anything starting with it is considered a
// complete sentence and not wrapped again.
func formatTriggerClause(trig string) string {
	trig = strings.TrimSpace(trig)
	if trig == "" {
		return ""
	}
	const sentinel = "use when the user asks"
	if strings.HasPrefix(strings.ToLower(trig), sentinel) {
		if !strings.HasSuffix(trig, ".") && !strings.HasSuffix(trig, "?") && !strings.HasSuffix(trig, "!") {
			trig += "."
		}
		return trig
	}
	// Fragment form — strip author-supplied trailing periods so we don't
	// produce "…deploy..".
	trig = strings.TrimRight(trig, ".")
	return "Use when the user asks to " + trig + "."
}

// joinOr returns a natural-language "a, b, or c" join (Oxford comma for 3+).
func joinOr(vs []string) string {
	switch len(vs) {
	case 0:
		return ""
	case 1:
		return vs[0]
	case 2:
		return vs[0] + " or " + vs[1]
	default:
		return strings.Join(vs[:len(vs)-1], ", ") + ", or " + vs[len(vs)-1]
	}
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
