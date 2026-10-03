package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/rithikamandiv-ux/stackrun/internal/config"
	"github.com/rithikamandiv-ux/stackrun/internal/events"
	"github.com/rithikamandiv-ux/stackrun/internal/output"
	"github.com/rithikamandiv-ux/stackrun/internal/supervisor"
)

func newUpCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Start all services and stream their output",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(opts.configPath)
			if err != nil {
				return err
			}

			names := cfg.ServiceNames()
			out := cmd.OutOrStdout()
			printer := output.NewPrinter(out, names, useColour(out))

			noun := "services"
			if len(names) == 1 {
				noun = "service"
			}
			printer.Info(fmt.Sprintf("starting %d %s", len(names), noun))

			sup := supervisor.New(cfg)
			go sup.Run()

			failed := 0
			for e := range sup.Events() {
				printer.Handle(e)
				if isFailure(e) {
					failed++
				}
			}

			if failed > 0 {
				return fmt.Errorf("%d of %d %s did not exit cleanly", failed, len(names), noun)
			}
			printer.Info("all services exited")
			return nil
		},
	}
}

// isFailure reports whether an event means a service did not succeed.
func isFailure(e events.Event) bool {
	switch e.Kind {
	case events.FailedToStart:
		return true
	case events.Exited:
		return e.ExitCode != 0 || e.Err != nil
	}
	return false
}

// useColour enables colour only when writing directly to a terminal.
func useColour(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && output.ShouldUseColour(f)
}
