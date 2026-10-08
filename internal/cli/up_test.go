package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rithikamandiv-ux/stackrun/internal/config"
)

func runUpCommand(t *testing.T, content string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stackrun.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"up", "--config", path})

	err := cmd.Execute()
	return out.String(), err
}

func TestUpStreamsOutput(t *testing.T) {
	out, err := runUpCommand(t, "services:\n  api:\n    command: echo hello\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"stackrun | starting 1 service\n",
		"api      | hello\n",
		"stackrun | api exited with code 0\n",
		"stackrun | all services exited\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestUpReturnsErrorWhenServiceFails(t *testing.T) {
	_, err := runUpCommand(t, "services:\n  api:\n    command: exit 2\n")

	if err == nil {
		t.Fatal("expected an error when a service exits with a non-zero code")
	}
	if got := ExitCode(err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
}

func TestUpStopsServicesOnSignal(t *testing.T) {
	cfg := &config.Config{Services: map[string]*config.Service{
		"api": {
			Name:        "api",
			Command:     "sleep 30",
			Dir:         t.TempDir(),
			Restart:     config.RestartNever,
			StopTimeout: config.DefaultStopTimeout,
		},
	}}

	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGINT

	var out bytes.Buffer
	begin := time.Now()
	err := runUp(context.Background(), cfg, &out, signals)

	if got := ExitCode(err); got != 130 {
		t.Errorf("exit code = %d, want 130 (error: %v)", got, err)
	}
	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Errorf("shutdown took %v, want well under the stop timeout", elapsed)
	}
	if !strings.Contains(out.String(), "received SIGINT") {
		t.Errorf("output does not mention the signal:\n%s", out.String())
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "exit error", err: &exitError{code: 130}, want: 130},
		{name: "wrapped exit error", err: fmt.Errorf("context: %w", &exitError{code: 143}), want: 143},
		{name: "ordinary error", err: errors.New("boom"), want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestUpSucceedsWhenRestartRecovers(t *testing.T) {
	// The first run creates a marker file and fails; the restart finds the
	// marker and succeeds, simulating a service that recovers.
	cfg := &config.Config{Services: map[string]*config.Service{
		"api": {
			Name:        "api",
			Command:     "if [ -f crashed ]; then echo recovered; else touch crashed; exit 1; fi",
			Dir:         t.TempDir(),
			Restart:     config.RestartOnFailure,
			StopTimeout: config.DefaultStopTimeout,
		},
	}}

	var out bytes.Buffer
	err := runUp(context.Background(), cfg, &out, make(chan os.Signal))

	if err != nil {
		t.Fatalf("unexpected error after a successful restart: %v\noutput:\n%s", err, out.String())
	}
	for _, want := range []string{
		"stackrun | restarting api in 1s (attempt 1 of 5)\n",
		"api      | recovered\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out.String())
		}
	}
}
