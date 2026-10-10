package health

import (
	"github.com/ktrubilo9/edge-proxy/internal/config"
	"github.com/ktrubilo9/edge-proxy/internal/proxy/runtime"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeHealthRuntime struct {
	cfg     config.HealthCheckConfig
	backend *config.BackendConfig
	status  *runtime.BackendStatus
}

func (f *fakeHealthRuntime) GetBackend(id string) *config.BackendConfig {
	if f.backend != nil && f.backend.Id == id {
		return f.backend
	}
	return nil
}

func (f *fakeHealthRuntime) GetBackends() []*config.BackendConfig {
	return []*config.BackendConfig{f.backend}
}

func (f *fakeHealthRuntime) GetHealthConfig() config.HealthCheckConfig {
	return f.cfg
}

func (f *fakeHealthRuntime) GetBackendStatus(id string) (*runtime.BackendStatus, bool) {
	return f.status, f.backend != nil && f.backend.Id == id
}

func TestRecoveryBackoffStartsAtInitialAndCapsAtMaximum(t *testing.T) {
	cfg := config.HealthCheckConfig{
		Thresholds: config.HealthThresholdConfig{Unhealthy: 2},
		Recovery: config.HealthRecoveryConfig{
			Backoff: config.HealthBackoffConfig{
				Enabled:    true,
				InitialMs:  100,
				MaxMs:      500,
				Multiplier: 2,
			},
		},
	}

	if got := recoveryBackoff(runtime.BackendStatusSnapshot{ConsecutiveFailures: 2}, cfg); got != 100*time.Millisecond {
		t.Fatalf("initial backoff = %s, want 100ms", got)
	}
	if got := recoveryBackoff(runtime.BackendStatusSnapshot{ConsecutiveFailures: 4}, cfg); got != 400*time.Millisecond {
		t.Fatalf("scaled backoff = %s, want 400ms", got)
	}
	if got := recoveryBackoff(runtime.BackendStatusSnapshot{ConsecutiveFailures: 10}, cfg); got != 500*time.Millisecond {
		t.Fatalf("capped backoff = %s, want 500ms", got)
	}
}

func TestShouldScheduleProbeHonorsRecoveryBackoff(t *testing.T) {
	now := time.Now()
	cfg := config.HealthCheckConfig{
		Thresholds: config.HealthThresholdConfig{Unhealthy: 1},
		Recovery: config.HealthRecoveryConfig{
			Backoff: config.HealthBackoffConfig{
				Enabled:    true,
				InitialMs:  1000,
				MaxMs:      5000,
				Multiplier: 2,
			},
		},
	}
	status := runtime.BackendStatusSnapshot{
		HealthState:         runtime.HealthUnhealthy,
		ConsecutiveFailures: 1,
		LastHealthCheck:     now,
	}

	if shouldScheduleProbe(status, cfg, now.Add(999*time.Millisecond)) {
		t.Fatal("probe was scheduled before backoff elapsed")
	}
	if !shouldScheduleProbe(status, cfg, now.Add(time.Second)) {
		t.Fatal("probe was not scheduled after backoff elapsed")
	}
	status.HealthState = runtime.HealthHealthy
	if !shouldScheduleProbe(status, cfg, now) {
		t.Fatal("healthy backend should use the normal schedule")
	}
}

func TestHealthManagerChecksExistingBackendsImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fake := &fakeHealthRuntime{
		backend: &config.BackendConfig{Id: "backend", URL: server.URL, Enabled: true},
		status:  &runtime.BackendStatus{},
		cfg: config.HealthCheckConfig{
			Enabled: true,
			Probe: config.HealthProbeConfig{
				Method:       http.MethodGet,
				Path:         "/health",
				TimeoutMs:    500,
				SuccessCodes: []int32{http.StatusOK},
			},
			Schedule:    config.HealthScheduleConfig{IntervalMs: 60000},
			Concurrency: config.HealthConcurrencyConfig{Workers: 1, QueueSize: 1},
			Thresholds:  config.HealthThresholdConfig{Healthy: 1, Unhealthy: 1},
			Transport: config.HealthTransportConfig{
				MaxIdleConns:        1,
				MaxIdleConnsPerHost: 1,
				MaxConnsPerHost:     1,
				KeepAliveMs:         1000,
			},
		},
	}
	manager := NewHealthManager(fake, nil)
	if err := manager.Start(); err != nil {
		t.Fatalf("start health manager: %v", err)
	}
	t.Cleanup(manager.Stop)

	waitForHealthTest(t, time.Second, fake.status.IsActive)
	if snapshot := fake.status.Snapshot(); snapshot.LastHealthCheck.IsZero() {
		t.Fatal("initial health check time was not recorded")
	}
}
