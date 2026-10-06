// Package config loads, normalises and validates stackrun configuration files.
package config

import (
	"maps"
	"slices"
	"time"
)

const (
	// DefaultStopTimeout is how long a service gets to stop after SIGTERM
	// before it is killed. Docker Compose uses the same default.
	DefaultStopTimeout = 10 * time.Second

	// MaxStopTimeout guards against values that would make shutdown hang
	// for an unreasonable time, such as a mistyped "10h".
	MaxStopTimeout = 5 * time.Minute
)

// Config is the top-level structure of a stackrun.yaml file.
type Config struct {
	Services map[string]*Service `yaml:"services"`
}

// Service describes one process that stackrun manages.
type Service struct {
	Name        string            `yaml:"-"`
	Command     string            `yaml:"command"`
	Dir         string            `yaml:"dir"`
	Env         map[string]string `yaml:"env"`
	Restart     RestartPolicy     `yaml:"restart"`
	StopTimeout time.Duration     `yaml:"stop_timeout"`
}

// RestartPolicy controls what happens when a service exits.
type RestartPolicy string

const (
	RestartNever     RestartPolicy = "never"
	RestartOnFailure RestartPolicy = "on-failure"
	RestartAlways    RestartPolicy = "always"
)

// IsValid reports whether p is one of the supported restart policies.
func (p RestartPolicy) IsValid() bool {
	switch p {
	case RestartNever, RestartOnFailure, RestartAlways:
		return true
	}
	return false
}

// ServiceNames returns the service names in alphabetical order.
// Go randomises map iteration order, so every loop over services
// should use this method to keep output and errors deterministic.
func (c *Config) ServiceNames() []string {
	return slices.Sorted(maps.Keys(c.Services))
}
