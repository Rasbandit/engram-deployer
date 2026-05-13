// Package oidctest provides an in-process RSA-signed JWT issuer that mimics
// GitHub Actions' OIDC token issuer. Used by tests across packages.
//
// A process-wide singleton amortizes RSA-2048 key generation (~50-100ms).
package oidctest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// KeyID is the kid header value used by Mint and exposed in the JWKS.
const KeyID = "engram-deployer-test-key"

var (
	sharedOnce sync.Once
	shared     *Issuer
)

// Issuer hosts a /jwks endpoint and signs JWTs with its corresponding
// private key.
type Issuer struct {
	server     *httptest.Server
	privateKey *rsa.PrivateKey
}

// Shared returns a process-wide test issuer. Safe to call from any test;
// the httptest server lives until the test binary exits.
func Shared(t *testing.T) *Issuer {
	t.Helper()
	sharedOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
			jwks := map[string]any{
				"keys": []map[string]any{
					{
						"kty": "RSA",
						"kid": KeyID,
						"use": "sig",
						"alg": "RS256",
						"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
						"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jwks)
		})
		shared = &Issuer{
			server:     httptest.NewServer(mux),
			privateKey: key,
		}
	})
	return shared
}

// JWKSURL is the URL of the in-process JWKS endpoint.
func (i *Issuer) JWKSURL() string { return i.server.URL + "/jwks" }

// PrivateKey returns the issuer's RSA private key (for tests that need to
// sign tokens directly, e.g. wrong-key forgeries).
func (i *Issuer) PrivateKey() *rsa.PrivateKey { return i.privateKey }

// Mint signs claims as RS256 and tags the header with KeyID so a JWKS
// lookup finds the matching public key.
func (i *Issuer) Mint(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = KeyID
	signed, err := token.SignedString(i.privateKey)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return signed
}
