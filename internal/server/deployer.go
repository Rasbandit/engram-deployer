package server

import (
	"context"
	"time"
)

// DeployEvent is a single progress message streamed back to the caller
// while a deploy runs. Phase is one of a small fixed set (pull, tag,
// update, health, ...); Message is free-form human-readable detail.
type DeployEvent struct {
	Phase   string    `json:"phase"`
	Message string    `json:"message,omitempty"`
	Time    time.Time `json:"time"`
}

// DeployResult is the terminal record of a deploy attempt. Streamed as the
// final line of the response body so callers can grep tail for success.
type DeployResult struct {
	Status     string    `json:"status"` // "ok" | "fail"
	Error      string    `json:"error,omitempty"`
	Version    string    `json:"version"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMs int64     `json:"duration_ms"`
}

// Deployer executes the actual deploy workflow. Implemented by the
// internal/deploy package in Step 4; the server depends only on this
// interface so it can be tested with fakes.
//
// Run MUST close events when finished. The returned error becomes the
// "fail" reason in the terminal DeployResult.
type Deployer interface {
	Run(ctx context.Context, version string, events chan<- DeployEvent) error
}
