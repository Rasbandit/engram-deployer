package deploy

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// UpdateContainer wraps Unraid's host-only `update_container` script —
// the only piece of the deploy flow we can't replace with Docker API
// calls, because it's a PHP-aware wrapper that keeps Unraid GUI state
// in sync with the underlying container.
type UpdateContainer struct {
	scriptPath string
}

// DefaultUpdateContainerPath is where Unraid installs the script.
const DefaultUpdateContainerPath = "/usr/local/emhttp/plugins/dynamix.docker.manager/scripts/update_container"

// NewUpdateContainer wraps scriptPath (typically DefaultUpdateContainerPath).
func NewUpdateContainer(scriptPath string) *UpdateContainer {
	return &UpdateContainer{scriptPath: scriptPath}
}

// Run invokes `<scriptPath> <name>`. The script is expected to be
// idempotent on Unraid: it stops, removes, and re-creates the container
// from its template XML.
//
// Non-zero exit codes surface as errors with the captured stderr included
// (Unraid emits useful diagnostics to stderr on failure).
func (u *UpdateContainer) Run(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, u.scriptPath, name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	groupKill(cmd)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("update_container %s: %w; stderr: %s",
			name, err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}
