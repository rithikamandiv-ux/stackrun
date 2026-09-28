package cli

import "github.com/spf13/cobra"

// newRootCmd builds the full command tree.
// A constructor function (instead of a global variable) gives
// every test a fresh command with no leftover state.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "stackrun",
		Short: "Run and manage multiple local development services",
		Long: `stackrun starts the services described in a stackrun.yaml file,
merges their logs, restarts them if they crash, and stops them cleanly.`,
		SilenceUsage: true,
	}

	root.AddCommand(newVersionCmd())
	return root
}

// Execute runs the CLI. It is the only function main needs.
func Execute() error {
	return newRootCmd().Execute()
}
