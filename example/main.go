// Command mytool is a runnable example of a CLI that exposes an agent skill
// via skillgen. Try:
//
//	go run ./example skills print
//	go run ./example skills generate --dir ./out
package main

import (
	"os"

	"github.com/bueti/skillgen"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "mytool",
		Short: "Build and deploy mytool services",
		Long: "mytool is a small example CLI used to demonstrate agent-skill " +
			"generation via the skillgen library.",
		Annotations: map[string]string{
			skillgen.AnnotationTrigger: "build, deploy, promote, ship, or release a mytool service",
		},
	}
	root.PersistentFlags().Bool("verbose", false, "enable verbose logging")

	build := &cobra.Command{
		Use:     "build <service>",
		Short:   "Build an artifact of a service",
		Example: "mytool build api --tag v1.2.3",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("would build %s\n", args[0])
			return nil
		},
	}
	build.Flags().String("tag", "latest", "image tag to apply")
	build.Flags().Bool("push", false, "push the built image after building")

	deploy := &cobra.Command{
		Use:     "deploy <service>",
		Short:   "Deploy a built artifact to an environment",
		Long:    "Deploy promotes an already-built artifact of a service into a named environment.",
		Example: "mytool deploy api --env staging",
		Args:    cobra.ExactArgs(1),
		Annotations: map[string]string{
			skillgen.AnnotationExamples: "Tip: pair with `--dry-run` to preview before applying.",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, _ := cmd.Flags().GetString("env")
			cmd.Printf("would deploy %s to %s\n", args[0], env)
			return nil
		},
	}
	deploy.Flags().String("env", "", "target environment (staging|prod)")
	_ = deploy.MarkFlagRequired("env")
	deploy.Flags().Bool("dry-run", false, "print the plan without applying")

	status := &cobra.Command{
		Use:   "status <service>",
		Short: "Show current deployment state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("status: %s is healthy\n", args[0])
			return nil
		},
	}
	status.Flags().String("env", "staging", "environment to query")

	root.AddCommand(build, deploy, status)
	root.AddCommand(skillgen.NewSkillsCmd(root))

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
