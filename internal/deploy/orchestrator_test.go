package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/engram-app/engram-deployer/internal/server"
)

type fakePuller struct {
	pullArgs   []string
	tagArgs    []string
	pullErr    error
	tagErr     error
	progressOn []string
}

func (f *fakePuller) Pull(_ context.Context, image, version string, progress io.Writer) error {
	f.pullArgs = append(f.pullArgs, image+":"+version)
	for _, line := range f.progressOn {
		_, _ = fmt.Fprintln(progress, line)
	}
	return f.pullErr
}

func (f *fakePuller) Tag(_ context.Context, source, target string) error {
	f.tagArgs = append(f.tagArgs, source+" "+target)
	return f.tagErr
}

type fakeUpdater struct {
	calls []string
	err   error
}

func (f *fakeUpdater) Run(_ context.Context, name string) error {
	f.calls = append(f.calls, name)
	return f.err
}

type fakeHealth struct {
	waits []string
	err   error
}

func (f *fakeHealth) Wait(_ context.Context, url, version string) error {
	f.waits = append(f.waits, url+"@"+version)
	return f.err
}

// writeTemplate drops a minimal Unraid container template into the dir.
func writeTemplate(t *testing.T, dir, name, image, version string) string {
	t.Helper()
	path := filepath.Join(dir, "my-"+name+".xml")
	body := fmt.Sprintf(`<?xml version="1.0"?>
<Container>
  <Name>%s</Name>
  <Repository>%s:%s</Repository>
</Container>
`, name, image, version)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	return path
}

func newOrchestratorTestHarness(t *testing.T) (*Orchestrator, *fakePuller, *fakeUpdater, *fakeHealth, string) {
	t.Helper()
	dir := t.TempDir()
	writeTemplate(t, dir, "engram-saas", "ghcr.io/engram-app/engram", "0.5.60")
	writeTemplate(t, dir, "engram-selfhost", "ghcr.io/engram-app/engram", "0.5.60")

	puller := &fakePuller{}
	updater := &fakeUpdater{}
	health := &fakeHealth{}

	o := &Orchestrator{
		Image:       "ghcr.io/engram-app/engram",
		TemplateDir: dir,
		Containers: []ContainerSpec{
			{Name: "engram-saas", Port: 8000},
			{Name: "engram-selfhost", Port: 8001},
		},
		Puller:  puller,
		Updater: updater,
		Health:  health,
	}
	return o, puller, updater, health, dir
}

// drain collects all events emitted into a slice; blocks until close.
func drain(events <-chan server.DeployEvent) []server.DeployEvent {
	var out []server.DeployEvent
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func TestOrchestrator_HappyPath(t *testing.T) {
	o, puller, updater, health, dir := newOrchestratorTestHarness(t)

	events := make(chan server.DeployEvent, 32)
	doneEvents := make(chan []server.DeployEvent, 1)
	go func() { doneEvents <- drain(events) }()

	err := o.Run(context.Background(), "0.5.61", events)
	if err != nil {
		t.Fatalf("Run returned %v, want nil", err)
	}
	emitted := <-doneEvents

	// Exactly one pull, one tag.
	if want := []string{"ghcr.io/engram-app/engram:0.5.61"}; !equalSlices(puller.pullArgs, want) {
		t.Errorf("pull args = %v, want %v", puller.pullArgs, want)
	}
	if want := []string{"ghcr.io/engram-app/engram:0.5.61 ghcr.io/engram-app/engram:latest"}; !equalSlices(puller.tagArgs, want) {
		t.Errorf("tag args = %v, want %v", puller.tagArgs, want)
	}

	// SaaS updated then selfhost — order matters.
	if want := []string{"engram-saas", "engram-selfhost"}; !equalSlices(updater.calls, want) {
		t.Errorf("updater calls = %v, want %v", updater.calls, want)
	}

	// Health waited for each at the right port.
	wantHealth := []string{
		"http://localhost:8000/api/health@0.5.61",
		"http://localhost:8001/api/health@0.5.61",
	}
	if !equalSlices(health.waits, wantHealth) {
		t.Errorf("health waits = %v, want %v", health.waits, wantHealth)
	}

	// Template files were rewritten.
	for _, name := range []string{"engram-saas", "engram-selfhost"} {
		body, _ := os.ReadFile(filepath.Join(dir, "my-"+name+".xml"))
		if !strings.Contains(string(body), "<Repository>ghcr.io/engram-app/engram:0.5.61</Repository>") {
			t.Errorf("template %s not rewritten:\n%s", name, body)
		}
	}

	// Events stream covers each phase.
	wantPhases := []string{"pull", "tag", "template", "update", "health", "template", "update", "health"}
	if got := phaseList(emitted); !equalSlices(got, wantPhases) {
		t.Errorf("event phases = %v, want %v", got, wantPhases)
	}
}

func TestOrchestrator_StopsOnPullFailure(t *testing.T) {
	o, puller, updater, _, _ := newOrchestratorTestHarness(t)
	puller.pullErr = errors.New("ghcr unreachable")

	events := make(chan server.DeployEvent, 32)
	doneEvents := make(chan []server.DeployEvent, 1)
	go func() { doneEvents <- drain(events) }()

	err := o.Run(context.Background(), "0.5.61", events)
	<-doneEvents

	if err == nil || !strings.Contains(err.Error(), "ghcr unreachable") {
		t.Fatalf("expected pull error to bubble up, got: %v", err)
	}
	if len(updater.calls) != 0 {
		t.Errorf("updater should not be called after pull failure, got: %v", updater.calls)
	}
}

func TestOrchestrator_StopsBeforeSelfhostIfSaasUpdateFails(t *testing.T) {
	o, _, updater, health, _ := newOrchestratorTestHarness(t)
	updater.err = errors.New("update_container saas exit 1")

	events := make(chan server.DeployEvent, 32)
	doneEvents := make(chan []server.DeployEvent, 1)
	go func() { doneEvents <- drain(events) }()

	err := o.Run(context.Background(), "0.5.61", events)
	<-doneEvents

	if err == nil {
		t.Fatal("expected update_container failure to propagate")
	}
	// Updater is called once (for saas), never reaches selfhost.
	if want := []string{"engram-saas"}; !equalSlices(updater.calls, want) {
		t.Errorf("updater calls = %v, want %v (selfhost must be skipped)", updater.calls, want)
	}
	if len(health.waits) != 0 {
		t.Errorf("health check should not run after update failure, got: %v", health.waits)
	}
}

func TestOrchestrator_StopsBeforeSelfhostIfSaasHealthFails(t *testing.T) {
	o, _, updater, health, _ := newOrchestratorTestHarness(t)
	health.err = errors.New("health check timeout")

	events := make(chan server.DeployEvent, 32)
	doneEvents := make(chan []server.DeployEvent, 1)
	go func() { doneEvents <- drain(events) }()

	err := o.Run(context.Background(), "0.5.61", events)
	<-doneEvents

	if err == nil {
		t.Fatal("expected health timeout to propagate")
	}
	// SaaS gets updated, selfhost never does.
	if want := []string{"engram-saas"}; !equalSlices(updater.calls, want) {
		t.Errorf("updater calls = %v, want %v (selfhost must be skipped)", updater.calls, want)
	}
}

func TestOrchestrator_FailsIfTemplateMissing(t *testing.T) {
	o, _, _, _, dir := newOrchestratorTestHarness(t)
	_ = os.Remove(filepath.Join(dir, "my-engram-saas.xml"))

	events := make(chan server.DeployEvent, 32)
	doneEvents := make(chan []server.DeployEvent, 1)
	go func() { doneEvents <- drain(events) }()

	err := o.Run(context.Background(), "0.5.61", events)
	<-doneEvents

	if err == nil {
		t.Fatal("expected error for missing template")
	}
}

// equalSlices is a tiny helper to compare []string in tests.
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func phaseList(evs []server.DeployEvent) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.Phase
	}
	return out
}

// Sanity: orchestrator implements server.Deployer.
var _ server.Deployer = (*Orchestrator)(nil)
