package skillgen

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// NewSkillsCmd returns a `skills` subcommand authors can attach to their root
// command, analogous to cobra's `completion`. It exposes:
//
//   - `skills generate --dir <path>` — write skill files to disk.
//   - `skills print` — write the skill(s) to stdout.
//
// The same opts apply to both subcommands. The returned command is marked
// with skill.skip so it never appears in its own generated skill.
func NewSkillsCmd(root *cobra.Command, opts ...Option) *cobra.Command {
	skills := &cobra.Command{
		Use:   "skills",
		Short: "Generate agent skill files describing this CLI",
		Long: "Generate agent skill files (Markdown + YAML frontmatter) that teach AI coding " +
			"agents when and how to invoke this CLI.",
		Annotations: map[string]string{AnnotationSkip: "true"},
	}

	skills.AddCommand(newGenerateCmd(root, opts))
	skills.AddCommand(newPrintCmd(root, opts))
	return skills
}

func newGenerateCmd(root *cobra.Command, opts []Option) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Write generated skill files into --dir",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				return fmt.Errorf("--dir is required")
			}
			g := New(root, opts...)
			if err := g.WriteTo(dir); err != nil {
				return err
			}
			skills, _ := g.Skills()
			for _, s := range skills {
				fmt.Fprintf(cmd.OutOrStdout(), "wrote %s/%s\n", dir, s.Filename)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".claude/skills", "directory to write skill files into")
	return cmd
}

func newPrintCmd(root *cobra.Command, opts []Option) *cobra.Command {
	return &cobra.Command{
		Use:   "print",
		Short: "Print the generated skill(s) to stdout",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printSkills(cmd.OutOrStdout(), root, opts)
		},
	}
}

func printSkills(w io.Writer, root *cobra.Command, opts []Option) error {
	skills, err := New(root, opts...).Skills()
	if err != nil {
		return err
	}
	for i, s := range skills {
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		if _, err := w.Write(s.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

