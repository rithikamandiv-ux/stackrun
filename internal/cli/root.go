package cli

import "github.com/spf13/cobra"

// rootOptions holds values from flags shared by every subcommand.
type rootOptions struct {
	configPath string
}

// newRootCmd builds the full command tree.
// A constructor function (instead of a global variable) gives
// every test a fresh command with no leftover state.
func newRootCmd() *cobra.Command {
	opts := &rootOptions{}

	root := &cobra.Command{
		Use:   "stackrun",
		Short: "Run and manage multiple local development services",
		Long: `stackrun starts the services described in a stackrun.yaml file,
merges their logs, restarts them if they crash, and stops them cleanly.`,
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVarP(&opts.configPath, "config", "c", "stackrun.yaml", "path to the config file")

	root.AddCommand(newVersionCmd(), newValidateCmd(opts))
	return root
}

// Execute runs the CLI. It is the only function main needs.
func Execute() error {
	return newRootCmd().Execute()
}
