//go:build unix

package supervisor

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/rithikamandiv-ux/stackrun/internal/events"
)

// processExists asks the OS whether pid exists, using signal 0.
func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// waitUntilGone fails the test if pid still exists after two seconds.
func waitUntilGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for processExists(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d is still running", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunStopsServicesWhenContextIsCancelled(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "sleep 30", "web": "sleep 30"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup := New(cfg)
	t.Cleanup(sup.ForceStop)
	go sup.Run(ctx)

	begin := time.Now()
	started := 0
	all := collect(t, sup, func(e events.Event) {
		if e.Kind == events.Started {
			started++
			if started == 2 {
				cancel()
			}
		}
	})

	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Errorf("shutdown took %v, want well under the stop timeout", elapsed)
	}
	for _, name := range []string{"api", "web"} {
		evs := eventsFor(all, name)
		if !slices.Contains(kinds(evs), events.Stopping) {
			t.Errorf("%s: no Stopping event in %v", name, kinds(evs))
		}
		if slices.Contains(kinds(evs), events.Killing) {
			t.Errorf("%s: was killed, want a graceful stop", name)
		}
		if last := evs[len(evs)-1]; last.Kind != events.Exited || !last.StopRequested {
			t.Errorf("%s: last event = %+v, want Exited with StopRequested", name, last)
		}
	}
}

func TestRunKillsServiceThatIgnoresTerm(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "trap '' TERM; echo ready; sleep 30"})
	cfg.Services["api"].StopTimeout = 300 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup := New(cfg)
	t.Cleanup(sup.ForceStop)
	go sup.Run(ctx)

	// "ready" is printed after the trap is installed, so cancelling only
	// then guarantees SIGTERM is actually ignored.
	all := collect(t, sup, func(e events.Event) {
		if e.Kind == events.Output && e.Line == "ready" {
			cancel()
		}
	})

	evs := eventsFor(all, "api")
	for _, want := range []events.Kind{events.Stopping, events.Killing} {
		if !slices.Contains(kinds(evs), want) {
			t.Errorf("no %v event in %v", want, kinds(evs))
		}
	}
	last := evs[len(evs)-1]
	if last.Kind != events.Exited || !last.StopRequested || last.ExitCode != events.ExitCodeSignal {
		t.Errorf("last event = %+v, want Exited by signal with StopRequested", last)
	}
}

func TestForceStopSkipsTheStopTimeout(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "trap '' TERM; echo ready; sleep 30"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup := New(cfg)
	t.Cleanup(sup.ForceStop)
	go sup.Run(ctx)

	begin := time.Now()
	all := collect(t, sup, func(e events.Event) {
		switch {
		case e.Kind == events.Output && e.Line == "ready":
			cancel()
		case e.Kind == events.Stopping:
			sup.ForceStop()
		}
	})

	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Errorf("forced shutdown took %v, want it to skip the %v stop timeout",
			elapsed, cfg.Services["api"].StopTimeout)
	}
	if !slices.Contains(kinds(eventsFor(all, "api")), events.Killing) {
		t.Error("no Killing event after ForceStop")
	}
}

func TestShutdownLeavesNoOrphans(t *testing.T) {
	cfg := newConfig(t, map[string]string{"api": "sleep 30 & echo $!; wait"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup := New(cfg)
	t.Cleanup(sup.ForceStop)
	go sup.Run(ctx)

	child := 0
	collect(t, sup, func(e events.Event) {
		if e.Kind == events.Output && child == 0 {
			if pid, err := strconv.Atoi(e.Line); err == nil {
				child = pid
				cancel()
			}
		}
	})

	if child == 0 {
		t.Fatal("the service never printed its background child's PID")
	}
	waitUntilGone(t, child)
}
