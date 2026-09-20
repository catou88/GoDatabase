package middleware

import (
	"net/http"
	"strings"
)

func CORS(origin string) func(http.Handler) http.Handler {
	allowedOrigins := make([]string, 0)
	for _, value := range strings.Split(origin, ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			allowedOrigins = append(allowedOrigins, trimmed)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestOrigin := r.Header.Get("Origin")
			if containsOrigin(allowedOrigins, requestOrigin) {
				w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func containsOrigin(allowed []string, origin string) bool {
	for _, candidate := range allowed {
		if candidate == origin {
			return true
		}
	}
	return false
}
