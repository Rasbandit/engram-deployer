package auth

import "testing"

func TestIPAllowlist_AllowedIP(t *testing.T) {
	a, err := NewIPAllowlist([]string{"10.20.99.10"})
	if err != nil {
		t.Fatalf("constructor error: %v", err)
	}

	if !a.Allowed("10.20.99.10:54321") {
		t.Fatal("10.20.99.10 must be allowed")
	}
	if a.Allowed("10.20.99.11:54321") {
		t.Fatal("10.20.99.11 must be denied")
	}
}

func TestIPAllowlist_IPv6(t *testing.T) {
	a, err := NewIPAllowlist([]string{"::1"})
	if err != nil {
		t.Fatalf("constructor error: %v", err)
	}
	// net/http supplies IPv6 RemoteAddr as "[::1]:port"
	if !a.Allowed("[::1]:54321") {
		t.Fatal("::1 must be allowed when configured")
	}
}

func TestIPAllowlist_FailsClosedOnGarbage(t *testing.T) {
	a, err := NewIPAllowlist([]string{"10.20.99.10"})
	if err != nil {
		t.Fatalf("constructor error: %v", err)
	}

	cases := []string{
		"",
		"not an address",
		"10.20.99.10",        // no port
		"10.20.99.10:",       // trailing colon
		":54321",             // no host
		"999.999.999.999:80", // bogus IP
	}
	for _, c := range cases {
		if a.Allowed(c) {
			t.Fatalf("%q must be denied (fail-closed)", c)
		}
	}
}

func TestIPAllowlist_RejectsMalformedConfig(t *testing.T) {
	if _, err := NewIPAllowlist([]string{"not an ip"}); err == nil {
		t.Fatal("constructor must reject malformed IP in config")
	}
}
