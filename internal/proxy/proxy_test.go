package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ktrubilo9/edge-proxy/internal/config"
	healthview "github.com/ktrubilo9/edge-proxy/internal/health"
	"github.com/ktrubilo9/edge-proxy/internal/middleware"
	"github.com/ktrubilo9/edge-proxy/internal/proxy/runtime"
	"github.com/ktrubilo9/edge-proxy/internal/testutil"
)

const (
	testBackendID  = "backend"
	testBackendURL = "http://backend.internal:3000"
	testHost       = "app.local"
)

// newTestProxy builds a Proxy that is wired exactly like a started one, except
// that no listener is opened and no server is started. This keeps the handler
// tests free of fixed ports such as 8080 or 9091.
func newTestProxy(t *testing.T, fullConfig *config.FullConfig) *Proxy {
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
			t.Fatalf("missing backend status for %s", backend.Id)
		}
		status.ApplyProbeResult(true, nil, config.HealthThresholdConfig{Healthy: 1, Unhealthy: 1}, time.Now())
	}

	clientIPs, err := middleware.NewClientIPResolver("")
	if err != nil {
		t.Fatalf("create client IP resolver: %v", err)
	}

	return &Proxy{
		Runtime:             rt,
		clientIPs:           clientIPs,
		ratelimitMiddleware: middleware.NewRateLimitMiddleware(rt, clientIPs),
	}
}

func newSingleRouteProxy(t *testing.T, backendURL string) *Proxy {
	t.Helper()

	return newTestProxy(t, &config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{
			{Id: testBackendID, URL: backendURL, Weight: 1, Enabled: true},
		},
		HealthCheck: testutil.DefaultHealthCheckConfig(),
		Timeouts: config.TimeoutsConfig{
			ConnectTimeoutMs:   1000,
			ResponseTimeoutMs:  1000,
			KeepAliveTimeoutMs: 1000,
			IdleConnTimeoutMs:  1000,
		},
		VirtualHosts: []config.VirtualHost{
			{
				Domain:           testHost,
				BackendIDs:       []string{testBackendID},
				SecurityPolicyID: "default",
			},
		},
	})
}

// recordingBackend is an httptest server that records the paths it received, so
// tests can prove that a request was never forwarded to a backend.
type recordingBackend struct {
	server *httptest.Server

	mu    sync.Mutex
	paths []string
}

func newRecordingBackend(t *testing.T, response string) *recordingBackend {
	t.Helper()

	backend := &recordingBackend{}
	backend.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend.mu.Lock()
		backend.paths = append(backend.paths, r.URL.Path)
		backend.mu.Unlock()
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(backend.server.Close)

	return backend
}

func (b *recordingBackend) URL() string {
	return b.server.URL
}

func (b *recordingBackend) receivedPaths() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.paths...)
}

func serve(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = testHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestPublicHandlerServesProxyTraffic(t *testing.T) {
	backend := newRecordingBackend(t, "backend-ok")
	proxy := newSingleRouteProxy(t, backend.URL())

	rec := serve(proxy.publicHandler(), http.MethodGet, "http://"+testHost+"/api/items")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "backend-ok" {
		t.Fatalf("body = %q, want %q", body, "backend-ok")
	}

	paths := backend.receivedPaths()
	if len(paths) != 1 || paths[0] != "/api/items" {
		t.Fatalf("backend paths = %v, want [/api/items]", paths)
	}
}

func TestPublicHandlerHealthReturnsSmallResponse(t *testing.T) {
	proxy := newSingleRouteProxy(t, testBackendURL)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			rec := serve(proxy.publicHandler(), method, "http://"+testHost+"/health")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", contentType)
			}
			if body := strings.TrimSpace(rec.Body.String()); body != `{"status":"healthy"}` {
				t.Fatalf("body = %q, want %q", body, `{"status":"healthy"}`)
			}
		})
	}
}

func TestPublicHandlerHealthDoesNotExposeBackendDetails(t *testing.T) {
	proxy := newSingleRouteProxy(t, testBackendURL)

	rec := serve(proxy.publicHandler(), http.MethodGet, "http://"+testHost+"/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if strings.Contains(body, testBackendURL) {
		t.Fatalf("public health response exposed backend URL: %s", body)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("public health keys = %v, want only status", payload)
	}
	if _, ok := payload["status"]; !ok {
		t.Fatalf("public health response missing status key: %s", body)
	}
	for _, detailed := range []string{"backends", "active_count", "total_count"} {
		if _, ok := payload[detailed]; ok {
			t.Fatalf("public health response exposed %q: %s", detailed, body)
		}
	}
}

func TestPublicHandlerHealthRejectsUnsupportedMethods(t *testing.T) {
	backend := newRecordingBackend(t, "backend-ok")
	proxy := newSingleRouteProxy(t, backend.URL())

	for _, method := range []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodTrace,
	} {
		t.Run(method, func(t *testing.T) {
			rec := serve(proxy.publicHandler(), method, "http://"+testHost+"/health")

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
			}
			if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
				t.Fatalf("Allow = %q, want %q", allow, "GET, HEAD")
			}
		})
	}

	// The reserved /health path must never fall through to the catch-all proxy
	// route, otherwise an unsupported method would be forwarded to a backend.
	if paths := backend.receivedPaths(); len(paths) != 0 {
		t.Fatalf("backend received %v, want no requests", paths)
	}
}

func TestPublicHandlerDoesNotServeMetrics(t *testing.T) {
	backend := newRecordingBackend(t, "backend-ok")
	proxy := newSingleRouteProxy(t, backend.URL())

	metricPaths := []string{"/metrics", "/metrics/prometheus"}
	for _, path := range metricPaths {
		for _, method := range []string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPost,
			http.MethodDelete,
		} {
			t.Run(method+" "+path, func(t *testing.T) {
				rec := serve(proxy.publicHandler(), method, "http://"+testHost+path)

				if rec.Code != http.StatusNotFound {
					t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
				}
			})
		}
	}

	if paths := backend.receivedPaths(); len(paths) != 0 {
		t.Fatalf("backend received %v, want no requests", paths)
	}
}

func TestOperationalHandlerHealthReturnsDetailedResponse(t *testing.T) {
	proxy := newSingleRouteProxy(t, testBackendURL)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			rec := serve(proxy.operationalHandler(), method, "http://ops.local/health")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", contentType)
			}
		})
	}

	rec := serve(proxy.operationalHandler(), http.MethodGet, "http://ops.local/health")

	var payload healthview.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "healthy" {
		t.Fatalf("status = %q, want %q", payload.Status, "healthy")
	}
	if payload.ActiveCount != 1 || payload.TotalCount != 1 {
		t.Fatalf("active/total = %d/%d, want 1/1", payload.ActiveCount, payload.TotalCount)
	}
	if len(payload.Backends) != 1 {
		t.Fatalf("backend count = %d, want 1", len(payload.Backends))
	}
	if payload.Backends[0].URL != testBackendURL {
		t.Fatalf("backend URL = %q, want %q", payload.Backends[0].URL, testBackendURL)
	}
}

func TestOperationalHandlerMetricsReturnsJSON(t *testing.T) {
	proxy := newSingleRouteProxy(t, testBackendURL)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			rec := serve(proxy.operationalHandler(), method, "http://ops.local/metrics")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", contentType)
			}
		})
	}

	rec := serve(proxy.operationalHandler(), http.MethodGet, "http://ops.local/metrics")

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, key := range []string{"system", "backends", "rate_limit", "http"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("metrics response missing %q key: %s", key, rec.Body.String())
		}
	}
}

func TestOperationalHandlerPrometheusMetrics(t *testing.T) {
	proxy := newSingleRouteProxy(t, testBackendURL)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			rec := serve(proxy.operationalHandler(), method, "http://ops.local/metrics/prometheus")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
				t.Fatalf("Content-Type = %q, want a Prometheus text format", contentType)
			}
		})
	}

	rec := serve(proxy.operationalHandler(), http.MethodGet, "http://ops.local/metrics/prometheus")

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !strings.Contains(string(body), "reverse_proxy_requests_total") {
		t.Fatalf("prometheus response missing reverse_proxy_requests_total:\n%s", body)
	}
}

func TestOperationalHandlerRejectsUnsupportedMethods(t *testing.T) {
	proxy := newSingleRouteProxy(t, testBackendURL)

	for _, path := range []string{"/health", "/metrics", "/metrics/prometheus"} {
		for _, method := range []string{
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
			http.MethodTrace,
		} {
			t.Run(method+" "+path, func(t *testing.T) {
				rec := serve(proxy.operationalHandler(), method, "http://ops.local"+path)

				if rec.Code != http.StatusMethodNotAllowed {
					t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
				}
				if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
					t.Fatalf("Allow = %q, want %q", allow, "GET, HEAD")
				}
			})
		}
	}
}

func TestOperationalHandlerHealthReportsUnavailable(t *testing.T) {
	proxy := newSingleRouteProxy(t, testBackendURL)

	status, ok := proxy.Runtime.State().BackendStatus(testBackendID)
	if !ok {
		t.Fatal("missing backend status")
	}
	status.ApplyProbeResult(
		false,
		errors.New("forced unavailable for test"),
		config.HealthThresholdConfig{Healthy: 1, Unhealthy: 1},
		time.Now(),
	)

	rec := serve(proxy.operationalHandler(), http.MethodGet, "http://ops.local/health")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
