// Package server hosts the TLS HTTP server and request handlers.
//
// Exposes:
//
//	POST /deploy   — runs the deploy workflow, streams progress events
//	GET  /status   — last deploy summary + daemon uptime
//	GET  /healthz  — liveness probe (unauthenticated)
package server
