package proc

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows support arrives in a later phase")
	}
}

func TestMergeEnv(t *testing.T) {
	base := []string{"HOME=/home/me", "PORT=3000", "PATH=/usr/bin"}

	got := mergeEnv(base, map[string]string{"PORT": "4000", "DEBUG": "1"})

	want := []string{"HOME=/home/me", "PATH=/usr/bin", "DEBUG=1", "PORT=4000"}
	if !slices.Equal(got, want) {
		t.Errorf("mergeEnv() = %v, want %v", got, want)
	}
}

func TestCommandSetsDirAndEnv(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()

	cmd, err := Command(Spec{Command: "true", Dir: dir, Env: map[string]string{"GREETING": "hello"}})
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	if cmd.Dir != dir {
		t.Errorf("Dir = %q, want %q", cmd.Dir, dir)
	}
	for _, want := range []string{"GREETING=hello", "PWD=" + dir} {
		if !slices.Contains(cmd.Env, want) {
			t.Errorf("Env does not contain %q", want)
		}
	}
}

func TestCommandRunsThroughShell(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("found"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd, err := Command(Spec{
		Command: `printf '%s %s' "$GREETING" "$(cat marker.txt)"`,
		Dir:     dir,
		Env:     map[string]string{"GREETING": "hello"},
	})
	if err != nil {
		t.Fatalf("Command() returned error: %v", err)
	}

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running command: %v", err)
	}
	if got, want := string(out), "hello found"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
