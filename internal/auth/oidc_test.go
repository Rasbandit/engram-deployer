package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/Rasbandit/engram-deployer/internal/oidctest"
	"github.com/golang-jwt/jwt/v5"
)

// validClaims returns a claim set that should pass every gate, given
// the matching config from validConfig.
func validClaims(jti string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":          "https://token.actions.githubusercontent.com",
		"aud":          "engram-deploy",
		"iat":          now.Unix(),
		"nbf":          now.Unix(),
		"exp":          now.Add(15 * time.Minute).Unix(),
		"jti":          jti,
		"repository":   "Rasbandit/Engram",
		"ref":          "refs/heads/main",
		"workflow_ref": "Rasbandit/Engram/.github/workflows/ci.yml@refs/heads/main",
	}
}

func validConfig(iss *oidctest.Issuer) OIDCConfig {
	return OIDCConfig{
		JWKSURL:     iss.JWKSURL(),
		Issuer:      "https://token.actions.githubusercontent.com",
		Audience:    "engram-deploy",
		Repository:  "Rasbandit/Engram",
		Ref:         "refs/heads/main",
		WorkflowRef: "Rasbandit/Engram/.github/workflows/ci.yml@refs/heads/main",
	}
}

func TestOIDCValidator_AcceptsValidToken(t *testing.T) {
	iss := oidctest.Shared(t)
	v, err := NewValidator(context.Background(), validConfig(iss))
	if err != nil {
		t.Fatalf("validator init: %v", err)
	}

	token := iss.Mint(t, validClaims("test-jti-1"))

	claims, err := v.Validate(token)
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if claims.JTI != "test-jti-1" {
		t.Errorf("JTI = %q, want test-jti-1", claims.JTI)
	}
}

// Each row mutates exactly one claim away from the valid set and asserts
// the validator rejects. If any row accepts, that gate is missing.
func TestOIDCValidator_RejectsInvalidClaims(t *testing.T) {
	iss := oidctest.Shared(t)
	v, err := NewValidator(context.Background(), validConfig(iss))
	if err != nil {
		t.Fatalf("validator init: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(jwt.MapClaims)
	}{
		{"wrong audience", func(c jwt.MapClaims) { c["aud"] = "some-other-aud" }},
		{"wrong issuer", func(c jwt.MapClaims) { c["iss"] = "https://evil.example.com" }},
		{"wrong repository", func(c jwt.MapClaims) { c["repository"] = "Rasbandit/Evil" }},
		{"wrong ref", func(c jwt.MapClaims) { c["ref"] = "refs/heads/attacker" }},
		{"wrong workflow_ref", func(c jwt.MapClaims) {
			c["workflow_ref"] = "Rasbandit/Engram/.github/workflows/evil.yml@refs/heads/main"
		}},
		{"expired", func(c jwt.MapClaims) {
			c["iat"] = time.Now().Add(-1 * time.Hour).Unix()
			c["nbf"] = time.Now().Add(-1 * time.Hour).Unix()
			c["exp"] = time.Now().Add(-30 * time.Minute).Unix()
		}},
		{"missing exp", func(c jwt.MapClaims) { delete(c, "exp") }},
		{"missing jti", func(c jwt.MapClaims) { delete(c, "jti") }},
		{"empty jti", func(c jwt.MapClaims) { c["jti"] = "" }},
		{"missing repository", func(c jwt.MapClaims) { delete(c, "repository") }},
		{"missing ref", func(c jwt.MapClaims) { delete(c, "ref") }},
		{"missing workflow_ref", func(c jwt.MapClaims) { delete(c, "workflow_ref") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := validClaims("jti-" + tc.name)
			tc.mutate(claims)
			token := iss.Mint(t, claims)

			if _, err := v.Validate(token); err == nil {
				t.Fatalf("validator accepted token mutated for %q; should have rejected", tc.name)
			}
		})
	}
}

// alg=none and similar algorithm-confusion attacks must be refused
// regardless of claim values.
func TestOIDCValidator_RejectsAlgNone(t *testing.T) {
	iss := oidctest.Shared(t)
	v, err := NewValidator(context.Background(), validConfig(iss))
	if err != nil {
		t.Fatalf("validator init: %v", err)
	}

	// Build a token with alg=none by hand (golang-jwt refuses to sign one).
	token := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims("alg-none-attack"))
	token.Header["kid"] = oidctest.KeyID
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("mint alg=none token: %v", err)
	}

	if _, err := v.Validate(signed); err == nil {
		t.Fatal("validator accepted alg=none token; signature gate is broken")
	}
}

// Token signed with a wrong key must be rejected by signature verification.
func TestOIDCValidator_RejectsWrongSignatureKey(t *testing.T) {
	iss := oidctest.Shared(t)
	v, err := NewValidator(context.Background(), validConfig(iss))
	if err != nil {
		t.Fatalf("validator init: %v", err)
	}

	// Generate an unrelated RSA key, sign the otherwise-valid claims with it,
	// but lie in the header by claiming the test issuer's kid.
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate wrong key: %v", err)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims("wrong-key"))
	tok.Header["kid"] = oidctest.KeyID
	signed, err := tok.SignedString(wrongKey)
	if err != nil {
		t.Fatalf("sign with wrong key: %v", err)
	}

	if _, err := v.Validate(signed); err == nil {
		t.Fatal("validator accepted token signed by wrong key")
	}
}
