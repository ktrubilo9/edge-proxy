package admin

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func NewAuthMiddleware(enabled bool, token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			next.ServeHTTP(w, r)
			return
		}
		if token == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="admin-api"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || subtle.ConstantTimeCompare([]byte(token), []byte(parts[1])) == 0 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="admin-api"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
