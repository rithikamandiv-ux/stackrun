// Package supervisor starts services, restarts them according to their
// restart policy, stops them on request, and reports what happens to them
// as events on a channel.
package supervisor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/rithikamandiv-ux/stackrun/internal/config"
	"github.com/rithikamandiv-ux/stackrun/internal/events"
	"github.com/rithikamandiv-ux/stackrun/internal/proc"
)

// eventBufferSize lets the channel absorb bursts of output.
// When it is full, senders wait, which limits memory use (backpressure).
const eventBufferSize = 256

// maxLineSize is the longest output line delivered in one piece.
// Longer lines are split into chunks of this size.
const maxLineSize = 1024 * 1024

// pipeCloseGrace is how long to wait for output pipes to close after a
// service is killed. A process that escaped its group could keep them
// open forever, so after this grace period stackrun closes them itself.
const pipeCloseGrace = 2 * time.Second

// Supervisor runs a set of services and reports their lifecycle as events.
type Supervisor struct {
	cfg       *config.Config
	limits    restartLimits
	events    chan events.Event
	force     chan struct{}
	forceOnce sync.Once
}

// New creates a supervisor for every service in cfg.
func New(cfg *config.Config) *Supervisor {
	return &Supervisor{
		cfg:    cfg,
		limits: defaultRestartLimits,
		events: make(chan events.Event, eventBufferSize),
		force:  make(chan struct{}),
	}
}

// Events returns the channel that receives every event.
// It is closed after Run has finished, once all services have exited.
func (s *Supervisor) Events() <-chan events.Event {
	return s.events
}

// Run starts every service and blocks until all of them have exited for
// good. Cancelling ctx stops every service gracefully: SIGTERM first, then
// SIGKILL after the service's stop timeout. Run must be called once.
func (s *Supervisor) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, name := range s.cfg.ServiceNames() {
		svc := s.cfg.Services[name]
		wg.Go(func() { s.runService(ctx, svc) })
	}
	wg.Wait()
	close(s.events)
}

// ForceStop kills every running service immediately, without waiting
// for stop timeouts. It is safe to call more than once, from any goroutine.
func (s *Supervisor) ForceStop() {
	s.forceOnce.Do(func() { close(s.force) })
}

// shuttingDown reports whether a graceful or forced stop has begun.
func (s *Supervisor) shuttingDown(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	select {
	case <-s.force:
		return true
	default:
		return false
	}
}

// runService runs svc, restarting it according to its restart policy,
// until it exits for good or shutdown begins.
func (s *Supervisor) runService(ctx context.Context, svc *config.Service) {
	attempt := 0
	for !s.shuttingDown(ctx) {
		exit, ran, ok := s.runOnce(ctx, svc)
		if !ok {
			return // start failures are never retried
		}

		outcome := runOutcome{
			exitCode:      exit.ExitCode,
			err:           exit.Err,
			stopRequested: exit.StopRequested,
			duration:      ran,
		}

		// The shuttingDown check covers a service that crashed on its own
		// at the same moment shutdown began.
		restart := shouldRestart(svc.Restart, outcome) && !s.shuttingDown(ctx)
		if restart {
			attempt = s.limits.nextAttempt(attempt, outcome.duration)
		}
		giveUp := restart && attempt > s.limits.maxRestarts

		exit.WillRestart = restart && !giveUp
		s.emit(exit)

		switch {
		case !restart:
			return
		case giveUp:
			s.emit(events.Event{Service: svc.Name, Kind: events.GaveUp, MaxAttempts: s.limits.maxRestarts})
			return
		}

		delay := s.limits.backoffDelay(attempt)
		s.emit(events.Event{
			Service:     svc.Name,
			Kind:        events.Restarting,
			Attempt:     attempt,
			MaxAttempts: s.limits.maxRestarts,
			Delay:       delay,
		})
		if !s.waitBeforeRestart(ctx, delay) {
			return
		}
	}
}

// waitBeforeRestart waits for delay. It returns false if shutdown begins
// first, in which case the service must not be started again.
func (s *Supervisor) waitBeforeRestart(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	case <-s.force:
		return false
	}
}

// runOnce starts the service, supervises it until it exits, and returns
// its Exited event (not yet sent) and how long it ran. ok is false if the
// service failed to start, which has already been reported.
func (s *Supervisor) runOnce(ctx context.Context, svc *config.Service) (exit events.Event, ran time.Duration, ok bool) {
	failed := func(err error) {
		s.emit(events.Event{Service: svc.Name, Kind: events.FailedToStart, Err: err})
	}

	cmd, err := proc.Command(proc.Spec{Command: svc.Command, Dir: svc.Dir, Env: svc.Env})
	if err != nil {
		failed(err)
		return events.Event{}, 0, false
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		failed(err)
		return events.Event{}, 0, false
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		failed(err)
		return events.Event{}, 0, false
	}

	if err := cmd.Start(); err != nil {
		failed(err)
		return events.Event{}, 0, false
	}
	startedAt := time.Now()
	s.emit(events.Event{Service: svc.Name, Kind: events.Started, PID: cmd.Process.Pid})

	var readers sync.WaitGroup
	readers.Go(func() { s.forward(svc.Name, events.Stdout, stdout) })
	readers.Go(func() { s.forward(svc.Name, events.Stderr, stderr) })

	// Wait for the process in the background. Output must be fully read
	// before cmd.Wait, because Wait closes the pipes.
	waitDone := make(chan error, 1)
	go func() {
		readers.Wait()
		waitDone <- cmd.Wait()
	}()

	stopRequested, waitErr := s.supervise(ctx, svc, cmd, waitDone, stdout, stderr)

	exit = exitEvent(svc.Name, cmd, waitErr)
	exit.StopRequested = stopRequested
	return exit, time.Since(startedAt), true
}

// supervise waits for the service to exit, stopping it if shutdown is
// requested. It reports whether a stop was requested, and the result of
// cmd.Wait.
//
// Signals are only sent before waitDone has delivered a result. There is
// a tiny window where Wait has reaped the process but its result has not
// arrived yet; a signal sent then is harmless in practice, because the
// operating system does not reuse a process ID that quickly.
func (s *Supervisor) supervise(ctx context.Context, svc *config.Service, cmd *exec.Cmd,
	waitDone <-chan error, pipes ...io.Closer) (bool, error) {

	forced := false
	select {
	case err := <-waitDone:
		return false, err
	case <-ctx.Done():
	case <-s.force:
		forced = true
	}

	reason := "forced stop"
	if !forced {
		s.emit(events.Event{Service: svc.Name, Kind: events.Stopping})

		if err := proc.Terminate(cmd); err != nil {
			reason = fmt.Sprintf("graceful stop failed: %v", err)
		} else {
			timer := time.NewTimer(svc.StopTimeout)
			defer timer.Stop()

			select {
			case err := <-waitDone:
				return true, err
			case <-timer.C:
				reason = fmt.Sprintf("did not stop within %s", svc.StopTimeout)
			case <-s.force:
			}
		}
	}

	s.emit(events.Event{Service: svc.Name, Kind: events.Killing, Line: reason})
	_ = proc.Kill(cmd) // only fails if the group is already gone

	select {
	case err := <-waitDone:
		return true, err
	case <-time.After(pipeCloseGrace):
		// Closing our end of the pipes unblocks the readers, which lets
		// cmd.Wait run even if an escaped process still holds them open.
		for _, p := range pipes {
			_ = p.Close()
		}
		return true, <-waitDone
	}
}

// forward sends each line read from r as an Output event.
func (s *Supervisor) forward(service string, stream events.Stream, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)
	scanner.Split(scanLinesOrChunks)

	for scanner.Scan() {
		s.emit(events.Event{Service: service, Kind: events.Output, Stream: stream, Line: scanner.Text()})
	}

	// If reading stopped early, keep draining the pipe. A process whose
	// output pipe fills up blocks forever, so the pipe must never be
	// abandoned while the process is still writing.
	if scanner.Err() != nil {
		_, _ = io.Copy(io.Discard, r)
	}
}

// scanLinesOrChunks splits input into lines like bufio.ScanLines, but
// returns an over-long line in maxLineSize chunks instead of failing.
func scanLinesOrChunks(data []byte, atEOF bool) (advance int, token []byte, err error) {
	advance, token, err = bufio.ScanLines(data, atEOF)
	if advance == 0 && token == nil && err == nil && len(data) >= maxLineSize {
		return len(data), data, nil
	}
	return advance, token, err
}

// exitEvent builds the Exited event from the result of cmd.Wait.
func exitEvent(service string, cmd *exec.Cmd, waitErr error) events.Event {
	e := events.Event{Service: service, Kind: events.Exited, ExitCode: events.ExitCodeSignal}

	if cmd.ProcessState != nil {
		// ExitCode returns -1 when the process was stopped by a signal,
		// which matches events.ExitCodeSignal.
		e.ExitCode = cmd.ProcessState.ExitCode()
	}

	// A non-zero exit is reported as *exec.ExitError, which is normal and
	// already captured by ExitCode. Any other error is unexpected.
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		e.Err = waitErr
	}
	return e
}

func (s *Supervisor) emit(e events.Event) {
	e.Time = time.Now()
	s.events <- e
}
