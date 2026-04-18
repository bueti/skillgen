package skillgen

import (
	"fmt"
	"io"
	"path/filepath"

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
				totalTokens += EstimateTokens(b)
				cmd.Printf("wrote %s\n", filepath.Join(dir, s.Path))
				if EstimateTokens([]byte(s.Body)) > SpecMaxBodyTokens {
					oversized = append(oversized, s.Name)
				}
			}

			cmd.Printf("\n%d skill(s), ~%s tokens (%s)\n",
				len(skills), formatThousands(totalTokens), formatBytes(totalBytes))
			for _, name := range oversized {
				cmd.PrintErrf("warning: skill %q body exceeds the spec recommendation (~%d tokens) — consider split mode or moving detail to referenced files\n", name, SpecMaxBodyTokens)
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

// budgetWarnThreshold is the token count above which `skills generate` emits a
// warning. Skills go into agent context on every turn, so a 15k-token skill is
// a meaningful tax even if it's "only" a few KB of markdown.
const budgetWarnThreshold = 15000

// EstimateTokens returns a rough token count for the given bytes. Uses the
// common bytes/4 heuristic for English prose, which is close enough for a
// budget warning (actual tokenizers vary by model).
func EstimateTokens(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	// Round up so tiny skills don't show as 0 tokens.
	return (len(b) + 3) / 4
}

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
	var strict bool
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Report missing or low-quality skill signal in the command tree",
		Long: "Walk the command tree and report issues that would produce a low-quality " +
			"skill — missing descriptions, missing trigger hints, deprecated commands " +
			"without a replacement message, and so on.\n\n" +
			"Exit code is 1 when any error is found. With --strict, warnings are treated " +
			"as errors too — use this in CI to enforce a quality bar.",
		// Lint failures are not usage errors; keep CI output tidy.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			issues := New(root, opts...).Lint()
			cmd.Print(FormatIssues(issues))

			var failing int
			for _, iss := range issues {
				if iss.Level == IssueError || (strict && iss.Level == IssueWarning) {
					failing++
				}
			}
			if failing > 0 {
				return fmt.Errorf("%d lint finding(s) failed the check", failing)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as errors")
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

