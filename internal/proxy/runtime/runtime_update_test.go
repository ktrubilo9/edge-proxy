package runtime

import (
	"edge-proxy/internal/config"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestApplyUpdatePublishesBackendChangesAtomically(t *testing.T) {
	rt := newRuntimeUpdateTestInstance(t)
	previous := rt.State()
	previousStatus, ok := rt.BackendStatus("http://backend-1")
	if !ok {
		t.Fatal("missing initial backend status")
	}

	addedURL := "http://backend-2"
	if err := rt.AddBackend(config.BackendConfig{
		URL:     addedURL,
		Weight:  1,
		Enabled: true,
	}); err != nil {
		t.Fatalf("add backend: %v", err)
	}

	current := rt.State()
	if current == previous {
		t.Fatal("runtime state was not replaced")
	}
	if current.HTTPClient != previous.HTTPClient {
		t.Fatal("HTTP client was replaced by a backend-only update")
	}
	if current.LoadBalancer != previous.LoadBalancer {
		t.Fatal("load balancer was replaced by a backend-only update")
	}
	if current.Snapshot.BackendsByURL[addedURL] == nil {
		t.Fatal("new backend is missing from the published snapshot")
	}
	if rt.Metrics.Backends.Get(addedURL) == nil {
		t.Fatal("new backend metrics were not registered")
	}
	if _, ok := rt.BackendStatus(addedURL); !ok {
		t.Fatal("new backend status was not registered")
	}

	currentStatus, ok := rt.BackendStatus("http://backend-1")
	if !ok || currentStatus != previousStatus {
		t.Fatal("existing backend status was not preserved")
	}

	if err := rt.RemoveBackend(addedURL); err != nil {
		t.Fatalf("remove backend: %v", err)
	}
	if rt.State().Snapshot.BackendsByURL[addedURL] != nil {
		t.Fatal("removed backend is still present in the published snapshot")
	}
	if rt.Metrics.Backends.Get(addedURL) != nil {
		t.Fatal("removed backend metrics are still registered")
	}
	if _, ok := rt.BackendStatus(addedURL); ok {
		t.Fatal("removed backend status is still registered")
	}
}

func TestApplyUpdateFailureLeavesRuntimeStateUntouched(t *testing.T) {
	rt := newRuntimeUpdateTestInstance(t)
	previous := rt.State()
	previousMetrics := rt.Metrics.Backends.Get("http://backend-1")

	err := rt.AddBackend(config.BackendConfig{
		URL:     "http://backend-1",
		Weight:  1,
		Enabled: true,
	})
	if err == nil {
		t.Fatal("expected duplicate backend update to fail")
	}

	if rt.State() != previous {
		t.Fatal("failed update replaced the runtime state")
	}
	if rt.Metrics.Backends.Get("http://backend-1") != previousMetrics {
		t.Fatal("failed update replaced backend metrics")
	}
}

func TestApplyUpdateSerializesConcurrentBackendChanges(t *testing.T) {
	rt := newRuntimeUpdateTestInstance(t)

	const updates = 12
	errs := make(chan error, updates)
	var wg sync.WaitGroup
	for i := 0; i < updates; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			url := fmt.Sprintf("http://backend-%d", index+2)
			errs <- rt.AddBackend(config.BackendConfig{
				URL:     url,
				Weight:  1,
				Enabled: true,
			})
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent update failed: %v", err)
		}
	}

	state := rt.State()
	if got, want := len(state.Snapshot.Raw.Backends), updates+1; got != want {
		t.Fatalf("backend count = %d, want %d", got, want)
	}
	for i := 0; i < updates; i++ {
		url := fmt.Sprintf("http://backend-%d", i+2)
		if state.Snapshot.BackendsByURL[url] == nil {
			t.Fatalf("published snapshot is missing %s", url)
		}
		if rt.Metrics.Backends.Get(url) == nil {
			t.Fatalf("metrics are missing %s", url)
		}
		if _, ok := rt.BackendStatus(url); !ok {
			t.Fatalf("status registry is missing %s", url)
		}
	}
}

func TestRateLimitUpdateDoesNotRequireCallback(t *testing.T) {
	rt := newRuntimeUpdateTestInstance(t)
	rate := config.RateLimitingConfig{
		Enabled:   true,
		RatePerIP: 10,
		Burst:     20,
		WindowSec: 60,
	}

	if err := rt.UpdateVirtualHostRateLimiting("app.local", rate); err != nil {
		t.Fatalf("update rate limit without callback: %v", err)
	}

	got := rt.GetVirtualHostSecurityConfig("app.local")
	if got == nil || got.RateLimiting != rate {
		t.Fatal("published runtime state does not contain the rate limit update")
	}
}

func newRuntimeUpdateTestInstance(t *testing.T) *Runtime {
	t.Helper()

	cfg := config.FullConfig{
		ProxyPort:  8080,
		LBStrategy: "least-connections",
		Backends: []*config.BackendConfig{
			{URL: "http://backend-1", Weight: 1, Enabled: true},
		},
		HealthCheck: config.HealthCheckConfig{
			Path:             "/health",
			IntervalSeconds:  1,
			TimeoutSeconds:   1,
			HealthyThreshold: 1,
			SuccessCodes:     []int32{200},
		},
		Timeouts: defaultRuntimeTestTimeouts(),
		VirtualHosts: []config.VirtualHost{
			{
				Domain:   "app.local",
				Backends: []string{"http://backend-1"},
				Security: &config.SecurityConfig{},
			},
		},
		Logging: config.LoggingConfig{
			Level:      "info",
			BufferSize: 16,
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	rt, err := NewRuntime(path)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	return rt
}
