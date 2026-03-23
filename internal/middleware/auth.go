package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// APIKeyAuth returns middleware that validates requests against the provided
// API key. If apiKey is empty, authentication is disabled (pass-through).
// The /health endpoint is always exempt.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if no key configured, for health checks, or for the frontend UI
			if apiKey == "" || r.URL.Path == "/health" || r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/ui/") {
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")

			if token == auth || subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
