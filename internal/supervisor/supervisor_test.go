package supervisor

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rithikamandiv-ux/stackrun/internal/config"
	"github.com/rithikamandiv-ux/stackrun/internal/events"
)

// testRestartLimits keeps restart tests fast: milliseconds instead of seconds.
var testRestartLimits = restartLimits{
	maxRestarts: 3,
	baseDelay:   10 * time.Millisecond,
	maxDelay:    50 * time.Millisecond,
	stableAfter: time.Second,
}

// newConfig builds a valid config where each service runs in its own temp dir.
func newConfig(t *testing.T, commands map[string]string) *config.Config {
	t.Helper()
	cfg := &config.Config{Services: map[string]*config.Service{}}
	for name, command := range commands {
		cfg.Services[name] = &config.Service{
			Name:        name,
			Command:     command,
			Dir:         t.TempDir(),
			Restart:     config.RestartNever,
			StopTimeout: config.DefaultStopTimeout,
		}
	}
	return cfg
}

// startSupervisor runs a supervisor for cfg with the given restart limits.
// It returns the supervisor and a function that starts a graceful shutdown.
// Cleanup stops everything if a test ends early.
func startSupervisor(t *testing.T, cfg *config.Config, limits restartLimits) (*Supervisor, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	sup := New(cfg)
	sup.limits = limits
	t.Cleanup(func() {
		cancel()
		sup.ForceStop()
	})

	go sup.Run(ctx)
	return sup, cancel
}

// runAndCollect runs the supervisor without cancelling it and returns
// every event it sends.
func runAndCollect(t *testing.T, cfg *config.Config) []events.Event {
	t.Helper()
	sup, _ := startSupervisor(t, cfg, testRestartLimits)
	return collect(t, sup, nil)
}

// collect returns every event the supervisor sends, calling hook (if not
// nil) for each event as it arrives. It fails the test if the event
// channel is not closed in time.
func collect(t *testing.T, sup *Supervisor, hook func(events.Event)) []events.Event {
	t.Helper()
	var got []events.Event
	timeout := time.After(15 * time.Second)
	for {
		select {
		case e, ok := <-sup.Events():
			if !ok {
				return got
			}
			got = append(got, e)
			if hook != nil {
				hook(e)
			}
		case <-timeout:
			t.Fatal("timed out waiting for services to exit")
		}
	}
}

// eventsFor returns the events of one service, in the order received.
func eventsFor(all []events.Event, service string) []events.Event {
	var result []events.Event
	for _, e := range all {
		if e.Service == service {
			result = append(result, e)
		}
	}
	return result
}

// ofKind returns the events of one kind, in order.
func ofKind(evs []events.Event, kind events.Kind) []events.Event {
	var result []events.Event
	for _, e := range evs {
		if e.Kind == kind {
			result = append(result, e)
		}
	}
	return result
}

// kinds returns the kind of every event, in order.
func kinds(evs []events.Event) []events.Kind {
	result := make([]events.Kind, len(evs))
	for i, e := range evs {
		result[i] = e.Kind
	}
	return result
}

// lines returns the output lines of one service on one stream.
func lines(evs []events.Event, stream events.Stream) []string {
	var result []string
	for _, e := range evs {
		if e.Kind == events.Output && e.Stream == stream {
			result = append(result, e.Line)
		}
	}
	return result
}

func TestRunCapturesOutputAndExitCode(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "echo hello; echo oops >&2; exit 3"})

	evs := eventsFor(runAndCollect(t, cfg), "api")

	first, last := evs[0], evs[len(evs)-1]
	if first.Kind != events.Started || first.PID <= 0 {
		t.Errorf("first event = %v (pid %d), want Started with a PID", first.Kind, first.PID)
	}
	if last.Kind != events.Exited || last.ExitCode != 3 || last.StopRequested || last.WillRestart {
		t.Errorf("last event = %+v, want a final Exited with code 3", last)
	}
	if got := lines(evs, events.Stdout); !slices.Equal(got, []string{"hello"}) {
		t.Errorf("stdout = %v, want [hello]", got)
	}
	if got := lines(evs, events.Stderr); !slices.Equal(got, []string{"oops"}) {
		t.Errorf("stderr = %v, want [oops]", got)
	}
}

func TestRunMultipleServices(t *testing.T) {
	cfg := newConfig(t, map[string]string{"a": "echo from-a", "b": "echo from-b"})

	all := runAndCollect(t, cfg)

	for _, name := range []string{"a", "b"} {
		evs := eventsFor(all, name)
		if got, want := lines(evs, events.Stdout), []string{"from-" + name}; !slices.Equal(got, want) {
			t.Errorf("%s stdout = %v, want %v", name, got, want)
		}
		if last := evs[len(evs)-1]; last.Kind != events.Exited || last.ExitCode != 0 {
			t.Errorf("%s last event = %v (code %d), want Exited with code 0", name, last.Kind, last.ExitCode)
		}
	}
}

func TestRunPreservesLineOrderWithinStream(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "for i in 1 2 3 4 5; do echo $i; done"})

	got := lines(eventsFor(runAndCollect(t, cfg), "api"), events.Stdout)

	if want := []string{"1", "2", "3", "4", "5"}; !slices.Equal(got, want) {
		t.Errorf("stdout = %v, want %v", got, want)
	}
}

func TestRunReportsFailedToStart(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "echo never"})
	cfg.Services["api"].Dir = filepath.Join(t.TempDir(), "missing")

	evs := runAndCollect(t, cfg)

	if len(evs) != 1 || evs[0].Kind != events.FailedToStart || evs[0].Err == nil {
		t.Fatalf("events = %+v, want a single FailedToStart with an error", evs)
	}
}

func TestRunSplitsVeryLongLines(t *testing.T) {
	const length = 1_500_000
	cfg := newConfig(t, map[string]string{"api": "head -c 1500000 /dev/zero | tr '\\0' 'x'; echo"})

	got := lines(eventsFor(runAndCollect(t, cfg), "api"), events.Stdout)

	total := 0
	for _, chunk := range got {
		if len(chunk) > maxLineSize {
			t.Errorf("chunk of %d bytes exceeds maxLineSize", len(chunk))
		}
		if strings.Trim(chunk, "x") != "" {
			t.Errorf("chunk contains unexpected characters")
		}
		total += len(chunk)
	}
	if total != length {
		t.Errorf("received %d bytes in %d chunks, want %d bytes", total, len(got), length)
	}
}

func TestRunDoesNotStartServicesAfterCancellation(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "echo should-not-run"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sup := New(cfg)
	go sup.Run(ctx)

	if evs := collect(t, sup, nil); len(evs) != 0 {
		t.Errorf("events = %+v, want none", evs)
	}
}

func TestOnFailureRestartsUntilGivingUp(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "exit 1"})
	cfg.Services["api"].Restart = config.RestartOnFailure
	maxRestarts := testRestartLimits.maxRestarts

	evs := eventsFor(runAndCollect(t, cfg), "api")

	if got := len(ofKind(evs, events.Started)); got != maxRestarts+1 {
		t.Errorf("started %d times, want %d (first run plus %d restarts)", got, maxRestarts+1, maxRestarts)
	}

	var attempts []int
	for _, e := range ofKind(evs, events.Restarting) {
		attempts = append(attempts, e.Attempt)
	}
	if want := []int{1, 2, 3}; !slices.Equal(attempts, want) {
		t.Errorf("restart attempts = %v, want %v", attempts, want)
	}

	exits := ofKind(evs, events.Exited)
	for i, e := range exits {
		if want := i < len(exits)-1; e.WillRestart != want {
			t.Errorf("exit %d: WillRestart = %v, want %v", i+1, e.WillRestart, want)
		}
	}

	if last := evs[len(evs)-1]; last.Kind != events.GaveUp {
		t.Errorf("last event = %v, want GaveUp", last.Kind)
	}
}

func TestAlwaysRestartsAfterSuccess(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "echo hi"})
	cfg.Services["api"].Restart = config.RestartAlways

	evs := eventsFor(runAndCollect(t, cfg), "api")

	if got, want := len(ofKind(evs, events.Started)), testRestartLimits.maxRestarts+1; got != want {
		t.Errorf("started %d times, want %d", got, want)
	}
}

func TestOnFailureDoesNotRestartAfterSuccess(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "exit 0"})
	cfg.Services["api"].Restart = config.RestartOnFailure

	evs := eventsFor(runAndCollect(t, cfg), "api")

	if got := len(ofKind(evs, events.Started)); got != 1 {
		t.Errorf("started %d times, want 1", got)
	}
	if got := len(ofKind(evs, events.Restarting)); got != 0 {
		t.Errorf("got %d Restarting events, want 0", got)
	}
}

func TestStartFailuresAreNotRetried(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "echo never"})
	cfg.Services["api"].Dir = filepath.Join(t.TempDir(), "missing")
	cfg.Services["api"].Restart = config.RestartAlways

	evs := runAndCollect(t, cfg)

	if !slices.Equal(kinds(evs), []events.Kind{events.FailedToStart}) {
		t.Errorf("event kinds = %v, want only FailedToStart", kinds(evs))
	}
}

func TestShutdownDuringBackoffPreventsRestart(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "exit 1"})
	cfg.Services["api"].Restart = config.RestartOnFailure

	slow := testRestartLimits
	slow.baseDelay = 10 * time.Second
	slow.maxDelay = 10 * time.Second
	sup, cancel := startSupervisor(t, cfg, slow)

	begin := time.Now()
	evs := eventsFor(collect(t, sup, func(e events.Event) {
		if e.Kind == events.Restarting {
			cancel()
		}
	}), "api")

	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Errorf("shutdown took %v, want it to interrupt the 10s backoff", elapsed)
	}
	if got := len(ofKind(evs, events.Started)); got != 1 {
		t.Errorf("started %d times, want 1 (no restart after shutdown)", got)
	}
	if last := evs[len(evs)-1]; last.Kind != events.Restarting {
		t.Errorf("last event = %v, want Restarting (the interrupted wait)", last.Kind)
	}
}
