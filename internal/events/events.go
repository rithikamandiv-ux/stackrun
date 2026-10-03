// Package events defines the messages that describe what happens to services.
// The supervisor produces events and output components such as the printer
// consume them, so neither needs to know about the other.
package events

import (
	"fmt"
	"time"
)

// Kind identifies what happened.
type Kind int

const (
	// Started means the process is running. PID is set.
	Started Kind = iota + 1
	// Output is one line printed by the service. Stream and Line are set.
	Output
	// Exited means the process has ended. ExitCode is set, and Err is set
	// if the exit status could not be determined.
	Exited
	// FailedToStart means the process could not be launched. Err is set.
	FailedToStart
)

func (k Kind) String() string {
	switch k {
	case Started:
		return "Started"
	case Output:
		return "Output"
	case Exited:
		return "Exited"
	case FailedToStart:
		return "FailedToStart"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Stream identifies which output pipe a line came from.
type Stream int

const (
	Stdout Stream = iota + 1
	Stderr
)

// ExitCodeSignal is the exit code reported when a process was stopped
// by a signal instead of exiting normally.
const ExitCodeSignal = -1

// Event describes one thing that happened to one service.
type Event struct {
	Time     time.Time
	Service  string
	Kind     Kind
	PID      int
	Stream   Stream
	Line     string
	ExitCode int
	Err      error
}
