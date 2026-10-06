package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// validConfig returns a config that passes validation.
// Each test case starts from it and changes one thing.
func validConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{
		Services: map[string]*Service{
			"backend": {
				Name:        "backend",
				Command:     "npm run dev",
				Dir:         t.TempDir(),
				Env:         map[string]string{"PORT": "4000"},
				Restart:     RestartNever,
				StopTimeout: DefaultStopTimeout,
			},
		},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(t *testing.T, cfg *Config)
		wantPaths []string
	}{
		{
			name:      "valid config",
			modify:    func(t *testing.T, cfg *Config) {},
			wantPaths: nil,
		},
		{
			name:      "no services",
			modify:    func(t *testing.T, cfg *Config) { cfg.Services = nil },
			wantPaths: []string{"services"},
		},
		{
			name:      "empty service definition",
			modify:    func(t *testing.T, cfg *Config) { cfg.Services["worker"] = nil },
			wantPaths: []string{"services.worker"},
		},
		{
			name: "invalid service name",
			modify: func(t *testing.T, cfg *Config) {
				cfg.Services["my service"] = cfg.Services["backend"]
				delete(cfg.Services, "backend")
			},
			wantPaths: []string{"services.my service"},
		},
		{
			name:      "blank command",
			modify:    func(t *testing.T, cfg *Config) { cfg.Services["backend"].Command = "   " },
			wantPaths: []string{"services.backend.command"},
		},
		{
			name:      "invalid restart policy",
			modify:    func(t *testing.T, cfg *Config) { cfg.Services["backend"].Restart = "sometimes" },
			wantPaths: []string{"services.backend.restart"},
		},
		{
			name: "directory does not exist",
			modify: func(t *testing.T, cfg *Config) {
				cfg.Services["backend"].Dir = filepath.Join(t.TempDir(), "missing")
			},
			wantPaths: []string{"services.backend.dir"},
		},
		{
			name: "dir is a file",
			modify: func(t *testing.T, cfg *Config) {
				file := filepath.Join(t.TempDir(), "file.txt")
				if err := os.WriteFile(file, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				cfg.Services["backend"].Dir = file
			},
			wantPaths: []string{"services.backend.dir"},
		},
		{
			name:      "empty env name",
			modify:    func(t *testing.T, cfg *Config) { cfg.Services["backend"].Env[""] = "x" },
			wantPaths: []string{"services.backend.env"},
		},
		{
			name:      "env name contains equals",
			modify:    func(t *testing.T, cfg *Config) { cfg.Services["backend"].Env["A=B"] = "x" },
			wantPaths: []string{"services.backend.env.A=B"},
		},
		{
			name:      "zero stop timeout",
			modify:    func(t *testing.T, cfg *Config) { cfg.Services["backend"].StopTimeout = 0 },
			wantPaths: []string{"services.backend.stop_timeout"},
		},
		{
			name:      "negative stop timeout",
			modify:    func(t *testing.T, cfg *Config) { cfg.Services["backend"].StopTimeout = -time.Second },
			wantPaths: []string{"services.backend.stop_timeout"},
		},
		{
			name: "stop timeout too long",
			modify: func(t *testing.T, cfg *Config) {
				cfg.Services["backend"].StopTimeout = MaxStopTimeout + time.Second
			},
			wantPaths: []string{"services.backend.stop_timeout"},
		},
		{
			name: "multiple errors reported together",
			modify: func(t *testing.T, cfg *Config) {
				cfg.Services["backend"].Command = ""
				cfg.Services["backend"].Restart = "bad"
			},
			wantPaths: []string{"services.backend.command", "services.backend.restart"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			tt.modify(t, cfg)

			err := Validate(cfg)

			got := fieldErrorPaths(t, err)
			if !slices.Equal(got, tt.wantPaths) {
				t.Errorf("error paths = %v, want %v\nfull error:\n%v", got, tt.wantPaths, err)
			}
		})
	}
}

func TestServiceNamesAreSorted(t *testing.T) {
	cfg := &Config{Services: map[string]*Service{"web": {}, "api": {}, "db": {}}}

	got := cfg.ServiceNames()

	want := []string{"api", "db", "web"}
	if !slices.Equal(got, want) {
		t.Errorf("ServiceNames() = %v, want %v", got, want)
	}
}

// fieldErrorPaths extracts the Path of every FieldError inside err.
func fieldErrorPaths(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}

	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("expected joined errors, got %T: %v", err, err)
	}

	var paths []string
	for _, e := range joined.Unwrap() {
		var fe *FieldError
		if !errors.As(e, &fe) {
			t.Fatalf("expected *FieldError, got %T: %v", e, e)
		}
		paths = append(paths, fe.Path)
	}
	return paths
}
