package runtime_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"edge-proxy/internal/config"
	"edge-proxy/internal/health"
	runtimepkg "edge-proxy/internal/proxy/runtime"
	"edge-proxy/internal/testutil"
)

func TestAddEnabledBackendTriggersImmediateHealthCheckAndActivatesHealthyBackend(t *testing.T) {
	var checks atomic.Int32

	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer backendServer.Close()

	rt := newHealthCheckTestRuntime(t, 60)
	_ = newHealthCheckerWithCallback(t, rt)

	if err := rt.AddBackend(config.BackendConfig{
		Id:      "backend-2",
		URL:     backendServer.URL,
		Weight:  1,
		Enabled: true,
	}); err != nil {
		t.Fatalf("add backend: %v", err)
	}

	status, ok := rt.State().BackendStatus("backend-2")
	if !ok {
		t.Fatal("backend status was not created")
	}

	waitFor(t, time.Second, func() bool {
		return checks.Load() >= 1 && status.IsActive()
	})

	if !status.IsActive() {
		t.Fatal("healthy backend was not activated after the immediate health check")
	}
}

func TestAddEnabledBackendTriggersImmediateHealthCheckAndKeepsUnhealthyBackendInactive(t *testing.T) {
	var checks atomic.Int32

	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backendServer.Close()

	rt := newHealthCheckTestRuntime(t, 60)
	_ = newHealthCheckerWithCallback(t, rt)

	if err := rt.AddBackend(config.BackendConfig{
		Id:      "backend-2",
		URL:     backendServer.URL,
		Weight:  1,
		Enabled: true,
	}); err != nil {
		t.Fatalf("add backend: %v", err)
	}

	status, ok := rt.State().BackendStatus("backend-2")
	if !ok {
		t.Fatal("backend status was not created")
	}

	waitFor(t, time.Second, func() bool {
		return checks.Load() >= 1
	})

	if status.IsActive() {
		t.Fatal("unhealthy backend became active after the immediate health check")
	}
}

func TestEnableDisabledBackendTriggersImmediateHealthCheckAndActivatesHealthyBackend(t *testing.T) {
	var checks atomic.Int32

	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer backendServer.Close()

	rt := newHealthCheckTestRuntime(t, 60)
	_ = newHealthCheckerWithCallback(t, rt)

	if err := rt.UpdateBackend("backend-1", backendServer.URL, 1, true); err != nil {
		t.Fatalf("enable backend: %v", err)
	}

	status, ok := rt.State().BackendStatus("backend-1")
	if !ok {
		t.Fatal("backend status was not found")
	}

	waitFor(t, time.Second, func() bool {
		return checks.Load() >= 1 && status.IsActive()
	})

	if !status.IsActive() {
		t.Fatal("healthy backend was not activated after enable")
	}
}

func TestEnableDisabledBackendTriggersImmediateHealthCheckAndKeepsUnhealthyBackendInactive(t *testing.T) {
	var checks atomic.Int32
	var healthy atomic.Bool
	healthy.Store(true)

	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks.Add(1)
		if healthy.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backendServer.Close()

	// Threshold greater than one so the test proves that enable resets
	// Active=false before the first failed check is evaluated.
	rt := newHealthCheckTestRuntime(t, 2)
	_ = newHealthCheckerWithCallback(t, rt)

	if err := rt.UpdateBackend("backend-1", backendServer.URL, 1, true); err != nil {
		t.Fatalf("initially enable backend: %v", err)
	}

	status, ok := rt.State().BackendStatus("backend-1")
	if !ok {
		t.Fatal("backend status was not found")
	}

	waitFor(t, time.Second, func() bool {
		return checks.Load() >= 1 && status.IsActive()
	})

	if err := rt.UpdateBackend("backend-1", backendServer.URL, 1, false); err != nil {
		t.Fatalf("disable backend: %v", err)
	}

	healthy.Store(false)
	previousChecks := checks.Load()

	if err := rt.UpdateBackend("backend-1", backendServer.URL, 1, true); err != nil {
		t.Fatalf("re-enable backend: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		return checks.Load() > previousChecks
	})

	if status.IsActive() {
		t.Fatal("unhealthy backend remained active after re-enable and immediate health check")
	}
}

func newHealthCheckerWithCallback(t *testing.T, rt *runtimepkg.Runtime) *health.HealthManager {
	hm := health.NewHealthManager(rt, rt.Metrics)

	rt.SetOnBackendHealthCheckRequired(func(backend config.BackendConfig) {
		go hm.CheckBackend(backend.Id)
	})

	if err := hm.Start(); err != nil {
		t.Fatalf("start health manager: %v", err)
	}

	t.Cleanup(hm.Stop)

	return hm
}

func newHealthCheckTestRuntime(t *testing.T, healthyThreshold int32) *runtimepkg.Runtime {
	t.Helper()

	healthConfig := testutil.DefaultHealthCheckConfigWithInterval(60000)
	cfg := config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{
			{
				Id:      "backend-1",
				URL:     "http://backend-1",
				Weight:  1,
				Enabled: false,
			},
			{
				Id:      "backend-seed",
				URL:     "http://backend-seed",
				Weight:  1,
				Enabled: true,
			},
		},
		HealthCheck: healthConfig,
		Timeouts: config.TimeoutsConfig{
			ConnectTimeoutMs:   1000,
			ResponseTimeoutMs:  2000,
			KeepAliveTimeoutMs: 3000,
			IdleConnTimeoutMs:  4000,
		},
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

	rt, err := runtimepkg.NewRuntime(path)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}

	return rt
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition was not satisfied before timeout")
}
