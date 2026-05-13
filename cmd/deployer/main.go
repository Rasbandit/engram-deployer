// Command engram-deployer is the pull-based deploy daemon for Engram on FastRaid.
//
// Listens on :8443 over TLS, authenticates incoming requests via GitHub Actions
// OIDC tokens, and executes the Engram deploy workflow (docker pull + Unraid
// template pin + update_container + health check) on receipt of a valid
// /deploy request.
//
// Runs as a host service installed via an Unraid plugin (.plg). Not designed
// to run inside a container — needs host access to Unraid's update_container
// script and /boot/config/plugins/dockerMan/templates-user/.
package main

import (
	"log"
)

func main() {
	log.Println("engram-deployer: bootstrapping (scaffold — no logic yet)")
}
