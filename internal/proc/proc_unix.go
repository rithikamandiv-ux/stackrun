//go:build unix

package proc

import (
	"errors"
	"os/exec"
	"syscall"
)

// shellCommand wraps command in the POSIX shell so features such as
// pipes, && and variable expansion work as they do in a terminal.
// The shell starts in a new process group whose ID equals its PID,
// so the whole process tree can be signalled together.
func shellCommand(command string) (*exec.Cmd, error) {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

// Terminate politely asks every process in the command's group to stop.
func Terminate(cmd *exec.Cmd) error {
	return signalGroup(cmd, syscall.SIGTERM)
}

// Kill immediately stops every process in the command's group.
func Kill(cmd *exec.Cmd) error {
	return signalGroup(cmd, syscall.SIGKILL)
}

// signalGroup sends sig to the command's process group.
// It must not be called after cmd.Wait has returned: once the process
// is reaped, its ID can be reused by an unrelated process.
func signalGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd.Process == nil {
		return errNotStarted
	}

	// A negative PID addresses the whole process group with that ID.
	err := syscall.Kill(-cmd.Process.Pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return nil // every process in the group has already exited
	}
	return err
}
