// Package output formats supervisor events for a terminal.
package output

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/term"

	"github.com/rithikamandiv-ux/stackrun/internal/events"
)

// systemName is the prefix used for stackrun's own messages.
const systemName = "stackrun"

// ANSI escape codes. A terminal interprets these as formatting
// instructions instead of printing them.
const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
)

var palette = []string{
	"\x1b[36m", // cyan
	"\x1b[33m", // yellow
	"\x1b[35m", // magenta
	"\x1b[32m", // green
	"\x1b[34m", // blue
	"\x1b[91m", // bright red
}

// Printer writes events as aligned, optionally coloured lines.
// It is safe for concurrent use.
type Printer struct {
	mu      sync.Mutex
	w       io.Writer
	colour  bool
	width   int
	colours map[string]string
}

// NewPrinter creates a printer for the given service names.
// Colours are assigned in the order the names are given, so passing
// sorted names keeps colours stable between runs.
func NewPrinter(w io.Writer, services []string, colour bool) *Printer {
	width := len(systemName)
	colours := make(map[string]string, len(services))
	for i, name := range services {
		width = max(width, len(name))
		colours[name] = palette[i%len(palette)]
	}

	return &Printer{w: w, colour: colour, width: width, colours: colours}
}

// Handle prints one event.
func (p *Printer) Handle(e events.Event) {
	switch e.Kind {
	case events.Output:
		p.line(e.Service, e.Line)
	case events.Started:
		p.Info(fmt.Sprintf("started %s (pid %d)", e.Service, e.PID))
	case events.Exited:
		switch {
		case e.Err != nil:
			p.Info(fmt.Sprintf("%s exited: %v", e.Service, e.Err))
		case e.ExitCode == events.ExitCodeSignal:
			p.Info(fmt.Sprintf("%s was stopped by a signal", e.Service))
		default:
			p.Info(fmt.Sprintf("%s exited with code %d", e.Service, e.ExitCode))
		}
	case events.FailedToStart:
		p.Info(fmt.Sprintf("%s failed to start: %v", e.Service, e.Err))
	}
}

// Info prints a message from stackrun itself.
func (p *Printer) Info(msg string) {
	p.line(systemName, msg)
}

func (p *Printer) line(name, text string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	prefix := fmt.Sprintf("%-*s |", p.width, name)
	if p.colour {
		prefix = p.colourFor(name) + prefix + reset
	}
	fmt.Fprintf(p.w, "%s %s\n", prefix, text)
}

func (p *Printer) colourFor(name string) string {
	if name == systemName {
		return bold
	}
	return p.colours[name]
}

// ShouldUseColour reports whether coloured output suits f: it must be
// a terminal, and the environment must not ask for plain output.
func ShouldUseColour(f *os.File) bool {
	return colourAllowedByEnv() && term.IsTerminal(int(f.Fd()))
}

// colourAllowedByEnv follows two widely used conventions: a non-empty
// NO_COLOR disables colour, and TERM=dumb means the terminal cannot
// display formatting.
func colourAllowedByEnv() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return os.Getenv("TERM") != "dumb"
}
