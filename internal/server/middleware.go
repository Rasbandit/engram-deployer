package server

import (
	"net/http"
	"strings"
)

// extractBearer pulls the token out of an "Authorization: Bearer <token>"
// header. Returns ("", false) on any deviation — wrong scheme, missing token,
// trailing whitespace only, lowercase scheme, etc. Strict on purpose:
// security primitives should not be lenient.
func extractBearer(h http.Header) (string, bool) {
	raw := h.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return "", false
	}
	tok := strings.TrimPrefix(raw, prefix)
	if tok == "" || strings.TrimSpace(tok) == "" {
		return "", false
	}
	return tok, true
}
