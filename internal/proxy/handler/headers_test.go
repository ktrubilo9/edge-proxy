package handler

import (
	"net/http"
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
