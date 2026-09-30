package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rithikamandiv-ux/stackrun/internal/config"
)

func newValidateCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check the config file for errors",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(opts.configPath)
			if err != nil {
				return err
			}

			names := cfg.ServiceNames()
			noun := "services"
			if len(names) == 1 {
				noun = "service"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Config is valid: %d %s (%s)\n",
				len(names), noun, strings.Join(names, ", "))
			return nil
		},
	}
}
