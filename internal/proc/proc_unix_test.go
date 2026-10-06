//go:build unix

package proc

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// service is a started command whose exit is watched in the background.
type service struct {
	cmd    *exec.Cmd
	exited chan struct{} // closed when cmd.Wait returns
}

// startService starts command in dir and waits for it in a goroutine.
// If the test ends while it is still running, cleanup kills its group.
func startService(t *testing.T, command, dir string) *service {
	t.Helper()
	cmd, err := Command(Spec{Command: command, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	s := &service{cmd: cmd, exited: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(s.exited)
	}()

	t.Cleanup(func() {
		select {
		case <-s.exited:
		default:
			_ = Kill(cmd)
			<-s.exited
		}
	})
	return s
}

// exitsWithin reports whether the service exits before the timeout.
func (s *service) exitsWithin(timeout time.Duration) bool {
	select {
	case <-s.exited:
		return true
	case <-time.After(timeout):
		return false
	}
}

// processExists asks the OS whether pid exists, using signal 0, which
// performs the permission and existence checks without sending anything.
func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// waitUntilGone fails the test if pid still exists after two seconds.
// Polling is needed because an orphaned process is removed by the OS
// shortly after it dies, not instantly.
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

// readPID waits for a file containing a PID to be written, then parses it.
func readPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil && strings.HasSuffix(string(data), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatalf("invalid PID %q: %v", data, err)
			}
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("PID file %s was not written", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitForFile fails the test if path does not exist within two seconds.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s was not created", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCommandStartsNewProcessGroup(t *testing.T) {
	s := startService(t, "sleep 30", t.TempDir())
	pid := s.cmd.Process.Pid

	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Fatal(err)
	}

	if pgid != pid {
		t.Errorf("process group = %d, want %d (the service's own PID)", pgid, pid)
	}
	if pgid == syscall.Getpgrp() {
		t.Error("service shares the test's process group")
	}
}

func TestTerminateStopsWholeGroup(t *testing.T) {
	dir := t.TempDir()
	s := startService(t, "sleep 30 & echo $! > child.pid; wait", dir)
	child := readPID(t, filepath.Join(dir, "child.pid"))

	if err := Terminate(s.cmd); err != nil {
		t.Fatalf("Terminate() returned error: %v", err)
	}

	if !s.exitsWithin(5 * time.Second) {
		t.Fatal("shell did not exit after SIGTERM")
	}
	waitUntilGone(t, child)
}

func TestKillStopsProcessIgnoringTerm(t *testing.T) {
	dir := t.TempDir()
	s := startService(t, "trap '' TERM; touch ready; sleep 30", dir)

	// Wait until the trap is installed. A SIGTERM sent earlier would reach
	// the shell before it starts ignoring the signal, and kill it.
	waitForFile(t, filepath.Join(dir, "ready"))

	if err := Terminate(s.cmd); err != nil {
		t.Fatalf("Terminate() returned error: %v", err)
	}
	if s.exitsWithin(300 * time.Millisecond) {
		t.Fatal("process exited even though it ignores SIGTERM")
	}

	if err := Kill(s.cmd); err != nil {
		t.Fatalf("Kill() returned error: %v", err)
	}
	if !s.exitsWithin(5 * time.Second) {
		t.Fatal("process did not exit after SIGKILL")
	}
}

func TestSignalBeforeStartFails(t *testing.T) {
	err := Terminate(&exec.Cmd{})

	if !errors.Is(err, errNotStarted) {
		t.Errorf("error = %v, want errNotStarted", err)
	}
}
