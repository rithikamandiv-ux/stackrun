package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rithikamandiv-ux/stackrun/internal/config"
)

func runValidate(t *testing.T, content string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stackrun.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"validate", "--config", path})

	err := cmd.Execute()
	return out.String(), err
}

func TestValidateCommandValidConfig(t *testing.T) {
	out, err := runValidate(t, "services:\n  web:\n    command: npm run dev\n  api:\n    command: go run .\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Config is valid: 2 services (api, web)\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestValidateCommandInvalidConfig(t *testing.T) {
	_, err := runValidate(t, "services:\n  api:\n    restart: never\n")

	var fe *config.FieldError
	if !errors.As(err, &fe) {
		t.Fatalf("error = %v, want a *config.FieldError", err)
	}
}
