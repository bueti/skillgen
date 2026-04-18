package skillgen

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// makeSiblingLeaf builds a leaf command with a name, Short, and a shared
// flag signature (-p instances, -r reason) so tests can mix and match.
func makeSiblingLeaf(name, short string, withInstances, withReason bool) *cobra.Command {
	c := &cobra.Command{Use: name, Short: short}
	if withInstances {
		c.Flags().StringP("instances", "p", "", "target instances")
		_ = c.MarkFlagRequired("instances")
	}
	if withReason {
		c.Flags().StringP("reason", "r", "", "justification for the action")
		_ = c.MarkFlagRequired("reason")
	}
	return c
}

func TestSiblingsCollapseWhenFlagsIdentical(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	actions := &cobra.Command{Use: "actions", Short: "node actions parent"}
	for _, name := range []string{"cycle", "triage", "terminate", "reboot", "ignore"} {
		actions.AddCommand(makeSiblingLeaf(name, "do a thing to the node", true, true))
	}
	root.AddCommand(actions)

	skills, err := New(root).Skills()
	if err != nil {
		t.Fatal(err)
	}
	body := skills[0].Body

	// Parent section should carry the shared-flags block.
	if !strings.Contains(body, "Shared subcommand flags") {
		t.Errorf("expected shared-flags block on parent:\n%s", body)
	}
	// Siblings should NOT emit their own Flags: list.
	flagsOccurrences := strings.Count(body, "\nFlags:\n")
	if flagsOccurrences != 0 {
		t.Errorf("expected 0 per-child Flags blocks, got %d:\n%s", flagsOccurrences, body)
	}
	// Each sibling still renders as its own section with its own description.
	for _, name := range []string{"cycle", "triage", "terminate", "reboot", "ignore"} {
		if !strings.Contains(body, "`mytool actions "+name+"`") {
			t.Errorf("missing sibling section for %s:\n%s", name, body)
		}
	}
	// The flag details only appear once in the body.
	if n := strings.Count(body, "target instances"); n != 1 {
		t.Errorf("expected 1 rendering of --instances usage, got %d", n)
	}
}

func TestSiblingsDoNotCollapseWithPartialOverlap(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	actions := &cobra.Command{Use: "actions", Short: "x"}
	// Two siblings have both flags, one has only --instances: not uniform.
	actions.AddCommand(makeSiblingLeaf("cycle", "cycle a node", true, true))
	actions.AddCommand(makeSiblingLeaf("triage", "triage a node", true, true))
	actions.AddCommand(makeSiblingLeaf("ignore", "ignore a node", true, false))
	root.AddCommand(actions)

	skills, _ := New(root).Skills()
	body := skills[0].Body

	if strings.Contains(body, "Shared subcommand flags") {
		t.Errorf("did not expect shared-flags block when siblings disagree:\n%s", body)
	}
	// Each sibling should emit its own per-command Flags: block.
	if n := strings.Count(body, "\nFlags:\n"); n != 3 {
		t.Errorf("expected 3 per-child Flags blocks, got %d", n)
	}
}

func TestSiblingsDoNotCollapseWithFlagUsageDifference(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	actions := &cobra.Command{Use: "actions", Short: "x"}

	a := &cobra.Command{Use: "a", Short: "a-short-description"}
	a.Flags().StringP("instances", "p", "", "target instances")
	_ = a.MarkFlagRequired("instances")

	b := &cobra.Command{Use: "b", Short: "b-short-description"}
	b.Flags().StringP("instances", "p", "", "target pods") // different usage text
	_ = b.MarkFlagRequired("instances")

	actions.AddCommand(a, b)
	root.AddCommand(actions)

	skills, _ := New(root).Skills()
	if strings.Contains(skills[0].Body, "Shared subcommand flags") {
		t.Errorf("different flag usage text must prevent collapse:\n%s", skills[0].Body)
	}
}

func TestSiblingsDoNotCollapseWithNoFlags(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	parent := &cobra.Command{Use: "group", Short: "x"}
	parent.AddCommand(&cobra.Command{Use: "a", Short: "a-short-description"})
	parent.AddCommand(&cobra.Command{Use: "b", Short: "b-short-description"})
	root.AddCommand(parent)

	skills, _ := New(root).Skills()
	if strings.Contains(skills[0].Body, "Shared subcommand flags") {
		t.Errorf("siblings with no flags should not produce a shared-flags block:\n%s", skills[0].Body)
	}
}

func TestSiblingsDoNotCollapseWithSingleChild(t *testing.T) {
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	parent := &cobra.Command{Use: "group", Short: "x"}
	parent.AddCommand(makeSiblingLeaf("only", "only child", true, true))
	root.AddCommand(parent)

	skills, _ := New(root).Skills()
	if strings.Contains(skills[0].Body, "Shared subcommand flags") {
		t.Errorf("single-child parent should not collapse:\n%s", skills[0].Body)
	}
}

func TestRootLevelSiblingsCollapse(t *testing.T) {
	// Shared flags at the top level belong on the root's section, not on an
	// intermediate parent.
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	root.AddCommand(makeSiblingLeaf("cycle", "cycle a node", true, true))
	root.AddCommand(makeSiblingLeaf("triage", "triage a node", true, true))

	skills, _ := New(root).Skills()
	body := skills[0].Body
	if !strings.Contains(body, "## Shared subcommand flags") {
		t.Errorf("expected top-level shared-flags section on root:\n%s", body)
	}
	if n := strings.Count(body, "\nFlags:\n"); n != 0 {
		t.Errorf("expected 0 per-child Flags blocks under collapsed root, got %d", n)
	}
}

func TestCollapseDeterministic(t *testing.T) {
	build := func() *cobra.Command {
		root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
		actions := &cobra.Command{Use: "actions", Short: "x"}
		for _, name := range []string{"cycle", "triage", "ignore", "reboot"} {
			actions.AddCommand(makeSiblingLeaf(name, name+" description", true, true))
		}
		root.AddCommand(actions)
		return root
	}
	a, _ := New(build()).Skills()
	b, _ := New(build()).Skills()
	if string(a[0].Bytes()) != string(b[0].Bytes()) {
		t.Errorf("collapsed output not deterministic between runs")
	}
}

func TestSplitModeUnaffectedByCollapse(t *testing.T) {
	// Split mode emits one skill per leaf, each standalone — no duplication
	// to collapse. Sibling-collapse must not alter split output.
	root := &cobra.Command{Use: "mytool", Short: "A reasonable description here"}
	actions := &cobra.Command{Use: "actions", Short: "x"}
	actions.AddCommand(makeSiblingLeaf("cycle", "cycle a node", true, true))
	actions.AddCommand(makeSiblingLeaf("triage", "triage a node", true, true))
	root.AddCommand(actions)

	skills, _ := New(root, WithSplit(SplitPerLeaf)).Skills()
	// Find the cycle leaf — it should still carry its own flag table.
	var cycle Skill
	for _, s := range skills {
		if s.Name == "mytool-actions-cycle" {
			cycle = s
		}
	}
	if !strings.Contains(cycle.Body, "--instances") {
		t.Errorf("split-mode leaf should retain its own flag table:\n%s", cycle.Body)
	}
	if strings.Contains(cycle.Body, "Shared subcommand flags") {
		t.Errorf("split-mode leaf should not carry a shared-flags block:\n%s", cycle.Body)
	}
}
