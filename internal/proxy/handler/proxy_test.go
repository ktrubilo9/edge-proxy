package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ktrubilo9/edge-proxy/internal/config"
	"github.com/ktrubilo9/edge-proxy/internal/lb"
	"github.com/ktrubilo9/edge-proxy/internal/proxy/runtime"
	"github.com/ktrubilo9/edge-proxy/internal/testutil"
)

type stubLoadBalancer struct {
	backend *config.BackendConfig
	err     error
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func (s stubLoadBalancer) Next(_ []*config.BackendConfig) (*config.BackendConfig, error) {
	return s.backend, s.err
}

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
		status.ApplyProbeResult(true, nil, config.HealthThresholdConfig{Healthy: 1, Unhealthy: 1}, time.Now())
	}

	return rt
}

func newSingleRouteTestRuntime(t *testing.T, host string, backendURL string, route *config.PathRoute) *runtime.Runtime {
	t.Helper()
	healthConfig := testutil.DefaultHealthCheckConfig()
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
		HealthCheck: healthConfig,
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
	status.ApplyProbeResult(false, errors.New("forced inactive for test"), config.HealthThresholdConfig{Healthy: 1, Unhealthy: 1}, time.Now())

	req := httptest.NewRequest(http.MethodGet, "http://unknown.local/", nil)
	req.Host = "unknown.local"
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestProxyHandlerRejectsHTTPUpgradeWithoutContactingBackend(t *testing.T) {
	var backendRequests atomic.Uint64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		backendRequests.Add(1)
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer backend.Close()

	rt := newSingleRouteTestRuntime(t, "app.local", backend.URL, nil)

	tests := []struct {
		name       string
		connection []string
		upgrade    string
	}{
		{
			name:       "websocket",
			connection: []string{"keep-alive, Upgrade"},
			upgrade:    "websocket",
		},
		{
			name:       "h2c with multiple connection values",
			connection: []string{"keep-alive", "uPgRaDe"},
			upgrade:    "h2c",
		},
		{
			name:    "upgrade header without connection token",
			upgrade: "websocket",
		},
		{
			name:       "connection token without upgrade header",
			connection: []string{"upgrade"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://app.local/", nil)
			req.Host = "app.local"
			for _, value := range tt.connection {
				req.Header.Add("Connection", value)
			}
			if tt.upgrade != "" {
				req.Header.Set("Upgrade", tt.upgrade)
			}
			rec := httptest.NewRecorder()

			ProxyHandler(rt).ServeHTTP(rec, req)

			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
			}
			if body := strings.TrimSpace(rec.Body.String()); body != "HTTP protocol upgrades are not supported" {
				t.Fatalf("body = %q, want upgrade rejection message", body)
			}
			if requestID := rec.Header().Get("X-Request-ID"); requestID == "" {
				t.Fatal("expected rejected request to keep X-Request-ID")
			}
		})
	}

	if got := backendRequests.Load(); got != 0 {
		t.Fatalf("backend requests = %d, want 0", got)
	}
}

func TestProxyHandlerHandlesLoadBalancerSelectionFailure(t *testing.T) {
	tests := []struct {
		name       string
		balancer   stubLoadBalancer
		wantStatus int
		wantBody   string
	}{
		{
			name:       "no available backend",
			balancer:   stubLoadBalancer{err: lb.ErrNoAvailableBackend},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "No available backend",
		},
		{
			name:       "nil backend without error",
			balancer:   stubLoadBalancer{},
			wantStatus: http.StatusBadGateway,
			wantBody:   "Backend error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newSingleRouteTestRuntime(
				t,
				"app.local",
				"http://127.0.0.1:1",
				nil,
			)
			rt.State().LoadBalancer = tt.balancer

			req := httptest.NewRequest(http.MethodGet, "http://app.local/", nil)
			req.Host = "app.local"
			rec := httptest.NewRecorder()

			ProxyHandler(rt).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if body := strings.TrimSpace(rec.Body.String()); body != tt.wantBody {
				t.Fatalf("body = %q, want %q", body, tt.wantBody)
			}
		})
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

func TestProxyHandlerFiltersHopByHopRequestHeaders(t *testing.T) {
	rt := newSingleRouteTestRuntime(t, "app.local", "http://backend.internal:3000", nil)

	type capturedRequest struct {
		header           http.Header
		body             string
		close            bool
		transferEncoding []string
		trailer          http.Header
		requestURI       string
	}
	var captured capturedRequest
	rt.State().HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read upstream request body: %v", err)
		}
		captured = capturedRequest{
			header:           req.Header.Clone(),
			body:             string(body),
			close:            req.Close,
			transferEncoding: append([]string(nil), req.TransferEncoding...),
			trailer:          req.Trailer.Clone(),
			requestURI:       req.RequestURI,
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("backend-ok")),
			Request:    req,
		}, nil
	})

	req := httptest.NewRequest(http.MethodPost, "http://app.local/items", strings.NewReader("payload"))
	req.Host = "app.local"
	req.RemoteAddr = "192.0.2.10:4321"
	req.Header.Set("X-Request-ID", "request-123")
	req.Header.Set("X-Forwarded-For", "198.51.100.25")
	req.Header.Set("X-End-To-End", "preserved")
	req.Header["cOnNeCtIoN"] = []string{
		"Keep-Alive, X-Temporary-State",
		"X-Request-ID, X-Forwarded-For, X-Forwarded-Host, X-Forwarded-Proto",
	}
	req.Header["kEeP-aLiVe"] = []string{"timeout=5"}
	req.Header.Set("Proxy-Authenticate", "Basic")
	req.Header.Set("Proxy-Authorization", "Basic credentials")
	req.Header.Set("Proxy-Connection", "keep-alive")
	req.Header.Set("TE", "trailers")
	req.Header.Set("Trailer", "X-Request-Trailer")
	req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("X-Temporary-State", "remove-me")
	req.Close = true
	req.TransferEncoding = []string{"chunked"}
	req.Trailer = http.Header{"X-Request-Trailer": {"checksum"}}

	originalHeader := req.Header.Clone()
	originalTransferEncoding := append([]string(nil), req.TransferEncoding...)
	originalTrailer := req.Trailer.Clone()
	rec := httptest.NewRecorder()

	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	for _, name := range []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Proxy-Connection",
		"TE",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
		"X-Temporary-State",
	} {
		if headerContains(captured.header, name) {
			t.Errorf("upstream request contains hop-by-hop header %q: %v", name, captured.header)
		}
	}
	if got := captured.header.Get("X-End-To-End"); got != "preserved" {
		t.Errorf("X-End-To-End = %q, want preserved", got)
	}
	for name, want := range map[string]string{
		"X-Request-ID":      "request-123",
		"X-Forwarded-For":   "198.51.100.25",
		"X-Forwarded-Host":  "app.local",
		"X-Forwarded-Proto": "http",
	} {
		if got := captured.header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if captured.body != "payload" {
		t.Errorf("upstream body = %q, want payload", captured.body)
	}
	if captured.close {
		t.Error("upstream request kept Close=true")
	}
	if len(captured.transferEncoding) != 0 {
		t.Errorf("upstream TransferEncoding = %v, want none", captured.transferEncoding)
	}
	if len(captured.trailer) != 0 {
		t.Errorf("upstream Trailer = %v, want none", captured.trailer)
	}
	if captured.requestURI != "" {
		t.Errorf("upstream RequestURI = %q, want empty", captured.requestURI)
	}

	if !reflect.DeepEqual(req.Header, originalHeader) {
		t.Errorf("original headers changed:\ngot  %v\nwant %v", req.Header, originalHeader)
	}
	if !req.Close {
		t.Error("original request Close was changed")
	}
	if !reflect.DeepEqual(req.TransferEncoding, originalTransferEncoding) {
		t.Errorf("original TransferEncoding = %v, want %v", req.TransferEncoding, originalTransferEncoding)
	}
	if !reflect.DeepEqual(req.Trailer, originalTrailer) {
		t.Errorf("original Trailer = %v, want %v", req.Trailer, originalTrailer)
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

	healthConfig := testutil.DefaultHealthCheckConfig()
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
		HealthCheck: healthConfig,
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

	healthConfig := testutil.DefaultHealthCheckConfig()
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
		HealthCheck: healthConfig,
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

	healthConfig := testutil.DefaultHealthCheckConfig()
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
		HealthCheck: healthConfig,
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

func TestProxyHandlerPassiveHealthDeactivatesFailingBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer backend.Close()

	healthConfig := testutil.DefaultHealthCheckConfig()
	healthConfig.Passive.Enabled = true
	healthConfig.Thresholds.Unhealthy = 1
	fullConfig := &config.FullConfig{
		Server:       config.ServerConfig{ProxyPort: 8080, AdminGrpcPort: 50051},
		LoadBalancer: config.LoadBalancingConfig{Strategy: "least-connections"},
		Backends: []*config.BackendConfig{
			{Id: "backend", URL: backend.URL, Weight: 1, Enabled: true},
		},
		HealthCheck: healthConfig,
		Timeouts: config.TimeoutsConfig{
			ConnectTimeoutMs:   1000,
			ResponseTimeoutMs:  1000,
			KeepAliveTimeoutMs: 1000,
			IdleConnTimeoutMs:  1000,
		},
		VirtualHosts: []config.VirtualHost{
			{Domain: "app.local", BackendIDs: []string{"backend"}, SecurityPolicyID: "default"},
		},
	}
	rt := newTestRuntime(t, fullConfig)

	req := httptest.NewRequest(http.MethodGet, "http://app.local/", nil)
	req.Host = "app.local"
	rec := httptest.NewRecorder()
	ProxyHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	status, ok := rt.State().BackendStatus("backend")
	if !ok {
		t.Fatal("missing backend status")
	}
	if status.IsActive() {
		t.Fatal("passive health did not deactivate failing backend")
	}
}
