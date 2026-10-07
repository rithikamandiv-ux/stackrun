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

// runAndCollect runs the supervisor without ever cancelling it and
// returns every event it sends.
func runAndCollect(t *testing.T, cfg *config.Config) []events.Event {
	t.Helper()
	sup := New(cfg)
	t.Cleanup(sup.ForceStop)
	go sup.Run(context.Background())
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
	if last.Kind != events.Exited || last.ExitCode != 3 || last.StopRequested {
		t.Errorf("last event = %v (code %d, stop requested %v), want Exited with code 3",
			last.Kind, last.ExitCode, last.StopRequested)
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
