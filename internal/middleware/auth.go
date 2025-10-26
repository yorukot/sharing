package middleware

import (
	"net/http"
	"os"
)

// APIKeyAuth validates the API key from the request header
func APIKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		expectedKey := os.Getenv("API_KEY")

		if expectedKey == "" {
			http.Error(w, "Server configuration error: API key not set", http.StatusInternalServerError)
			return
		}

		if apiKey == "" {
			http.Error(w, "API key required", http.StatusUnauthorized)
			return
		}

		if apiKey != expectedKey {
			http.Error(w, "Invalid API key", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
