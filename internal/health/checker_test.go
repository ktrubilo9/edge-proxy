package health

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/lb"
	"edge-proxy/internal/metrics"
	"edge-proxy/internal/proxy/runtime"
	"net/http"
	"testing"
)

func newHealthTestChecker(threshold int32) (*HealthChecker, *config.BackendConfig, *runtime.BackendStatus) {
	backend := &config.BackendConfig{
		URL:     "http://backend-1",
		Weight:  1,
		Enabled: true,
	}

	fullConfig := &config.FullConfig{
		ProxyPort:  8080,
		LBStrategy: "least-connections",
		Backends:   []*config.BackendConfig{backend},
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

	m := metrics.NewMetrics()
	m.Backends.Register(backend.URL)

	rt := &runtime.Runtime{
		Snapshot:      config.BuildSnapshot(fullConfig),
		Metrics:       m,
		BackendStatus: map[string]*runtime.BackendStatus{backend.URL: {}},
		HTTPClient:    &http.Client{},
	}
	rt.LoadBalancer = lb.GetLoadBalancer("least-connections", m)

	status := rt.BackendStatus[backend.URL]
	status.Active.Store(true)

	return NewHealthChecker(rt, &fullConfig.HealthCheck), backend, status
}

func TestUpdateBackendStatusHonorsHealthyThreshold(t *testing.T) {
	checker, backend, status := newHealthTestChecker(2)

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
	checker, backend, status := newHealthTestChecker(3)

	status.Active.Store(false)
	status.ErrorCount.Store(0)

	checker.UpdateBackendStatus(backend, true)

	if !status.Active.Load() {
		t.Fatal("healthy backend was not reactivated")
	}
}
