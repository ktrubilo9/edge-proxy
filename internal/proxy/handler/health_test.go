package handler

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/metrics"
	"edge-proxy/internal/proxy/runtime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicHealthHandlerDoesNotExposeBackendDetails(t *testing.T) {
	backendURL := "http://backend.internal:3000"
	rt := &runtime.Runtime{
		Snapshot: config.BuildSnapshot(&config.FullConfig{
			Backends: []*config.BackendConfig{
				{URL: backendURL, Enabled: true},
			},
		}),
		BackendStatus: map[string]*runtime.BackendStatus{
			backendURL: {},
		},
		Metrics: metrics.NewMetrics(),
	}
	rt.BackendStatus[backendURL].Active.Store(true)

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
	rt := &runtime.Runtime{
		Snapshot:      config.BuildSnapshot(&config.FullConfig{}),
		BackendStatus: map[string]*runtime.BackendStatus{},
		Metrics:       metrics.NewMetrics(),
	}

	req := httptest.NewRequest(http.MethodGet, "http://app.local/health", nil)
	rec := httptest.NewRecorder()

	PublicHealthHandler(rt).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
