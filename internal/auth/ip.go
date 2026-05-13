package auth

import (
	"fmt"
	"net"
)

// IPAllowlist denies any request whose source IP is not in the configured set.
// Defense-in-depth alongside the host firewall — daemon also enforces.
type IPAllowlist struct {
	allowed map[string]struct{}
}

func NewIPAllowlist(addrs []string) (*IPAllowlist, error) {
	set := make(map[string]struct{}, len(addrs))
	for _, raw := range addrs {
		ip := net.ParseIP(raw)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP in allowlist: %q", raw)
		}
		set[ip.String()] = struct{}{}
	}
	return &IPAllowlist{allowed: set}, nil
}

// Allowed reports whether remoteAddr (as supplied by net/http via
// r.RemoteAddr — typically "host:port") is permitted.
//
// Fail-closed: any parse error → denied.
func (a *IPAllowlist) Allowed(remoteAddr string) bool {
	host, port, err := net.SplitHostPort(remoteAddr)
	if err != nil || port == "" {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	_, ok := a.allowed[ip.String()]
	return ok
}
