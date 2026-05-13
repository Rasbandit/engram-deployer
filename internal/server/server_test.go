package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// /healthz is unauthenticated and returns 200 with a tiny body.
// Used by infrastructure liveness probes; intentionally outside the auth chain.
func TestHealthz(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); body != "ok\n" {
		t.Fatalf("body = %q, want %q", body, "ok\n")
	}
}

// /deploy with no Authorization header must reject before any deploy work.
func TestDeploy_RejectsMissingAuth(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("POST", "/deploy", strings.NewReader(`{"version":"1.0.0"}`))
	req.RemoteAddr = "127.0.0.1:12345" // pass IP allowlist; auth gate is what's under test
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// Authorization without a Bearer prefix (or malformed) is also 401.
func TestDeploy_RejectsMalformedAuthHeader(t *testing.T) {
	s := newTestServer(t)

	cases := []string{
		"Basic dXNlcjpwYXNz", // wrong scheme
		"Bearer",             // no token
		"Bearer ",            // empty token
		"bearer abc.def.ghi", // lowercase scheme
		"abc.def.ghi",        // no scheme
	}
	for _, h := range cases {
		req := httptest.NewRequest("POST", "/deploy", strings.NewReader(`{"version":"1.0.0"}`))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Authorization", h)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Authorization=%q got %d, want 401", h, w.Code)
		}
	}
}

// Valid token, allowed IP, healthy deployer:
//   - server forwards version to deployer
//   - all scripted events stream to the caller as NDJSON
//   - terminal line is a DeployResult with status="ok"
//   - /status afterwards returns the same result
//   - replay of the same jti is refused
func TestDeploy_HappyPath(t *testing.T) {
	fake := &fakeDeployer{
		scriptEvts: []DeployEvent{
			{Phase: "pull", Message: "pulling 0.5.61", Time: time.Now()},
			{Phase: "update", Message: "updating engram-saas", Time: time.Now()},
			{Phase: "health", Message: "engram-saas healthy", Time: time.Now()},
		},
	}
	s := newTestServer(t, withDeployer(fake))

	body := strings.NewReader(`{"version":"0.5.61","sha":"abc1234"}`)
	req := httptest.NewRequest("POST", "/deploy", body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer "+mintValidToken(t, "jti-happy"))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/x-ndjson" {
		t.Errorf("Content-Type = %q, want application/x-ndjson", got)
	}

	// Parse NDJSON. First N lines are DeployEvent, final is DeployResult.
	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	wantEventLines := len(fake.scriptEvts)
	if len(lines) != wantEventLines+1 {
		t.Fatalf("got %d response lines, want %d events + 1 result. body=%s",
			len(lines), wantEventLines, w.Body.String())
	}

	for i, line := range lines[:wantEventLines] {
		var ev DeployEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d not valid DeployEvent: %v (line=%s)", i, err, line)
		}
		if ev.Phase != fake.scriptEvts[i].Phase {
			t.Errorf("line %d phase = %q, want %q", i, ev.Phase, fake.scriptEvts[i].Phase)
		}
	}

	var result DeployResult
	if err := json.Unmarshal([]byte(lines[wantEventLines]), &result); err != nil {
		t.Fatalf("terminal line not valid DeployResult: %v (line=%s)", err, lines[wantEventLines])
	}
	if result.Status != "ok" {
		t.Errorf("result.Status = %q, want %q (error=%q)", result.Status, "ok", result.Error)
	}
	if result.Version != "0.5.61" {
		t.Errorf("result.Version = %q, want %q", result.Version, "0.5.61")
	}

	if got := fake.CalledWith(); got != "0.5.61" {
		t.Errorf("deployer called with %q, want %q", got, "0.5.61")
	}

	// /status returns the same result.
	statusReq := httptest.NewRequest("GET", "/status", nil)
	statusW := httptest.NewRecorder()
	s.Handler().ServeHTTP(statusW, statusReq)
	if statusW.Code != 200 {
		t.Fatalf("/status = %d, want 200", statusW.Code)
	}
	var statusResult DeployResult
	if err := json.Unmarshal(statusW.Body.Bytes(), &statusResult); err != nil {
		t.Fatalf("/status body not DeployResult: %v", err)
	}
	if statusResult.Version != "0.5.61" || statusResult.Status != "ok" {
		t.Errorf("/status mismatch: %+v", statusResult)
	}

	// Replay the same jti — must be refused.
	body2 := strings.NewReader(`{"version":"0.5.61","sha":"abc1234"}`)
	req2 := httptest.NewRequest("POST", "/deploy", body2)
	req2.RemoteAddr = "127.0.0.1:12345"
	req2.Header.Set("Authorization", "Bearer "+req.Header.Get("Authorization")[7:]) // same token
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, want 401", w2.Code)
	}
}

// Failing deployer surfaces in the terminal DeployResult with status="fail"
// and the error message — HTTP code is still 200 because the response is
// streamed (status committed at first byte).
func TestDeploy_DeployerFailure(t *testing.T) {
	fake := &fakeDeployer{
		scriptEvts: []DeployEvent{
			{Phase: "pull", Message: "pulling 0.5.61", Time: time.Now()},
		},
		scriptErr: errString("docker pull: connection refused"),
	}
	s := newTestServer(t, withDeployer(fake))

	body := strings.NewReader(`{"version":"0.5.61"}`)
	req := httptest.NewRequest("POST", "/deploy", body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer "+mintValidToken(t, "jti-failpath"))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (failures are body-side)", w.Code)
	}

	// Last non-empty line is the result.
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	var result DeployResult
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &result); err != nil {
		t.Fatalf("terminal line not DeployResult: %v", err)
	}
	if result.Status != "fail" {
		t.Errorf("result.Status = %q, want %q", result.Status, "fail")
	}
	if !strings.Contains(result.Error, "connection refused") {
		t.Errorf("result.Error = %q, want substring 'connection refused'", result.Error)
	}
}

// Body version that's not strict X.Y.Z must be refused with 400 before
// any deploy work happens.
func TestDeploy_RejectsBadVersion(t *testing.T) {
	s := newTestServer(t)

	cases := []string{
		`{"version":""}`,
		`{"version":"1.2"}`,
		`{"version":"v1.2.3"}`,
		`{"version":"1.2.3-rc1"}`,
		`{"version":"1.2.3; rm -rf /"}`,
		`{}`,
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/deploy", strings.NewReader(c))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Authorization", "Bearer "+mintValidToken(t, "jti-badver-"+c))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body=%s got %d, want 400", c, w.Code)
		}
	}
}

// IP not in the allowlist is refused with 403 before any auth happens.
func TestDeploy_RejectsNonAllowlistedIP(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("POST", "/deploy", strings.NewReader(`{"version":"1.0.0"}`))
	req.RemoteAddr = "203.0.113.5:12345" // TEST-NET-3, not in allowlist
	req.Header.Set("Authorization", "Bearer "+mintValidToken(t, "jti-ip-test"))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}
