//go:build windows

package proc

import (
	"errors"
	"os/exec"
)

var errWindowsUnsupported = errors.New("stackrun does not support Windows yet")

// shellCommand is a placeholder until Windows support is implemented.
func shellCommand(command string) (*exec.Cmd, error) {
	return nil, errWindowsUnsupported
}

// Terminate is a placeholder until Windows support is implemented.
func Terminate(cmd *exec.Cmd) error {
	return errWindowsUnsupported
}

// Kill is a placeholder until Windows support is implemented.
func Kill(cmd *exec.Cmd) error {
	return errWindowsUnsupported
}
