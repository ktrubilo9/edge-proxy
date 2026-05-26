package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAuthMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		token          string
		authorization  string
		wantStatusCode int
		wantNextCalled bool
	}{
		{
			name:           "disabled auth allows request without token",
			enabled:        false,
			token:          "",
			wantStatusCode: http.StatusNoContent,
			wantNextCalled: true,
		},
		{
			name:           "enabled auth rejects empty configured token",
			enabled:        true,
			token:          "",
			authorization:  "Bearer secret",
			wantStatusCode: http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "enabled auth rejects missing authorization header",
			enabled:        true,
			token:          "secret",
			wantStatusCode: http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "enabled auth rejects malformed bearer header",
			enabled:        true,
			token:          "secret",
			authorization:  "Bearer",
			wantStatusCode: http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "enabled auth rejects non bearer scheme",
			enabled:        true,
			token:          "secret",
			authorization:  "Basic secret",
			wantStatusCode: http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "enabled auth rejects wrong token",
			enabled:        true,
			token:          "secret",
			authorization:  "Bearer wrong",
			wantStatusCode: http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "enabled auth allows valid bearer token",
			enabled:        true,
			token:          "secret",
			authorization:  "Bearer secret",
			wantStatusCode: http.StatusNoContent,
			wantNextCalled: true,
		},
		{
			name:           "enabled auth accepts case insensitive bearer scheme",
			enabled:        true,
			token:          "secret",
			authorization:  "bearer secret",
			wantStatusCode: http.StatusNoContent,
			wantNextCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/backend", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()

			NewAuthMiddleware(tt.enabled, tt.token, next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatusCode)
			}
			if nextCalled != tt.wantNextCalled {
				t.Fatalf("nextCalled = %v, want %v", nextCalled, tt.wantNextCalled)
			}
		})
	}
}
