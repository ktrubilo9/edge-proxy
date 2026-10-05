package health

import (
	"context"
	"edge-proxy/internal/config"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProberUsesConfiguredMethodPathAndSuccessCodes(t *testing.T) {
	var method string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		if r.URL.Path != "/health/ready" {
			t.Errorf("path = %q, want /health/ready", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	result := NewHTTPProber(nil).Probe(
		context.Background(),
		&config.BackendConfig{URL: server.URL + "/"},
		config.HealthProbeConfig{
			Method:       http.MethodHead,
			Path:         "/health/ready",
			TimeoutMs:    500,
			SuccessCodes: []int32{http.StatusOK, http.StatusNoContent},
		},
	)

	if !result.Healthy || result.Err != nil {
		t.Fatalf("probe result = %+v, want healthy", result)
	}
	if result.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusNoContent)
	}
	if method != http.MethodHead {
		t.Fatalf("method = %q, want HEAD", method)
	}
}

func TestHTTPProberReportsUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	result := NewHTTPProber(nil).Probe(
		context.Background(),
		&config.BackendConfig{URL: server.URL},
		config.HealthProbeConfig{
			Method:       http.MethodGet,
			Path:         "/health",
			TimeoutMs:    500,
			SuccessCodes: []int32{http.StatusOK},
		},
	)

	if result.Healthy || result.Err == nil {
		t.Fatalf("probe result = %+v, want unhealthy error", result)
	}
	if result.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestHTTPProberHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := NewHTTPProber(nil).Probe(
		context.Background(),
		&config.BackendConfig{URL: server.URL},
		config.HealthProbeConfig{
			Method:       http.MethodGet,
			Path:         "/health",
			TimeoutMs:    10,
			SuccessCodes: []int32{http.StatusOK},
		},
	)

	if result.Healthy || result.Err == nil {
		t.Fatalf("probe result = %+v, want timeout", result)
	}
}

func TestConfiguredHTTPProberUsesTransportSettings(t *testing.T) {
	cfg := config.HealthTransportConfig{
		MaxIdleConns:        12,
		MaxIdleConnsPerHost: 3,
		MaxConnsPerHost:     5,
		KeepAliveMs:         2500,
	}
	prober := NewConfiguredHTTPProber(cfg)
	t.Cleanup(prober.CloseIdleConnections)

	transport, ok := prober.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", prober.client.Transport)
	}
	if transport.MaxIdleConns != cfg.MaxIdleConns ||
		transport.MaxIdleConnsPerHost != cfg.MaxIdleConnsPerHost ||
		transport.MaxConnsPerHost != cfg.MaxConnsPerHost {
		t.Fatalf("transport connection limits do not match config: %+v", transport)
	}
	if transport.IdleConnTimeout != 2500*time.Millisecond {
		t.Fatalf("idle timeout = %s, want 2.5s", transport.IdleConnTimeout)
	}
}
