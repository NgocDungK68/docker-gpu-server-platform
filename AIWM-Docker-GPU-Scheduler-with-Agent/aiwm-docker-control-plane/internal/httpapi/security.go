package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// publicAuth leaves agent enrollment/identity endpoints to their independent auth boundary.
func publicAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && !strings.HasPrefix(r.URL.Path, "/api/v1/agents/") && !strings.HasPrefix(r.URL.Path, "/api/v1/training/") {
			got := bearerToken(r.Header.Get("Authorization"))
			if token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "valid public API bearer token required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// bodyLimit bounds decoded requests even when valid JSON is followed by excess whitespace.
func bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || !strings.HasPrefix(r.URL.Path, "/api/v1/training/") {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		next.ServeHTTP(w, r)
	})
}
