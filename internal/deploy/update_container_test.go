package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeScript drops a bash script with the given body into a tmp file
// and returns its path. The test framework removes it on completion.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "update_container")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n"+body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestUpdateContainer_PassesNameAndReturnsNilOnSuccess(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "args.log")
	script := writeScript(t, `echo "$@" > `+logPath+`
exit 0
`)

	uc := NewUpdateContainer(script)
	if err := uc.Run(context.Background(), "engram-saas"); err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}

	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	if strings.TrimSpace(string(got)) != "engram-saas" {
		t.Errorf("script received args %q, want %q", got, "engram-saas")
	}
}

func TestUpdateContainer_SurfacesNonZeroExit(t *testing.T) {
	script := writeScript(t, `echo "stderr message" >&2; exit 7`)
	uc := NewUpdateContainer(script)

	err := uc.Run(context.Background(), "engram-saas")
	if err == nil {
		t.Fatal("expected error on non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "exit") {
		t.Errorf("error doesn't mention exit code: %v", err)
	}
	if !strings.Contains(err.Error(), "stderr message") {
		t.Errorf("error doesn't include captured stderr: %v", err)
	}
}

func TestUpdateContainer_MissingScriptIsAnError(t *testing.T) {
	uc := NewUpdateContainer("/nonexistent/update_container")
	if err := uc.Run(context.Background(), "engram-saas"); err == nil {
		t.Fatal("expected error for missing script path, got nil")
	}
}

func TestUpdateContainer_RespectsContextCancel(t *testing.T) {
	script := writeScript(t, `sleep 10`)
	uc := NewUpdateContainer(script)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := uc.Run(ctx, "engram-saas")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from cancelled ctx, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run took %v after 100ms timeout — ctx cancel not honored", elapsed)
	}
}
