package handler

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy/runtime"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sync/atomic"
)

func newTestRuntime(t *testing.T, fullConfig *config.FullConfig) *runtime.Runtime {
	t.Helper()

	configData, err := json.Marshal(fullConfig)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	rt, err := runtime.NewRuntime(configPath)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	current := rt.State()
	for _, backend := range fullConfig.Backends {
		if backend == nil {
			continue
		}
		status, ok := current.BackendStatus(backend.Id)
		if !ok {
			t.Fatalf("missing backend status for %s", backend.URL)
		}
		status.Active.Store(true)
	}

	return rt
}

func newSingleRouteTestRuntime(t *testing.T, host string, backendURL string, route *config.PathRoute) *runtime.Runtime {
	t.Helper()

	fullConfig := &config.FullConfig{
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
		VirtualHosts: []config.VirtualHost{
			{
				Domain:           host,
				BackendIDs:       []string{"backend"},
				SecurityPolicyID: "default",
				PathRoutes: func() []config.PathRoute {
					if route == nil {
						return nil
					}
					return []config.PathRoute{*route}
				}(),
			},
		},
	}
	return newTestRuntime(t, fullConfig)
}

func TestProxyHandlerUnknownHostReturnsForbidden(t *testing.T) {
	rt := newSingleRouteTestRuntime(
		t,
		"configured.local",
		"http://127.0.0.1:1",
		nil,
	)

	current := rt.State()
	status, ok := current.BackendStatus("backend")
	if !ok {
		t.Fatal("missing backend status")
	}
	status.Active.Store(false)

	req := httptest.NewRequest(http.MethodGet, "http://unknown.local/", nil)
	req.Host = "unknown.local"
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestProxyHandlerRoutesRequestToBackend(t *testing.T) {
	var gotForwardedFor string
	var gotRequestID string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		gotRequestID = r.Header.Get("X-Request-ID")
		_, _ = w.Write([]byte("backend-ok"))
	}))
	defer backend.Close()

	rt := newSingleRouteTestRuntime(t, "app.local", backend.URL, nil)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/", nil)
	req.Host = "app.local"
	req.RemoteAddr = "203.0.113.10:4567"
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body, _ := io.ReadAll(rec.Body)
	if strings.TrimSpace(string(body)) != "backend-ok" {
		t.Fatalf("body = %q, want %q", string(body), "backend-ok")
	}

	if gotForwardedFor != "203.0.113.10" {
		t.Fatalf("X-Forwarded-For = %q, want %q", gotForwardedFor, "203.0.113.10")
	}

	if gotRequestID == "" {
		t.Fatal("expected X-Request-ID to be forwarded to backend")
	}

	if rec.Header().Get("X-Request-ID") != gotRequestID {
		t.Fatalf("response X-Request-ID = %q, want %q", rec.Header().Get("X-Request-ID"), gotRequestID)
	}

	if got := atomic.LoadUint64(&rt.Metrics.TotalRequests); got != 1 {
		t.Fatalf("total requests = %d, want 1", got)
	}
}

func TestProxyHandlerUsesPathRouteBackends(t *testing.T) {
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("api-backend"))
	}))
	defer apiBackend.Close()

	route := &config.PathRoute{
		Path:       "/api",
		BackendIDs: []string{"backend"},
	}

	rt := newSingleRouteTestRuntime(t, "app.local", apiBackend.URL, route)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/api/users", nil)
	req.Host = "app.local"
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body, _ := io.ReadAll(rec.Body)
	if strings.TrimSpace(string(body)) != "api-backend" {
		t.Fatalf("body = %q, want %q", string(body), "api-backend")
	}
}

func TestProxyHandlerPathRouteRequiresBoundaryMatch(t *testing.T) {
	defaultBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("default-backend"))
	}))
	defer defaultBackend.Close()

	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("api-backend"))
	}))
	defer apiBackend.Close()

	fullConfig := &config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{
			{Id: "default", URL: defaultBackend.URL, Weight: 1, Enabled: true},
			{Id: "api", URL: apiBackend.URL, Weight: 1, Enabled: true},
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
		VirtualHosts: []config.VirtualHost{
			{
				Domain:           "app.local",
				BackendIDs:       []string{"default"},
				SecurityPolicyID: "default",
				PathRoutes: []config.PathRoute{
					{Path: "/api", BackendIDs: []string{"api"}},
				},
			},
		},
	}

	rt := newTestRuntime(t, fullConfig)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/apix", nil)
	req.Host = "app.local"
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body, _ := io.ReadAll(rec.Body)
	if strings.TrimSpace(string(body)) != "default-backend" {
		t.Fatalf("body = %q, want %q", string(body), "default-backend")
	}
}

func TestProxyHandlerStripsPathPrefixBeforeForwarding(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("prefix-stripped"))
	}))
	defer backend.Close()

	route := &config.PathRoute{
		Path:        "/api",
		BackendIDs:  []string{"backend"},
		StripPrefix: true,
	}

	rt := newSingleRouteTestRuntime(t, "app.local", backend.URL, route)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/api/users", nil)
	req.Host = "app.local"
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if gotPath != "/users" {
		t.Fatalf("backend path = %q, want %q", gotPath, "/users")
	}
}

func TestProxyHandlerPrefersLongestMatchingPathRoute(t *testing.T) {
	var gotBody string
	apiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("generic-api"))
	}))
	defer apiBackend.Close()

	v1Backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("api-v1"))
	}))
	defer v1Backend.Close()

	fullConfig := &config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{
			{Id: "api", URL: apiBackend.URL, Weight: 1, Enabled: true},
			{Id: "api-v1", URL: v1Backend.URL, Weight: 1, Enabled: true},
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
		VirtualHosts: []config.VirtualHost{
			{
				Domain:           "app.local",
				BackendIDs:       []string{"api"},
				SecurityPolicyID: "default",
				PathRoutes: []config.PathRoute{
					{Path: "/api", BackendIDs: []string{"api"}},
					{Path: "/api/v1", BackendIDs: []string{"api-v1"}},
				},
			},
		},
	}

	rt := newTestRuntime(t, fullConfig)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/api/v1/users", nil)
	req.Host = "app.local"
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body, _ := io.ReadAll(rec.Body)
	gotBody = strings.TrimSpace(string(body))
	if gotBody != "api-v1" {
		t.Fatalf("body = %q, want %q", gotBody, "api-v1")
	}
}

func TestProxyHandlerPreservesIncomingRequestID(t *testing.T) {
	var gotRequestID string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get("X-Request-ID")
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	rt := newSingleRouteTestRuntime(t, "app.local", backend.URL, nil)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/", nil)
	req.Host = "app.local"
	req.Header.Set("X-Request-ID", "req-123")
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if gotRequestID != "req-123" {
		t.Fatalf("backend X-Request-ID = %q, want %q", gotRequestID, "req-123")
	}
	if rec.Header().Get("X-Request-ID") != "req-123" {
		t.Fatalf("response X-Request-ID = %q, want %q", rec.Header().Get("X-Request-ID"), "req-123")
	}
}

func TestProxyHandlerRetriesIdempotentRequestOnAlternateBackend(t *testing.T) {
	healthyBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("healthy"))
	}))
	defer healthyBackend.Close()

	fullConfig := &config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{
			{Id: "dead", URL: "http://127.0.0.1:1", Weight: 1, Enabled: true},
			{Id: "healthy", URL: healthyBackend.URL, Weight: 1, Enabled: true},
		},
		HealthCheck: config.HealthCheckConfig{
			Path:             "/health",
			IntervalSeconds:  1,
			TimeoutSeconds:   1,
			HealthyThreshold: 1,
			SuccessCodes:     []int32{200},
		},
		Timeouts: config.TimeoutsConfig{
			ConnectTimeoutMs:   200,
			ResponseTimeoutMs:  1000,
			KeepAliveTimeoutMs: 1000,
			IdleConnTimeoutMs:  1000,
		},
		VirtualHosts: []config.VirtualHost{
			{
				Domain:           "app.local",
				BackendIDs:       []string{"dead", "healthy"},
				SecurityPolicyID: "default",
			},
		},
	}

	rt := newTestRuntime(t, fullConfig)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/", nil)
	req.Host = "app.local"
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body, _ := io.ReadAll(rec.Body)
	if strings.TrimSpace(string(body)) != "healthy" {
		t.Fatalf("body = %q, want %q", string(body), "healthy")
	}
}
