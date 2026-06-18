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
	previousStatus, ok := previous.BackendStatus("backend-1")
	if !ok {
		t.Fatal("missing initial backend status")
	}

	addedID := "backend-2"
	addedURL := "http://backend-2"
	if err := rt.AddBackend(config.BackendConfig{
		Id:      addedID,
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
	if current.Snapshot.BackendsById[addedID] == nil {
		t.Fatal("new backend is missing from the published snapshot")
	}
	if rt.Metrics.Backends.Get(addedURL) == nil {
		t.Fatal("new backend metrics were not registered")
	}
	if _, ok := current.BackendStatus(addedID); !ok {
		t.Fatal("new backend status was not registered")
	}

	currentStatus, ok := current.BackendStatus("backend-1")
	if !ok || currentStatus != previousStatus {
		t.Fatal("existing backend status was not preserved")
	}

	if err := rt.RemoveBackend(addedID); err != nil {
		t.Fatalf("remove backend: %v", err)
	}
	if rt.State().Snapshot.BackendsById[addedID] != nil {
		t.Fatal("removed backend is still present in the published snapshot")
	}
	if rt.Metrics.Backends.Get(addedURL) != nil {
		t.Fatal("removed backend metrics are still registered")
	}
	current = rt.State()
	if _, ok := current.BackendStatus(addedID); ok {
		t.Fatal("removed backend status is still registered")
	}
}

func TestApplyUpdateFailureLeavesRuntimeStateUntouched(t *testing.T) {
	rt := newRuntimeUpdateTestInstance(t)
	previous := rt.State()
	previousMetrics := rt.Metrics.Backends.Get("http://backend-1")

	err := rt.AddBackend(config.BackendConfig{
		Id:      "backend-1",
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

func TestOldRuntimeStateRetainsRemovedBackendStatus(t *testing.T) {
	rt := newRuntimeUpdateTestInstance(t)
	oldState := rt.State()
	oldStatus, ok := oldState.BackendStatus("backend-1")
	if !ok {
		t.Fatal("old state is missing the initial backend status")
	}

	if err := rt.AddBackend(config.BackendConfig{
		Id:      "backend-2",
		URL:     "http://backend-2",
		Weight:  1,
		Enabled: true,
	}); err != nil {
		t.Fatalf("add replacement backend: %v", err)
	}
	if err := rt.UpdateVirtualHost("app.local", config.VirtualHost{
		BackendIDs:       []string{"backend-2"},
		SecurityPolicyID: "default",
	}); err != nil {
		t.Fatalf("switch virtual host to replacement backend: %v", err)
	}
	if err := rt.RemoveBackend("backend-1"); err != nil {
		t.Fatalf("remove initial backend: %v", err)
	}

	if _, ok := rt.State().BackendStatus("backend-1"); ok {
		t.Fatal("new state contains the removed backend status")
	}

	retained, ok := oldState.BackendStatus("backend-1")
	if !ok {
		t.Fatal("old state lost the removed backend status")
	}
	if retained != oldStatus {
		t.Fatal("old state changed its backend status reference")
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
			id := fmt.Sprintf("backend-%d", index+2)
			url := fmt.Sprintf("http://backend-%d", index+2)
			errs <- rt.AddBackend(config.BackendConfig{
				Id:      id,
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
		id := fmt.Sprintf("backend-%d", i+2)
		url := fmt.Sprintf("http://backend-%d", i+2)
		if state.Snapshot.BackendsById[id] == nil {
			t.Fatalf("published snapshot is missing %s", id)
		}
		if rt.Metrics.Backends.Get(url) == nil {
			t.Fatalf("metrics are missing %s", url)
		}
		if _, ok := state.BackendStatus(id); !ok {
			t.Fatalf("status registry is missing %s", id)
		}
	}
}

func TestPolicyUpdateDoesNotRequireCallback(t *testing.T) {
	rt := newRuntimeUpdateTestInstance(t)
	rate := config.RateLimitingConfig{
		Enabled:   true,
		RatePerIP: 10,
		Burst:     20,
		WindowSec: 60,
	}

	if err := rt.UpsertPolicy(config.SecurityPolicy{Id: "default", RateLimiting: rate}); err != nil {
		t.Fatalf("update policy without callback: %v", err)
	}

	got := rt.GetVirtualHostSecurity("app.local")
	if got == nil || got.Policy.RateLimiting != rate {
		t.Fatal("published runtime state does not contain the rate limit update")
	}
}

func newRuntimeUpdateTestInstance(t *testing.T) *Runtime {
	t.Helper()

	cfg := config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{
			{Id: "backend-1", URL: "http://backend-1", Weight: 1, Enabled: true},
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
				Domain:           "app.local",
				BackendIDs:       []string{"backend-1"},
				SecurityPolicyID: "default",
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
