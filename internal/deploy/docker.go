package deploy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Docker shells out to the host's docker CLI. We use the CLI rather than
// the Docker Go SDK because (a) the SDK pulls in a heavy dep tree and
// (b) the existing fastraid-deploy.sh already uses the CLI, so behavior
// stays identical post-port.
type Docker struct {
	// Path is the docker binary. Empty defaults to "docker" on $PATH.
	Path string
}

func (d *Docker) bin() string {
	if d.Path != "" {
		return d.Path
	}
	return "docker"
}

// Pull runs `docker pull image:version`, copying stdout to progress as it
// streams (one write per docker output line — caller can layer a
// line-buffered translator on top). Non-zero exit returns an error
// including captured stderr.
//
// progress may be nil to discard output.
func (d *Docker) Pull(ctx context.Context, image, version string, progress io.Writer) error {
	if progress == nil {
		progress = io.Discard
	}
	cmd := exec.CommandContext(ctx, d.bin(), "pull", image+":"+version)
	cmd.Stdout = progress
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	groupKill(cmd)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker pull %s:%s: %w; stderr: %s",
			image, version, err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}

// Tag runs `docker tag source target`. Idempotent on Docker's side.
func (d *Docker) Tag(ctx context.Context, source, target string) error {
	cmd := exec.CommandContext(ctx, d.bin(), "tag", source, target)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	groupKill(cmd)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker tag %s %s: %w; stderr: %s",
			source, target, err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}
