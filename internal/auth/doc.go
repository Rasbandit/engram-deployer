// Package auth validates incoming /deploy requests.
//
// Three independent gates, all must pass:
//
//  1. OIDC — JWT issued by GitHub Actions for the calling workflow run.
//     Signature verified against GitHub's JWKS. Claims allowlisted:
//     aud, repository, ref, workflow_ref, iat (5min skew).
//  2. JTI replay — each token's jti claim is recorded for 30min; second
//     sighting is refused. Kills replay even if a token is captured.
//  3. Source IP — only the runner VM's IP is accepted at the daemon layer
//     (defense-in-depth alongside the host firewall).
package auth
