//go:build unix

package proc

import "os/exec"

// shellCommand wraps command in the POSIX shell so features such as
// pipes, && and variable expansion work as they do in a terminal.
func shellCommand(command string) (*exec.Cmd, error) {
	return exec.Command("/bin/sh", "-c", command), nil
}
