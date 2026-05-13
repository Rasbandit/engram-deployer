package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Helper: spin up a test /api/health server whose response can be swapped
// at runtime via the returned setter.
type swappableHealth struct {
	server *httptest.Server
	body   atomic.Value // string
	status atomic.Int32
	calls  atomic.Int32
}

func newSwappableHealth(t *testing.T) *swappableHealth {
	t.Helper()
	sh := &swappableHealth{}
	sh.body.Store(`{}`)
	sh.status.Store(200)
	sh.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sh.calls.Add(1)
		w.WriteHeader(int(sh.status.Load()))
		_, _ = fmt.Fprint(w, sh.body.Load().(string))
	}))
	t.Cleanup(sh.server.Close)
	return sh
}

func (sh *swappableHealth) set(body string, status int) {
	sh.body.Store(body)
	sh.status.Store(int32(status))
}

func (sh *swappableHealth) URL() string { return sh.server.URL + "/api/health" }

func TestHealthChecker_ReturnsWhenVersionMatches(t *testing.T) {
	sh := newSwappableHealth(t)
	sh.set(`{"version":"0.5.61"}`, 200)

	hc := NewHealthChecker(50*time.Millisecond, 2*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := hc.Wait(ctx, sh.URL(), "0.5.61"); err != nil {
		t.Fatalf("Wait returned %v, want nil", err)
	}
}

func TestHealthChecker_KeepsPollingUntilVersionFlips(t *testing.T) {
	sh := newSwappableHealth(t)
	sh.set(`{"version":"0.5.60"}`, 200) // old version initially

	hc := NewHealthChecker(20*time.Millisecond, 2*time.Second)

	// Flip to the new version after 100ms — simulating container restart.
	go func() {
		time.Sleep(100 * time.Millisecond)
		sh.set(`{"version":"0.5.61"}`, 200)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := hc.Wait(ctx, sh.URL(), "0.5.61"); err != nil {
		t.Fatalf("Wait returned %v, want nil", err)
	}
	if calls := sh.calls.Load(); calls < 3 {
		t.Errorf("expected several polls (likely 5-6), got %d", calls)
	}
}

func TestHealthChecker_TimesOutIfNeverMatches(t *testing.T) {
	sh := newSwappableHealth(t)
	sh.set(`{"version":"never-going-to-match"}`, 200)

	hc := NewHealthChecker(20*time.Millisecond, 200*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := hc.Wait(ctx, sh.URL(), "0.5.61")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error doesn't look like a timeout: %v", err)
	}
}

func TestHealthChecker_TolersesTransientServerErrors(t *testing.T) {
	sh := newSwappableHealth(t)
	sh.set(`internal error`, 500)

	hc := NewHealthChecker(20*time.Millisecond, 2*time.Second)

	// Recover to 200 + matching version after 100ms.
	go func() {
		time.Sleep(100 * time.Millisecond)
		sh.set(`{"version":"0.5.61"}`, 200)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := hc.Wait(ctx, sh.URL(), "0.5.61"); err != nil {
		t.Fatalf("Wait returned %v during transient 500s, want nil", err)
	}
}

func TestHealthChecker_RespectsContextCancel(t *testing.T) {
	sh := newSwappableHealth(t)
	sh.set(`{"version":"0.5.60"}`, 200)

	hc := NewHealthChecker(20*time.Millisecond, 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- hc.Wait(ctx, sh.URL(), "0.5.61") }()

	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error after cancel, got nil")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait did not return after ctx cancel")
	}
}
