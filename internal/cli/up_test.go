package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runUp(t *testing.T, content string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stackrun.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"up", "--config", path})

	err := cmd.Execute()
	return out.String(), err
}

func TestUpStreamsOutput(t *testing.T) {
	out, err := runUp(t, "services:\n  api:\n    command: echo hello\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"stackrun | starting 1 service\n",
		"api      | hello\n",
		"stackrun | api exited with code 0\n",
		"stackrun | all services exited\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestUpReturnsErrorWhenServiceFails(t *testing.T) {
	_, err := runUp(t, "services:\n  api:\n    command: exit 2\n")

	if err == nil {
		t.Fatal("expected an error when a service exits with a non-zero code")
	}
}
