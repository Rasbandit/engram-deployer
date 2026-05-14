package deploy

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDockerCLI writes a bash script that records its args and exits with
// the configured code, optionally emitting stdout lines and stderr.
func fakeDockerCLI(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "docker")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n"+body), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	return path
}

func TestDocker_Pull_PassesImageRefAndStreamsProgress(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "args.log")
	cli := fakeDockerCLI(t, `echo "$@" > `+logPath+`
echo "Pulling from engram-app/engram"
echo "Status: Downloaded newer image"
exit 0
`)

	d := &Docker{Path: cli}
	var progress bytes.Buffer
	if err := d.Pull(context.Background(), "ghcr.io/engram-app/engram", "0.5.61", &progress); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	if got := strings.TrimSpace(string(args)); got != "pull ghcr.io/engram-app/engram:0.5.61" {
		t.Errorf("fake docker received %q, want %q", got, "pull ghcr.io/engram-app/engram:0.5.61")
	}
	if !strings.Contains(progress.String(), "Pulling from engram-app/engram") {
		t.Errorf("progress did not include Pull output:\n%s", progress.String())
	}
}

func TestDocker_Pull_SurfacesNonZeroExit(t *testing.T) {
	cli := fakeDockerCLI(t, `echo "unauthorized" >&2; exit 1`)
	d := &Docker{Path: cli}
	err := d.Pull(context.Background(), "ghcr.io/x/y", "1.0.0", nil)
	if err == nil {
		t.Fatal("expected error on non-zero docker exit")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("error missing stderr: %v", err)
	}
}

func TestDocker_Tag_PassesArgsAndReturnsNilOnSuccess(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "args.log")
	cli := fakeDockerCLI(t, `echo "$@" > `+logPath+`
exit 0
`)

	d := &Docker{Path: cli}
	if err := d.Tag(context.Background(), "ghcr.io/x/y:0.5.61", "ghcr.io/x/y:latest"); err != nil {
		t.Fatalf("Tag: %v", err)
	}

	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	want := "tag ghcr.io/x/y:0.5.61 ghcr.io/x/y:latest"
	if got := strings.TrimSpace(string(args)); got != want {
		t.Errorf("fake docker received %q, want %q", got, want)
	}
}
