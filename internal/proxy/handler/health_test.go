package handler

import (
	"edge-proxy/internal/config"
	healthview "edge-proxy/internal/health"
	"encoding/json"
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

func TestHealthHandlerReportsLastHealthCheckByBackendID(t *testing.T) {
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

	const lastHealthCheck = int64(123456789)
	status, ok := rt.State().BackendStatus("backend")
	if !ok {
		t.Fatal("missing backend status")
	}
	status.LastHealthCheck.Store(lastHealthCheck)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/health", nil)
	rec := httptest.NewRecorder()

	HealthHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body healthview.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Backends) != 1 {
		t.Fatalf("backend count = %d, want 1", len(body.Backends))
	}
	if body.Backends[0].LastHealthCheck != lastHealthCheck {
		t.Fatalf("last health check = %d, want %d", body.Backends[0].LastHealthCheck, lastHealthCheck)
	}
}
