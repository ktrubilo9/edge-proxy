package lb

import (
	"testing"

	"github.com/ktrubilo9/edge-proxy/internal/config"
	"github.com/ktrubilo9/edge-proxy/internal/metrics"
)

func TestLeastConnectionsNext(t *testing.T) {
	tests := []struct {
		name     string
		backends []*config.BackendConfig
		counts   map[string]uint64
		wantURL  string
	}{
		{name: "empty list"},
		{
			name:     "all disabled",
			backends: []*config.BackendConfig{{URL: "disabled", Enabled: false}},
			counts:   map[string]uint64{"disabled": 0},
		},
		{
			name:     "enabled without metrics",
			backends: []*config.BackendConfig{{URL: "missing", Enabled: true}},
		},
		{
			name:     "single available backend",
			backends: []*config.BackendConfig{{URL: "only", Enabled: true}},
			counts:   map[string]uint64{"only": 3},
			wantURL:  "only",
		},
		{
			name: "lowest count ignores disabled and missing metrics",
			backends: []*config.BackendConfig{
				{URL: "disabled", Enabled: false},
				{URL: "busy", Enabled: true},
				{URL: "missing", Enabled: true},
				{URL: "quiet", Enabled: true},
			},
			counts:  map[string]uint64{"disabled": 0, "busy": 5, "quiet": 2},
			wantURL: "quiet",
		},
		{
			name: "equal counts retain the first backend",
			backends: []*config.BackendConfig{
				{URL: "first", Enabled: true},
				{URL: "second", Enabled: true},
			},
			counts:  map[string]uint64{"first": 2, "second": 2},
			wantURL: "first",
		},
		{
			name: "counts above the former sentinel",
			backends: []*config.BackendConfig{
				{URL: "busy", Enabled: true},
				{URL: "quiet", Enabled: true},
			},
			counts:  map[string]uint64{"busy": 1 << 32, "quiet": 1 << 31},
			wantURL: "quiet",
		},
		{
			name:     "maximum connection count is still available",
			backends: []*config.BackendConfig{{URL: "only", Enabled: true}},
			counts:   map[string]uint64{"only": ^uint64(0)},
			wantURL:  "only",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := metrics.NewMetrics()
			for url, count := range tt.counts {
				setupBackendMetrics(m, url, 0, 0, 0, count)
			}
			backend, err := NewLeastConnections(m).Next(tt.backends)
			if tt.wantURL == "" {
				if backend != nil || err != ErrNoAvailableBackend {
					t.Fatalf("expected nil backend and ErrNoAvailableBackend, got %v, %v", backend, err)
				}
				return
			}
			if err != nil || backend == nil {
				t.Fatalf("expected backend %q without error, got %v, %v", tt.wantURL, backend, err)
			}
			if backend.URL != tt.wantURL {
				t.Fatalf("expected backend %q, got %q", tt.wantURL, backend.URL)
			}
		})
	}
}
