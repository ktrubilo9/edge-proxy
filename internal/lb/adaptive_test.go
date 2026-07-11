package lb

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/metrics"
	"math"
	"sync/atomic"
	"testing"
)

func TestAdaptiveLBNextEmptyBackends(t *testing.T) {
	lb := NewAdaptiveLB(metrics.NewMetrics())
	_, err := lb.Next(nil)
	if err != ErrNoAvailableBackend {
		t.Errorf("expected ErrNoAvailableBackend, got %v", err)
	}
}

func TestAdaptiveLBNextAllDisabled(t *testing.T) {
	lb := NewAdaptiveLB(metrics.NewMetrics())

	backends := []*config.BackendConfig{
		{URL: "http://b1", Enabled: false},
		{URL: "http://b2", Enabled: false},
	}

	_, err := lb.Next(backends)
	if err != ErrNoAvailableBackend {
		t.Errorf("expected ErrNoAvailableBackend when all backends disabled, got %v", err)
	}
}

func TestAdaptiveLBNextNoMetricsFallbackToRandom(t *testing.T) {
	lb := NewAdaptiveLB(metrics.NewMetrics())

	backends := []*config.BackendConfig{
		{URL: "http://b1", Enabled: true},
		{URL: "http://b2", Enabled: true},
	}

	backend, err := lb.Next(backends)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend == nil {
		t.Fatal("expected a backend, got nil")
	}
}

func TestAdaptiveLBNextSingleBackend(t *testing.T) {
	m := metrics.NewMetrics()
	lb := NewAdaptiveLB(m)

	backend := &config.BackendConfig{URL: "http://only-one", Enabled: true}
	backends := []*config.BackendConfig{backend}

	setupBackendMetrics(m, backend.URL, 10, 0.01, 0, 0)

	result, err := lb.Next(backends)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.URL != backend.URL {
		t.Errorf("expected %s, got %s", backend.URL, result.URL)
	}
}

func TestAdaptiveLBNextPrefersHealthierBackend(t *testing.T) {
	m := metrics.NewMetrics()
	lb := NewAdaptiveLB(m)

	b1 := &config.BackendConfig{URL: "http://good", Enabled: true, Weight: 1}
	b2 := &config.BackendConfig{URL: "http://bad", Enabled: true, Weight: 1}

	backends := []*config.BackendConfig{b1, b2}

	// good backend
	setupBackendMetrics(m, b1.URL, 5, 0.01, 0, 2)
	// bad backend
	setupBackendMetrics(m, b2.URL, 120, 0.4, 12, 50)

	goodCount := 0
	iterations := 1000

	for i := 0; i < iterations; i++ {
		result, err := lb.Next(backends)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.URL == b1.URL {
			goodCount++
		}
	}

	if goodCount < 700 {
		t.Errorf("expected good backend to be chosen more often, got %d/%d", goodCount, iterations)
	}
}

func setupBackendMetrics(m *metrics.Metrics, url string, latencyMs float64, errorRate float64, consecutiveFailures uint32, activeConns uint64) {
	bm := m.Backends.Get(url)

	if bm == nil {
		m.Backends.Register(url)
		bm = m.Backends.Get(url)
	}

	atomic.StoreUint64(&bm.LatencyEWMABits, math.Float64bits(latencyMs))
	atomic.StoreUint64(&bm.ErrorRateEWMABits, math.Float64bits(errorRate))
	atomic.StoreUint32(&bm.ConsecutiveFailures, consecutiveFailures)
	atomic.StoreUint64(&bm.ActiveConnections, activeConns)
}
