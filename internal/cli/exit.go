package cli

import (
	"errors"
	"fmt"
)

// exitError asks main to exit with a specific code. It is used when the
// outcome has already been reported, so no error message is printed.
type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

// ExitCode returns the process exit code for an error returned by Execute.
func ExitCode(err error) int {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return 1
}
