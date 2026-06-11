package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPResolver(t *testing.T) {
	resolver, err := NewClientIPResolver("10.0.0.0/8, 192.168.0.0/16")
	if err != nil {
		t.Fatalf("NewClientIPResolver returned error: %v", err)
	}

	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		want       string
	}{
		{
			name:       "ignores forwarded header from untrusted peer",
			remoteAddr: "203.0.113.10:1234",
			forwarded:  "198.51.100.5",
			want:       "203.0.113.10",
		},
		{
			name:       "uses forwarded client from trusted peer",
			remoteAddr: "10.0.0.2:1234",
			forwarded:  "198.51.100.5",
			want:       "198.51.100.5",
		},
		{
			name:       "skips trusted proxies from right to left",
			remoteAddr: "10.0.0.2:1234",
			forwarded:  "198.51.100.5, 192.168.1.10",
			want:       "198.51.100.5",
		},
		{
			name:       "falls back to peer for malformed header",
			remoteAddr: "10.0.0.2:1234",
			forwarded:  "not-an-ip",
			want:       "10.0.0.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://app.local/", nil)
			req.RemoteAddr = tt.remoteAddr
			req.Header.Set("X-Forwarded-For", tt.forwarded)

			if got := resolver.Resolve(req); got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewClientIPResolverRejectsInvalidCIDR(t *testing.T) {
	if _, err := NewClientIPResolver("invalid"); err == nil {
		t.Fatal("expected invalid CIDR error")
	}
}

func TestClientIPResolverMiddlewareSanitizesForwardedHeader(t *testing.T) {
	resolver, err := NewClientIPResolver("")
	if err != nil {
		t.Fatalf("NewClientIPResolver returned error: %v", err)
	}

	var forwarded string
	next := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		forwarded = req.Header.Get("X-Forwarded-For")
	})
	req := httptest.NewRequest(http.MethodGet, "http://app.local/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.5")

	resolver.Middleware(next).ServeHTTP(httptest.NewRecorder(), req)

	if forwarded != "203.0.113.10" {
		t.Fatalf("X-Forwarded-For = %q, want %q", forwarded, "203.0.113.10")
	}
}
