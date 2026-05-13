package auth

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

// testIssuer mints signed JWTs and serves their public key via a /jwks
// endpoint, mimicking GitHub Actions' token issuer. Shared across tests
// via package-level singleton to amortize RSA key generation cost.

const testKeyID = "engram-deployer-test-key"

var (
	sharedIssuerOnce sync.Once
	sharedIssuer     *testIssuer
)

type testIssuer struct {
	server     *httptest.Server
	privateKey *rsa.PrivateKey
}

// getSharedIssuer returns a process-wide test issuer.
//
// RSA-2048 generation is slow (~50-100ms); we pay once per `go test` run
// rather than once per test function.
func getSharedIssuer(t *testing.T) *testIssuer {
	t.Helper()
	sharedIssuerOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
			jwks := map[string]any{
				"keys": []map[string]any{
					{
						"kty": "RSA",
						"kid": testKeyID,
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
		sharedIssuer = &testIssuer{
			server:     httptest.NewServer(mux),
			privateKey: key,
		}
	})
	return sharedIssuer
}

func (ti *testIssuer) JWKSURL() string {
	return ti.server.URL + "/jwks"
}

// Mint signs claims as RS256 with the test key. Header includes kid so the
// validator's JWKS lookup finds the right public key.
func (ti *testIssuer) Mint(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKeyID
	signed, err := token.SignedString(ti.privateKey)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return signed
}
