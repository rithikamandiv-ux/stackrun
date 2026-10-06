package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeConfig writes content to stackrun.yaml inside dir and returns its path.
func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "stackrun.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeConfig(t, dir, `
services:
  backend:
    command: npm run dev
    dir: ./server
    env:
      PORT: "4000"
    restart: on-failure
    stop_timeout: 30s
  worker:
    command: python worker.py
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	backend := cfg.Services["backend"]
	if backend.Name != "backend" {
		t.Errorf("Name = %q, want %q", backend.Name, "backend")
	}
	if want := filepath.Join(dir, "server"); backend.Dir != want {
		t.Errorf("Dir = %q, want %q", backend.Dir, want)
	}
	if backend.Restart != RestartOnFailure {
		t.Errorf("Restart = %q, want %q", backend.Restart, RestartOnFailure)
	}
	if backend.Env["PORT"] != "4000" {
		t.Errorf("Env[PORT] = %q, want %q", backend.Env["PORT"], "4000")
	}
	if backend.StopTimeout != 30*time.Second {
		t.Errorf("StopTimeout = %v, want 30s", backend.StopTimeout)
	}

	worker := cfg.Services["worker"]
	if worker.Dir != dir {
		t.Errorf("default Dir = %q, want the config folder %q", worker.Dir, dir)
	}
	if worker.Restart != RestartNever {
		t.Errorf("default Restart = %q, want %q", worker.Restart, RestartNever)
	}
	if worker.StopTimeout != DefaultStopTimeout {
		t.Errorf("default StopTimeout = %v, want %v", worker.StopTimeout, DefaultStopTimeout)
	}
}

// This test documents how the YAML library treats unquoted values
// such as numbers and booleans when the Go field is a string.
func TestLoadUnquotedScalarsBecomeStrings(t *testing.T) {
	path := writeConfig(t, t.TempDir(), `
services:
  backend:
    command: node server.js
    env:
      PORT: 4000
      DEBUG: true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	env := cfg.Services["backend"].Env
	if env["PORT"] != "4000" {
		t.Errorf("PORT = %q, want %q", env["PORT"], "4000")
	}
	if env["DEBUG"] != "true" {
		t.Errorf("DEBUG = %q, want %q", env["DEBUG"], "true")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))

	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error = %v, want one wrapping fs.ErrNotExist", err)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantContains  string // text the error message must contain
		wantFieldPath string // if set, a FieldError with this path must exist
	}{
		{
			name:         "empty file",
			content:      "",
			wantContains: "file is empty",
		},
		{
			name:         "invalid YAML syntax",
			content:      "services: [unclosed",
			wantContains: "parsing",
		},
		{
			name: "unknown field",
			content: `
services:
  backend:
    comand: npm run dev
`,
			wantContains: "comand",
		},
		{
			name: "stop_timeout without a unit",
			content: `
services:
  backend:
    command: npm run dev
    stop_timeout: 30
`,
			wantContains: "parsing",
		},
		{
			name: "validation error",
			content: `
services:
  backend:
    dir: .
`,
			wantFieldPath: "services.backend.command",
		},
		{
			name: "empty service does not crash",
			content: `
services:
  worker:
`,
			wantFieldPath: "services.worker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, t.TempDir(), tt.content)

			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() returned nil error, want an error")
			}

			if tt.wantContains != "" && !strings.Contains(err.Error(), tt.wantContains) {
				t.Errorf("error %q does not contain %q", err, tt.wantContains)
			}

			if tt.wantFieldPath != "" {
				var fe *FieldError
				if !errors.As(err, &fe) || fe.Path != tt.wantFieldPath {
					t.Errorf("error = %v, want a FieldError with path %q", err, tt.wantFieldPath)
				}
			}
		})
	}
}
