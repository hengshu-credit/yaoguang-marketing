package middleware

import (
	"net/http"
	"os"
)

// CORSMiddleware handles CORS headers for all requests
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get allowed origin from environment variable with default value "*"
		allowOrigin := os.Getenv("CORS_ALLOW_ORIGIN")
		if allowOrigin == "" {
			allowOrigin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)

		// Allow specific HTTP methods
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		// Allow specific headers
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")

		// Allow credentials
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
