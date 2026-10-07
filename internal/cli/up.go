package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

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

			signals := make(chan os.Signal, 2)
			signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(signals)

			return runUp(cmd.Context(), cfg, cmd.OutOrStdout(), signals)
		},
	}
}

// runUp runs every service until all have exited. The first signal starts
// a graceful shutdown, and a second signal forces it. Taking the signals
// as a channel lets tests send fake signals.
func runUp(ctx context.Context, cfg *config.Config, out io.Writer, signals <-chan os.Signal) error {
	names := cfg.ServiceNames()
	count := pluralServices(len(names))
	printer := output.NewPrinter(out, names, useColour(out))
	printer.Info("starting " + count)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sup := supervisor.New(cfg)
	go sup.Run(ctx)

	var received os.Signal
	failed := 0

	// evs is set to nil once the channel closes. Receiving from a nil
	// channel blocks forever, so select ignores it, and the loop ends.
	evs := sup.Events()
	for evs != nil {
		select {
		case e, ok := <-evs:
			if !ok {
				evs = nil
				continue
			}
			printer.Handle(e)
			if isFailure(e) {
				failed++
			}
		case sig := <-signals:
			if received == nil {
				received = sig
				printer.Info(fmt.Sprintf("received %s, stopping %s (press Ctrl+C again to force)",
					signalName(sig), count))
				cancel()
			} else {
				printer.Info("received a second signal, killing all services")
				sup.ForceStop()
			}
		}
	}

	if received != nil {
		printer.Info("all services stopped")
		return &exitError{code: signalExitCode(received)}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %s did not exit cleanly", failed, count)
	}
	printer.Info("all services exited")
	return nil
}

// isFailure reports whether an event means a service did not succeed.
// A service that exits because stackrun stopped it is not a failure.
func isFailure(e events.Event) bool {
	switch e.Kind {
	case events.FailedToStart:
		return true
	case events.Exited:
		return !e.StopRequested && (e.ExitCode != 0 || e.Err != nil)
	}
	return false
}

func pluralServices(n int) string {
	if n == 1 {
		return "1 service"
	}
	return fmt.Sprintf("%d services", n)
}

func signalName(sig os.Signal) string {
	switch sig {
	case os.Interrupt:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	}
	return sig.String()
}

// signalExitCode follows the Unix convention of 128 plus the signal
// number, so SIGINT (2) gives 130 and SIGTERM (15) gives 143.
func signalExitCode(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 1
}

// useColour enables colour only when writing directly to a terminal.
func useColour(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && output.ShouldUseColour(f)
}
