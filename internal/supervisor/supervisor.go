// Package supervisor starts services and reports what happens to them
// as events on a channel.
package supervisor

import (
	"bufio"
	"errors"
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

// Supervisor runs a set of services and reports their lifecycle as events.
type Supervisor struct {
	cfg    *config.Config
	events chan events.Event
}

// New creates a supervisor for every service in cfg.
func New(cfg *config.Config) *Supervisor {
	return &Supervisor{
		cfg:    cfg,
		events: make(chan events.Event, eventBufferSize),
	}
}

// Events returns the channel that receives every event.
// It is closed after Run has finished, once all services have exited.
func (s *Supervisor) Events() <-chan events.Event {
	return s.events
}

// Run starts every service and blocks until all of them have exited.
// It must be called exactly once.
func (s *Supervisor) Run() {
	var wg sync.WaitGroup
	for _, name := range s.cfg.ServiceNames() {
		svc := s.cfg.Services[name]
		wg.Go(func() { s.runService(svc) })
	}
	wg.Wait()
	close(s.events)
}

func (s *Supervisor) runService(svc *config.Service) {
	failed := func(err error) {
		s.emit(events.Event{Service: svc.Name, Kind: events.FailedToStart, Err: err})
	}

	cmd, err := proc.Command(proc.Spec{Command: svc.Command, Dir: svc.Dir, Env: svc.Env})
	if err != nil {
		failed(err)
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		failed(err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		failed(err)
		return
	}

	if err := cmd.Start(); err != nil {
		failed(err)
		return
	}
	s.emit(events.Event{Service: svc.Name, Kind: events.Started, PID: cmd.Process.Pid})

	// Both pipes must be fully read before calling Wait, because Wait
	// closes them. Calling it too early can lose the final lines.
	var readers sync.WaitGroup
	readers.Go(func() { s.forward(svc.Name, events.Stdout, stdout) })
	readers.Go(func() { s.forward(svc.Name, events.Stderr, stderr) })
	readers.Wait()

	waitErr := cmd.Wait()
	s.emit(exitEvent(svc.Name, cmd, waitErr))
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
