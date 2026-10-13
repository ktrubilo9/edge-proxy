package handler

import (
	"net/http"
	"reflect"
	"testing"
)

func TestIsUpgradeRequest(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   bool
	}{
		{
			name: "websocket upgrade",
			header: http.Header{
				"Connection": {"keep-alive, Upgrade"},
				"Upgrade":    {"websocket"},
			},
			want: true,
		},
		{
			name: "mixed case connection token",
			header: http.Header{
				"connection": {"keep-alive", "uPgRaDe"},
				"upgrade":    {"h2c"},
			},
			want: true,
		},
		{
			name: "upgrade header without connection token",
			header: http.Header{
				"upgrade": {"websocket"},
			},
			want: true,
		},
		{
			name: "connection token without upgrade header",
			header: http.Header{
				"Connection": {"upgrade"},
			},
			want: true,
		},
		{
			name: "similarly named token",
			header: http.Header{
				"Connection": {"X-Not-Upgrade"},
				"X-Upgrade":  {"value"},
			},
			want: false,
		},
		{
			name: "ordinary request",
			header: http.Header{
				"Connection": {"keep-alive"},
				"Accept":     {"application/json"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUpgradeRequest(tt.header); got != tt.want {
				t.Fatalf("isUpgradeRequest() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRemoveHopByHopHeaders(t *testing.T) {
	tests := []struct {
		name      string
		header    http.Header
		removed   []string
		preserved http.Header
	}{
		{
			name: "standard hop-by-hop headers",
			header: http.Header{
				"Connection":          {"keep-alive"},
				"Keep-Alive":          {"timeout=5"},
				"Proxy-Authenticate":  {"Basic"},
				"Proxy-Authorization": {"Basic credentials"},
				"Proxy-Connection":    {"keep-alive"},
				"Te":                  {"trailers"},
				"Trailer":             {"X-Checksum"},
				"Transfer-Encoding":   {"chunked"},
				"Upgrade":             {"websocket"},
				"X-End-To-End":        {"preserved"},
			},
			removed: []string{
				"Connection",
				"Keep-Alive",
				"Proxy-Authenticate",
				"Proxy-Authorization",
				"Proxy-Connection",
				"TE",
				"Trailer",
				"Transfer-Encoding",
				"Upgrade",
			},
			preserved: http.Header{"X-End-To-End": {"preserved"}},
		},
		{
			name: "connection tokens use every value and ignore casing",
			header: http.Header{
				"cOnNeCtIoN":        {" X-Temporary-State, x-second-state ", "X-Third-State,,"},
				"x-temporary-state": {"temporary"},
				"X-Second-State":    {"second"},
				"x-THIRD-state":     {"third"},
				"Set-Cookie":        {"first=1", "second=2"},
				"X-End-To-End":      {"one", "two"},
			},
			removed: []string{
				"Connection",
				"X-Temporary-State",
				"X-Second-State",
				"X-Third-State",
			},
			preserved: http.Header{
				"Set-Cookie":   {"first=1", "second=2"},
				"X-End-To-End": {"one", "two"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removeHopByHopHeaders(tt.header)

			for _, name := range tt.removed {
				if headerContains(tt.header, name) {
					t.Errorf("header %q was not removed: %v", name, tt.header)
				}
			}
			for name, want := range tt.preserved {
				if got := headerValues(tt.header, name); !reflect.DeepEqual(got, want) {
					t.Errorf("header %q = %v, want %v", name, got, want)
				}
			}
		})
	}
}

func headerContains(header http.Header, target string) bool {
	for name := range header {
		if http.CanonicalHeaderKey(name) == http.CanonicalHeaderKey(target) {
			return true
		}
	}
	return false
}

func headerValues(header http.Header, target string) []string {
	for name, values := range header {
		if http.CanonicalHeaderKey(name) == http.CanonicalHeaderKey(target) {
			return values
		}
	}
	return nil
}
