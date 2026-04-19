package skillgen

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/bueti/skilllint"
	"github.com/bueti/skilllint/rules"
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
	skills.AddCommand(newLintCmd(root, opts))
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

			var totalBytes, totalTokens int
			var oversized []string
			for _, s := range skills {
				b := s.Bytes()
				totalBytes += len(b)
				totalTokens += rules.EstimateTokens(string(b))
				cmd.Printf("wrote %s\n", filepath.Join(dir, s.Path))
				if rules.EstimateTokens(s.Body) > rules.MaxBodyTokens {
					oversized = append(oversized, s.Name)
				}
			}

			cmd.Printf("\n%d skill(s), ~%s tokens (%s)\n",
				len(skills), formatThousands(totalTokens), formatBytes(totalBytes))
			for _, name := range oversized {
				cmd.PrintErrf("warning: skill %q body exceeds the spec recommendation (~%d tokens) — consider split mode or moving detail to referenced files\n", name, rules.MaxBodyTokens)
			}
			if totalTokens > budgetWarnThreshold {
				cmd.PrintErrf("warning: aggregate output is large (~%s tokens) — "+
					"consider --split=per-leaf, skill.skip on operator subtrees, "+
					"or trimming inherited flags\n", formatThousands(totalTokens))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".claude/skills", "directory to write skill files into")
	return cmd
}

// budgetWarnThreshold is the aggregate token count above which
// `skills generate` emits a warning. The per-skill threshold (spec's 5000)
// is owned by skilllint/rules; this one tracks the total across every skill
// the CLI writes in one run, which is a separate concern from any single
// skill's compliance.
const budgetWarnThreshold = 15000

func formatThousands(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func formatBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
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

func newLintCmd(root *cobra.Command, opts []Option) *cobra.Command {
	var (
		strict bool
		format string
	)
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Report missing or low-quality skill signal in the command tree",
		Long: "Walk the command tree and lint the resulting skills.\n\n" +
			"Two passes run: cobra-tree checks (`cmd-*` rule IDs) that inspect command " +
			"metadata, and spec-compliance checks from the skilllint library that inspect " +
			"the generated SKILL.md bytes. Exit code is 1 when any error is found. With " +
			"--strict, warnings are treated as errors too.",
		// Lint failures are not usage errors; keep CI output tidy.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			issues := New(root, opts...).Lint()
			if err := skilllint.Write(cmd.OutOrStdout(), issues, skilllint.Format(format)); err != nil {
				return err
			}
			if skilllint.HasErrors(issues) || (strict && skilllint.HasWarnings(issues)) {
				return fmt.Errorf("lint failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as errors")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, github-actions")
	return cmd
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

