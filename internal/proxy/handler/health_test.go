package handler

import (
	"edge-proxy/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicHealthHandlerDoesNotExposeBackendDetails(t *testing.T) {
	backendURL := "http://backend.internal:3000"
	rt := newTestRuntime(t, &config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{
			{Id: "backend", URL: backendURL, Weight: 1, Enabled: true},
		},
		HealthCheck: config.HealthCheckConfig{
			Path:             "/health",
			IntervalSeconds:  1,
			TimeoutSeconds:   1,
			HealthyThreshold: 1,
			SuccessCodes:     []int32{200},
		},
		Timeouts: config.TimeoutsConfig{
			ConnectTimeoutMs:   1000,
			ResponseTimeoutMs:  1000,
			KeepAliveTimeoutMs: 1000,
			IdleConnTimeoutMs:  1000,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://app.local/health", nil)
	rec := httptest.NewRecorder()

	PublicHealthHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); strings.Contains(body, backendURL) {
		t.Fatalf("public health response exposed backend URL: %s", body)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"status":"healthy"}` {
		t.Fatalf("body = %q", body)
	}
}

func TestPublicHealthHandlerReportsUnavailable(t *testing.T) {
	backendURL := "http://backend.internal:3000"
	rt := newTestRuntime(t, &config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{
			{Id: "backend", URL: backendURL, Weight: 1, Enabled: true},
		},
		HealthCheck: config.HealthCheckConfig{
			Path:             "/health",
			IntervalSeconds:  1,
			TimeoutSeconds:   1,
			HealthyThreshold: 1,
			SuccessCodes:     []int32{200},
		},
		Timeouts: config.TimeoutsConfig{
			ConnectTimeoutMs:   1000,
			ResponseTimeoutMs:  1000,
			KeepAliveTimeoutMs: 1000,
			IdleConnTimeoutMs:  1000,
		},
	})
	current := rt.State()
	status, ok := current.BackendStatus("backend")
	if !ok {
		t.Fatal("missing backend status")
	}
	status.Active.Store(false)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/health", nil)
	rec := httptest.NewRecorder()

	PublicHealthHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
