package health

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy/runtime"
	"encoding/json"
	"os"
	"testing"
)

func newHealthTestChecker(t *testing.T, threshold int32) (*HealthChecker, *config.BackendConfig, *runtime.BackendStatus) {
	t.Helper()

	backend := &config.BackendConfig{
		Id:      "backend-1",
		URL:     "http://backend-1",
		Weight:  1,
		Enabled: true,
	}

	fullConfig := &config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{backend},
		HealthCheck: config.HealthCheckConfig{
			Path:             "/health",
			IntervalSeconds:  1,
			TimeoutSeconds:   1,
			HealthyThreshold: threshold,
			SuccessCodes:     []int32{200},
		},
		Timeouts: config.TimeoutsConfig{
			ConnectTimeoutMs:   1000,
			ResponseTimeoutMs:  1000,
			KeepAliveTimeoutMs: 1000,
			IdleConnTimeoutMs:  1000,
		},
	}

	configData, err := json.Marshal(fullConfig)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	configFile, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create config file: %v", err)
	}
	if _, err := configFile.Write(configData); err != nil {
		_ = configFile.Close()
		t.Fatalf("write config: %v", err)
	}
	if err := configFile.Close(); err != nil {
		t.Fatalf("close config file: %v", err)
	}

	rt, err := runtime.NewRuntime(configFile.Name())
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	current := rt.State()
	status, ok := current.BackendStatus(backend.Id)
	if !ok {
		t.Fatal("missing backend status")
	}
	status.Active.Store(true)

	return NewHealthChecker(rt, &fullConfig.HealthCheck), backend, status
}

func TestUpdateBackendStatusHonorsHealthyThreshold(t *testing.T) {
	checker, backend, status := newHealthTestChecker(t, 2)

	status.ErrorCount.Store(1)
	checker.UpdateBackendStatus(backend, false)
	if !status.Active.Load() {
		t.Fatal("backend became inactive before reaching healthy threshold")
	}

	status.ErrorCount.Store(2)
	checker.UpdateBackendStatus(backend, false)
	if status.Active.Load() {
		t.Fatal("backend stayed active after reaching healthy threshold")
	}
}

func TestUpdateBackendStatusReactivatesHealthyBackend(t *testing.T) {
	checker, backend, status := newHealthTestChecker(t, 3)

	status.Active.Store(false)
	status.ErrorCount.Store(0)

	checker.UpdateBackendStatus(backend, true)

	if !status.Active.Load() {
		t.Fatal("healthy backend was not reactivated")
	}
}
