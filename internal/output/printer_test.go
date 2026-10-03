package output

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rithikamandiv-ux/stackrun/internal/events"
)

func TestPrinterAlignsPrefixes(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, []string{"api", "database"}, false)

	p.Info("starting")
	p.Handle(events.Event{Service: "api", Kind: events.Output, Stream: events.Stdout, Line: "hello"})
	p.Handle(events.Event{Service: "database", Kind: events.Output, Stream: events.Stderr, Line: "ready"})

	want := "" +
		"stackrun | starting\n" +
		"api      | hello\n" +
		"database | ready\n"
	if got := buf.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrinterLifecycleMessages(t *testing.T) {
	tests := []struct {
		name  string
		event events.Event
		want  string
	}{
		{
			name:  "started",
			event: events.Event{Service: "api", Kind: events.Started, PID: 42},
			want:  "stackrun | started api (pid 42)\n",
		},
		{
			name:  "exited with code",
			event: events.Event{Service: "api", Kind: events.Exited, ExitCode: 1},
			want:  "stackrun | api exited with code 1\n",
		},
		{
			name:  "stopped by signal",
			event: events.Event{Service: "api", Kind: events.Exited, ExitCode: events.ExitCodeSignal},
			want:  "stackrun | api was stopped by a signal\n",
		},
		{
			name:  "failed to start",
			event: events.Event{Service: "api", Kind: events.FailedToStart, Err: errors.New("boom")},
			want:  "stackrun | api failed to start: boom\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			NewPrinter(&buf, []string{"api"}, false).Handle(tt.event)

			if got := buf.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrinterColour(t *testing.T) {
	event := events.Event{Service: "api", Kind: events.Output, Line: "hello"}

	var plain, coloured bytes.Buffer
	NewPrinter(&plain, []string{"api"}, false).Handle(event)
	NewPrinter(&coloured, []string{"api"}, true).Handle(event)

	if strings.Contains(plain.String(), "\x1b[") {
		t.Errorf("plain output contains escape codes: %q", plain.String())
	}
	if !strings.HasPrefix(coloured.String(), palette[0]) || !strings.Contains(coloured.String(), reset) {
		t.Errorf("coloured output missing expected codes: %q", coloured.String())
	}
}

func TestPrinterIsSafeForConcurrentUse(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, []string{"api"}, false)

	const goroutines, linesEach = 10, 100
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range linesEach {
				p.Handle(events.Event{Service: "api", Kind: events.Output, Line: "line"})
			}
		})
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != goroutines*linesEach {
		t.Fatalf("got %d lines, want %d", len(lines), goroutines*linesEach)
	}
	for _, line := range lines {
		if line != "api      | line" {
			t.Fatalf("corrupted line: %q", line)
		}
	}
}

func TestColourAllowedByEnv(t *testing.T) {
	tests := []struct {
		name    string
		noColor string
		term    string
		want    bool
	}{
		{name: "normal terminal", noColor: "", term: "xterm-256color", want: true},
		{name: "NO_COLOR set", noColor: "1", term: "xterm-256color", want: false},
		{name: "dumb terminal", noColor: "", term: "dumb", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)

			if got := colourAllowedByEnv(); got != tt.want {
				t.Errorf("colourAllowedByEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldUseColourIsFalseForFiles(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if ShouldUseColour(f) {
		t.Error("colour enabled when writing to a regular file")
	}
}
