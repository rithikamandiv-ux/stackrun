package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

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
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVarP(&opts.configPath, "config", "c", "stackrun.yaml", "path to the config file")

	root.AddCommand(newVersionCmd(), newValidateCmd(opts), newUpCmd(opts))
	return root
}

// Execute runs the CLI and prints any error. Pass the returned error to
// ExitCode to get the process exit code.
func Execute() error {
	err := newRootCmd().Execute()

	var ee *exitError
	if err != nil && !errors.As(err, &ee) {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
	return err
}
