package grpc

import (
	"context"
	"edge-proxy/internal/api/adminpb"
	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy/runtime"
	"os"
	"path/filepath"
	"testing"
)

const (
	backend1URL = "http://backend-1:3000"
	backend2URL = "http://backend-2:3000"
	backend3URL = "http://backend-3:3000"
)

func TestBackendHandlersAddGetUpdateRemove(t *testing.T) {
	srv, rt := newTestAdminServer(t)

	resp, err := srv.AddBackend(context.Background(), &adminpb.AddBackendRequest{
		Url:    backend3URL,
		Weight: 3,
	})
	requireSuccess(t, resp, err)

	got, err := srv.GetBackend(context.Background(), &adminpb.GetBackendRequest{Url: backend3URL})
	if err != nil {
		t.Fatalf("GetBackend returned error: %v", err)
	}
	if got.Url != backend3URL {
		t.Fatalf("backend url = %q, want %q", got.Url, backend3URL)
	}
	if got.Weight != 3 {
		t.Fatalf("backend weight = %d, want 3", got.Weight)
	}
	if !got.Enabled {
		t.Fatal("new backend should be enabled")
	}
	if rt.SnapshotView().BackendsByURL[backend3URL] == nil {
		t.Fatal("new backend was not added to runtime snapshot")
	}

	resp, err = srv.UpdateBackend(context.Background(), &adminpb.UpdateBackendRequest{
		Url:     backend3URL,
		Weight:  7,
		Enabled: false,
	})
	requireSuccess(t, resp, err)

	got, err = srv.GetBackend(context.Background(), &adminpb.GetBackendRequest{Url: backend3URL})
	if err != nil {
		t.Fatalf("GetBackend after update returned error: %v", err)
	}
	if got.Weight != 7 {
		t.Fatalf("updated backend weight = %d, want 7", got.Weight)
	}
	if got.Enabled {
		t.Fatal("updated backend should be disabled")
	}

	resp, err = srv.RemoveBackend(context.Background(), &adminpb.RemoveBackendRequest{Url: backend3URL})
	requireSuccess(t, resp, err)

	if _, err := srv.GetBackend(context.Background(), &adminpb.GetBackendRequest{Url: backend3URL}); err == nil {
		t.Fatal("GetBackend after remove returned nil error, want not found error")
	}
	if rt.SnapshotView().BackendsByURL[backend3URL] != nil {
		t.Fatal("removed backend is still present in runtime snapshot")
	}
}

func TestBackendHandlersRejectInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		call func(*AdminGRPCServer) (*adminpb.BasicResponse, error)
	}{
		{
			name: "add backend rejects empty url",
			call: func(srv *AdminGRPCServer) (*adminpb.BasicResponse, error) {
				return srv.AddBackend(context.Background(), &adminpb.AddBackendRequest{Weight: 1})
			},
		},
		{
			name: "add backend rejects duplicate url",
			call: func(srv *AdminGRPCServer) (*adminpb.BasicResponse, error) {
				return srv.AddBackend(context.Background(), &adminpb.AddBackendRequest{Url: backend1URL, Weight: 1})
			},
		},
		{
			name: "add backend rejects zero weight",
			call: func(srv *AdminGRPCServer) (*adminpb.BasicResponse, error) {
				return srv.AddBackend(context.Background(), &adminpb.AddBackendRequest{Url: backend3URL})
			},
		},
		{
			name: "update backend rejects missing url",
			call: func(srv *AdminGRPCServer) (*adminpb.BasicResponse, error) {
				return srv.UpdateBackend(context.Background(), &adminpb.UpdateBackendRequest{Url: "http://missing:3000", Weight: 1, Enabled: true})
			},
		},
		{
			name: "remove backend rejects missing url",
			call: func(srv *AdminGRPCServer) (*adminpb.BasicResponse, error) {
				return srv.RemoveBackend(context.Background(), &adminpb.RemoveBackendRequest{Url: "http://missing:3000"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newTestAdminServer(t)
			resp, err := tt.call(srv)
			requireFailure(t, resp, err)
		})
	}
}

func TestRemoveBackendDropsReferencesFromVirtualHosts(t *testing.T) {
	srv, _ := newTestAdminServer(t)

	resp, err := srv.RemoveBackend(context.Background(), &adminpb.RemoveBackendRequest{Url: backend2URL})
	requireSuccess(t, resp, err)

	vhost, err := srv.GetVirtualHost(context.Background(), &adminpb.GetVirtualHostRequest{Domain: "app.local"})
	if err != nil {
		t.Fatalf("GetVirtualHost returned error: %v", err)
	}
	if containsString(vhost.Backends, backend2URL) {
		t.Fatalf("virtual host backends still contain removed backend %q", backend2URL)
	}

	route := findPathRoute(vhost.PathRoutes, "/api")
	if route == nil {
		t.Fatal("expected /api path route to remain after backend removal")
	}
	if containsString(route.Backends, backend2URL) {
		t.Fatalf("path route backends still contain removed backend %q", backend2URL)
	}
}

func TestVirtualHostHandlersAddGetUpdateRemove(t *testing.T) {
	srv, _ := newTestAdminServer(t)

	resp, err := srv.AddVirtualHost(context.Background(), &adminpb.AddVirtualHostRequest{
		Vhost: &adminpb.VirtualHost{
			Domain:   "api.local",
			Backends: []string{backend1URL},
		},
	})
	requireSuccess(t, resp, err)

	got, err := srv.GetVirtualHost(context.Background(), &adminpb.GetVirtualHostRequest{Domain: "api.local"})
	if err != nil {
		t.Fatalf("GetVirtualHost returned error: %v", err)
	}
	if got.Domain != "api.local" {
		t.Fatalf("virtual host domain = %q, want api.local", got.Domain)
	}
	if !containsString(got.Backends, backend1URL) {
		t.Fatalf("virtual host backends = %v, want %q", got.Backends, backend1URL)
	}

	resp, err = srv.UpdateVirtualHost(context.Background(), &adminpb.UpdateVirtualHostRequest{
		Domain: "api.local",
		Vhost: &adminpb.VirtualHost{
			Domain:   "ignored-by-runtime.local",
			Backends: []string{backend2URL},
			PathRoutes: []*adminpb.PathRoute{
				{Path: "/v1", Backends: []string{backend2URL}, StripPrefix: true},
			},
			SecurityConfig: &adminpb.SecurityConfigResponse{
				RateLimiting: &adminpb.RateLimitingConfig{
					Enabled:   true,
					RatePerIp: 10,
					Burst:     5,
					WindowSec: 60,
				},
			},
		},
	})
	requireSuccess(t, resp, err)

	got, err = srv.GetVirtualHost(context.Background(), &adminpb.GetVirtualHostRequest{Domain: "api.local"})
	if err != nil {
		t.Fatalf("GetVirtualHost after update returned error: %v", err)
	}
	if got.Domain != "api.local" {
		t.Fatalf("updated virtual host domain = %q, want api.local", got.Domain)
	}
	if !containsString(got.Backends, backend2URL) {
		t.Fatalf("updated virtual host backends = %v, want %q", got.Backends, backend2URL)
	}
	route := findPathRoute(got.PathRoutes, "/v1")
	if route == nil {
		t.Fatal("expected updated /v1 path route")
	}
	if !route.StripPrefix {
		t.Fatal("updated path route should strip prefix")
	}
	if got.SecurityConfig == nil || got.SecurityConfig.RateLimiting == nil || !got.SecurityConfig.RateLimiting.Enabled {
		t.Fatal("updated virtual host should include enabled rate limiting config")
	}

	resp, err = srv.RemoveVirtualHost(context.Background(), &adminpb.RemoveVirtualHostRequest{Domain: "api.local"})
	requireSuccess(t, resp, err)

	if _, err := srv.GetVirtualHost(context.Background(), &adminpb.GetVirtualHostRequest{Domain: "api.local"}); err == nil {
		t.Fatal("GetVirtualHost after remove returned nil error, want not found error")
	}
}

func TestVirtualHostHandlersRejectUnknownBackend(t *testing.T) {
	srv, _ := newTestAdminServer(t)

	resp, err := srv.AddVirtualHost(context.Background(), &adminpb.AddVirtualHostRequest{
		Vhost: &adminpb.VirtualHost{
			Domain:   "missing-backend.local",
			Backends: []string{"http://missing:3000"},
		},
	})
	requireFailure(t, resp, err)
}

func TestSecurityHandlersUpdateAndGet(t *testing.T) {
	srv, _ := newTestAdminServer(t)

	resp, err := srv.UpdateVirtualHostSecurityConfig(context.Background(), &adminpb.UpdateSecurityConfigRequest{
		Domain: "app.local",
		Config: &adminpb.SecurityConfigResponse{
			RateLimiting: &adminpb.RateLimitingConfig{
				Enabled:   true,
				RatePerIp: 25,
				Burst:     10,
				WindowSec: 30,
			},
		},
	})
	requireSuccess(t, resp, err)

	got, err := srv.GetVirtualHostSecurityConfig(context.Background(), &adminpb.GetVirtualHostRequest{Domain: "app.local"})
	if err != nil {
		t.Fatalf("GetVirtualHostSecurityConfig returned error: %v", err)
	}
	if got.RateLimiting == nil {
		t.Fatal("rate limiting config = nil")
	}
	if !got.RateLimiting.Enabled || got.RateLimiting.RatePerIp != 25 || got.RateLimiting.Burst != 10 || got.RateLimiting.WindowSec != 30 {
		t.Fatalf("rate limiting config = %+v, want enabled 25/10/30", got.RateLimiting)
	}
}

func TestGlobalConfigHandlersGetSet(t *testing.T) {
	srv, _ := newTestAdminServer(t)

	got, err := srv.GetGlobalConfig(context.Background(), &adminpb.Empty{})
	if err != nil {
		t.Fatalf("GetGlobalConfig returned error: %v", err)
	}
	if got.ProxyPort != 8080 {
		t.Fatalf("proxy port = %d, want 8080", got.ProxyPort)
	}
	if got.LbStrategy != "least-connections" {
		t.Fatalf("lb strategy = %q, want least-connections", got.LbStrategy)
	}

	resp, err := srv.SetGlobalConfig(context.Background(), &adminpb.GlobalConfig{
		ProxyPort:  9090,
		LbStrategy: "least-connections",
	})
	requireSuccess(t, resp, err)

	got, err = srv.GetGlobalConfig(context.Background(), &adminpb.Empty{})
	if err != nil {
		t.Fatalf("GetGlobalConfig after update returned error: %v", err)
	}
	if got.ProxyPort != 9090 {
		t.Fatalf("updated proxy port = %d, want 9090", got.ProxyPort)
	}

	resp, err = srv.SetGlobalConfig(context.Background(), &adminpb.GlobalConfig{
		ProxyPort:  70000,
		LbStrategy: "least-connections",
	})
	requireFailure(t, resp, err)
}

func newTestAdminServer(t *testing.T) (*AdminGRPCServer, *runtime.Runtime) {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(testConfigJSON), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	rt, err := runtime.NewRuntime(configPath)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	rt.SetOnRateLimitUpdate(func(string, config.RateLimitingConfig) {})

	return &AdminGRPCServer{Runtime: rt}, rt
}

func requireSuccess(t *testing.T, resp *adminpb.BasicResponse, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("response = nil")
	}
	if !resp.Success {
		t.Fatalf("response success = false, error = %q", resp.Error)
	}
}

func requireFailure(t *testing.T, resp *adminpb.BasicResponse, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if resp == nil {
		t.Fatal("response = nil")
	}
	if resp.Success {
		t.Fatalf("response success = true, want false")
	}
	if resp.Error == "" {
		t.Fatal("response error is empty")
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func findPathRoute(routes []*adminpb.PathRoute, path string) *adminpb.PathRoute {
	for _, route := range routes {
		if route.Path == path {
			return route
		}
	}
	return nil
}

const testConfigJSON = `{
  "proxy_port": 8080,
  "lb_strategy": "least-connections",
  "backends": [
    { "url": "http://backend-1:3000", "weight": 1, "enabled": true },
    { "url": "http://backend-2:3000", "weight": 2, "enabled": true }
  ],
  "health_check": {
    "path": "/health",
    "interval_seconds": 1,
    "timeout_seconds": 1,
    "healthy_threshold": 1,
    "success_codes": [200]
  },
  "timeouts": {
    "connect_timeout_ms": 1000,
    "response_timeout_ms": 1000,
    "keep_alive_timeout_ms": 1000,
    "idle_conn_timeout_ms": 1000
  },
  "logging": {
    "enabled": true,
    "level": "error",
    "async": false,
    "buffer_size": 4096
  },
  "virtual_hosts": [
    {
      "domain": "app.local",
      "backends": ["http://backend-1:3000", "http://backend-2:3000"],
      "path_routes": [
        {
          "path": "/api",
          "backends": ["http://backend-2:3000"],
          "strip_prefix": true
        }
      ],
      "security": {
        "rate_limiting": {
          "enabled": false,
          "rate_per_ip": 100,
          "burst": 50,
          "window_sec": 60
        }
      }
    }
  ]
}`
