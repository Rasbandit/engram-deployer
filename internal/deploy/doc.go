// Package deploy executes the Engram deploy workflow against the local
// Unraid host. Pure Go replacement for the previous fastraid-deploy.sh.
//
// Sequence per deploy:
//
//  1. docker pull ghcr.io/engram-app/engram:<version>
//  2. docker tag <version> latest
//  3. For each container in order (engram-saas, then engram-selfhost):
//     a. sed Unraid template XML to pin <Repository> tag
//     b. exec /usr/local/emhttp/.../scripts/update_container <name>
//     c. poll http://localhost:<port>/api/health for version match
//
// Sequential ordering is intentional — SaaS deploys first so a broken
// image fails before selfhost (production-facing) is touched.
package deploy
