// Package proc builds operating system commands for services.
package proc

import (
	"errors"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// errNotStarted is returned when signalling a command that never started.
var errNotStarted = errors.New("process has not been started")

// Spec describes a command to run.
type Spec struct {
	Command string
	Dir     string
	Env     map[string]string
}

// Command builds an *exec.Cmd for spec without starting it.
// The command runs through the system shell, in spec.Dir, with the
// current environment plus spec.Env. Stdin is left nil, so the process
// reads from the null device instead of competing for the terminal.
func Command(spec Spec) (*exec.Cmd, error) {
	cmd, err := shellCommand(spec.Command)
	if err != nil {
		return nil, err
	}

	overrides := make(map[string]string, len(spec.Env)+1)
	maps.Copy(overrides, spec.Env)

	// The inherited PWD would point at the folder stackrun started in.
	// Shells and tools read PWD, so it must match the real directory.
	if _, set := overrides["PWD"]; !set && spec.Dir != "" {
		overrides["PWD"] = spec.Dir
	}

	cmd.Dir = spec.Dir
	cmd.Env = mergeEnv(os.Environ(), overrides)
	return cmd, nil
}

// mergeEnv returns base ("KEY=value" entries) with overrides applied.
// Overridden keys are removed from base so each key appears once.
func mergeEnv(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))

	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, overridden := overrides[key]; overridden {
			continue
		}
		result = append(result, entry)
	}

	for _, key := range slices.Sorted(maps.Keys(overrides)) {
		result = append(result, key+"="+overrides[key])
	}
	return result
}
