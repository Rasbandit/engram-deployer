// Command engram-deployer is the pull-based deploy daemon for Engram on
// FastRaid. See README.md and internal/server/doc.go for the full picture.
//
// Config is loaded from environment variables (set by the Unraid plugin's
// rc.d wrapper). Real Deployer wiring lives in internal/deploy and is
// composed in Step 4 — for now this builds and exits with "deployer not
// configured" so the package compiles end-to-end.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Rasbandit/engram-deployer/internal/auth"
	"github.com/Rasbandit/engram-deployer/internal/server"
)

type config struct {
	Addr        string
	CertFile    string
	KeyFile     string
	JWKSURL     string
	Issuer      string
	Audience    string
	Repository  string
	Ref         string
	WorkflowRef string
	AllowedIPs  []string
}

func loadConfig() (config, error) {
	cfg := config{
		Addr:        envOr("DEPLOYER_ADDR", ":8443"),
		CertFile:    os.Getenv("DEPLOYER_CERT_FILE"),
		KeyFile:     os.Getenv("DEPLOYER_KEY_FILE"),
		JWKSURL:     envOr("DEPLOYER_JWKS_URL", "https://token.actions.githubusercontent.com/.well-known/jwks"),
		Issuer:      envOr("DEPLOYER_ISSUER", "https://token.actions.githubusercontent.com"),
		Audience:    envOr("DEPLOYER_AUDIENCE", "engram-deploy"),
		Repository:  os.Getenv("DEPLOYER_REPOSITORY"),
		Ref:         envOr("DEPLOYER_REF", "refs/heads/main"),
		WorkflowRef: os.Getenv("DEPLOYER_WORKFLOW_REF"),
		AllowedIPs:  splitCSV(os.Getenv("DEPLOYER_ALLOWED_IPS")),
	}
	var missing []string
	if cfg.CertFile == "" {
		missing = append(missing, "DEPLOYER_CERT_FILE")
	}
	if cfg.KeyFile == "" {
		missing = append(missing, "DEPLOYER_KEY_FILE")
	}
	if cfg.Repository == "" {
		missing = append(missing, "DEPLOYER_REPOSITORY")
	}
	if cfg.WorkflowRef == "" {
		missing = append(missing, "DEPLOYER_WORKFLOW_REF")
	}
	if len(cfg.AllowedIPs) == 0 {
		missing = append(missing, "DEPLOYER_ALLOWED_IPS")
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing required env: %v", missing)
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	validator, err := auth.NewValidator(ctx, auth.OIDCConfig{
		JWKSURL:     cfg.JWKSURL,
		Issuer:      cfg.Issuer,
		Audience:    cfg.Audience,
		Repository:  cfg.Repository,
		Ref:         cfg.Ref,
		WorkflowRef: cfg.WorkflowRef,
	})
	if err != nil {
		log.Fatalf("init OIDC validator: %v", err)
	}

	ipAllow, err := auth.NewIPAllowlist(cfg.AllowedIPs)
	if err != nil {
		log.Fatalf("init IP allowlist: %v", err)
	}

	srv := server.New(server.Config{
		Validator: validator,
		JTI:       auth.NewJTISet(1000, 30*time.Minute),
		IPAllow:   ipAllow,
		Deployer:  notWiredDeployer{},
	})

	log.Printf("engram-deployer listening on %s (Step 4 will wire real Deployer)", cfg.Addr)
	if err := srv.ListenAndServeTLS(ctx, cfg.Addr, cfg.CertFile, cfg.KeyFile); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// notWiredDeployer is a placeholder until Step 4 lands the real impl.
// Any /deploy request fails with a clear "not configured" message.
type notWiredDeployer struct{}

func (notWiredDeployer) Run(_ context.Context, _ string, events chan<- server.DeployEvent) error {
	close(events)
	return errors.New("deploy logic not yet wired (Step 4 pending)")
}
