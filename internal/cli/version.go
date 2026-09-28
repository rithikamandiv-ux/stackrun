package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is replaced at build time for real releases, for example:
// go build -ldflags "-X github.com/rithikamandiv-ux/stackrun/internal/cli.version=v0.1.0"
var version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the stackrun version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "stackrun %s\n", version)
		},
	}
}
