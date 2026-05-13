package deploy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Rasbandit/engram-deployer/internal/server"
)

// ContainerSpec describes one of the engram containers managed on FastRaid.
type ContainerSpec struct {
	Name string // e.g. "engram-saas" — also names the Unraid template my-<Name>.xml
	Port int    // localhost port for /api/health probing
}

// Puller, Updater, Health are narrow interfaces over the executors that
// keep the orchestrator unit-testable without docker, the Unraid host, or
// HTTP.
type Puller interface {
	Pull(ctx context.Context, image, version string, progress io.Writer) error
	Tag(ctx context.Context, source, target string) error
}

type Updater interface {
	Run(ctx context.Context, name string) error
}

type Health interface {
	Wait(ctx context.Context, url, expectedVersion string) error
}

// Orchestrator implements server.Deployer. It runs the deploy phases that
// previously lived in fastraid-deploy.sh: pull, tag, then per-container
// (template-rewrite, update_container, health-wait), in order, fail-fast.
type Orchestrator struct {
	Image       string          // e.g. "ghcr.io/rasbandit/engram"
	TemplateDir string          // e.g. "/boot/config/plugins/dockerMan/templates-user"
	Containers  []ContainerSpec // order matters: SaaS before selfhost
	Puller      Puller
	Updater     Updater
	Health      Health
}

func (o *Orchestrator) Run(ctx context.Context, version string, events chan<- server.DeployEvent) error {
	// MUST close events on every return path (server expects sender-closes).
	defer close(events)

	if err := o.pullAndTag(ctx, version, events); err != nil {
		return err
	}

	for _, c := range o.Containers {
		if err := o.deployOne(ctx, c, version, events); err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
	}
	return nil
}

func (o *Orchestrator) pullAndTag(ctx context.Context, version string, events chan<- server.DeployEvent) error {
	emit(events, "pull", fmt.Sprintf("docker pull %s:%s", o.Image, version))

	var pullOut bytes.Buffer
	if err := o.Puller.Pull(ctx, o.Image, version, &pullOut); err != nil {
		return fmt.Errorf("docker pull: %w", err)
	}

	emit(events, "tag", fmt.Sprintf("docker tag %s:%s %s:latest", o.Image, version, o.Image))
	versionRef := fmt.Sprintf("%s:%s", o.Image, version)
	latestRef := fmt.Sprintf("%s:latest", o.Image)
	if err := o.Puller.Tag(ctx, versionRef, latestRef); err != nil {
		return fmt.Errorf("docker tag: %w", err)
	}
	return nil
}

func (o *Orchestrator) deployOne(ctx context.Context, c ContainerSpec, version string, events chan<- server.DeployEvent) error {
	templatePath := filepath.Join(o.TemplateDir, "my-"+c.Name+".xml")

	emit(events, "template", fmt.Sprintf("%s: pinning %s to %s", c.Name, templatePath, version))
	raw, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read template %s: %w", templatePath, err)
	}
	rewritten, err := ReplaceRepoTag(raw, o.Image, version)
	if err != nil {
		return fmt.Errorf("rewrite template %s: %w", templatePath, err)
	}
	if err := os.WriteFile(templatePath, rewritten, 0o644); err != nil {
		return fmt.Errorf("write template %s: %w", templatePath, err)
	}

	emit(events, "update", fmt.Sprintf("%s: update_container", c.Name))
	if err := o.Updater.Run(ctx, c.Name); err != nil {
		return fmt.Errorf("update_container: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d/api/health", c.Port)
	emit(events, "health", fmt.Sprintf("%s: waiting for %s to report %s", c.Name, url, version))
	if err := o.Health.Wait(ctx, url, version); err != nil {
		return fmt.Errorf("health: %w", err)
	}
	return nil
}

func emit(events chan<- server.DeployEvent, phase, msg string) {
	events <- server.DeployEvent{Phase: phase, Message: msg, Time: time.Now()}
}
