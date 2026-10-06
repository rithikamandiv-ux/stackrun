package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
)

// serviceNamePattern allows letters, digits, '-' and '_',
// and requires the name to start with a letter or digit.
var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// FieldError describes one problem at a specific location in the config.
type FieldError struct {
	Path    string // for example "services.backend.command"
	Message string // for example "is required"
}

func (e *FieldError) Error() string {
	return e.Path + ": " + e.Message
}

// Validate checks a normalised config and returns every problem found,
// joined into a single error. It returns nil when the config is valid.
func Validate(cfg *Config) error {
	var errs []error
	add := func(path, format string, args ...any) {
		errs = append(errs, &FieldError{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	if len(cfg.Services) == 0 {
		add("services", "at least one service must be defined")
		return errors.Join(errs...)
	}

	for _, name := range cfg.ServiceNames() {
		path := "services." + name
		svc := cfg.Services[name]

		if !serviceNamePattern.MatchString(name) {
			add(path, "name must start with a letter or digit and contain only letters, digits, '-' or '_'")
		}

		if svc == nil {
			add(path, "service definition is empty")
			continue
		}

		if strings.TrimSpace(svc.Command) == "" {
			add(path+".command", "is required")
		}

		if !svc.Restart.IsValid() {
			add(path+".restart", "must be one of never, on-failure, always (got %q)", svc.Restart)
		}

		switch {
		case svc.StopTimeout <= 0:
			add(path+".stop_timeout", "must be greater than zero (got %s)", svc.StopTimeout)
		case svc.StopTimeout > MaxStopTimeout:
			add(path+".stop_timeout", "must be at most %s (got %s)", MaxStopTimeout, svc.StopTimeout)
		}

		if svc.Dir != "" {
			info, err := os.Stat(svc.Dir)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				add(path+".dir", "directory %q does not exist", svc.Dir)
			case err != nil:
				add(path+".dir", "cannot access directory %q: %v", svc.Dir, err)
			case !info.IsDir():
				add(path+".dir", "%q is not a directory", svc.Dir)
			}
		}

		for _, key := range slices.Sorted(maps.Keys(svc.Env)) {
			switch {
			case key == "":
				add(path+".env", "variable names must not be empty")
			case strings.Contains(key, "="):
				add(path+".env."+key, "variable names must not contain '='")
			}
		}
	}

	return errors.Join(errs...)
}
