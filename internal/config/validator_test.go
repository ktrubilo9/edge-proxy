package config

import (
	"strings"
	"testing"
)

func validTestConfig() *FullConfig {
	return &FullConfig{
		ProxyPort:  8080,
		LBStrategy: "least-connections",
		Backends: []*BackendConfig{
			{URL: "http://backend-1", Weight: 1, Enabled: true},
			{URL: "http://backend-2", Weight: 1, Enabled: true},
		},
		HealthCheck: HealthCheckConfig{
			Path:             "/health",
			IntervalSeconds:  1,
			TimeoutSeconds:   1,
			HealthyThreshold: 2,
			SuccessCodes:     []int32{200},
		},
		Timeouts: TimeoutsConfig{
			ConnectTimeoutMs:   1000,
			ResponseTimeoutMs:  1000,
			KeepAliveTimeoutMs: 1000,
			IdleConnTimeoutMs:  1000,
		},
		VirtualHosts: []VirtualHost{
			{
				Domain:   "app.local",
				Backends: []string{"http://backend-1"},
				Security: &SecurityConfig{},
				PathRoutes: []PathRoute{
					{Path: "/api", Backends: []string{"http://backend-2"}},
				},
			},
		},
	}
}

func TestValidateConfigRejectsUnknownVirtualHostBackend(t *testing.T) {
	cfg := validTestConfig()
	cfg.VirtualHosts[0].Backends = []string{"http://missing-backend"}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "references unknown backend") {
		t.Fatalf("error = %q, want unknown backend validation message", err.Error())
	}
}

func TestValidateConfigRejectsUnknownPathRouteBackend(t *testing.T) {
	cfg := validTestConfig()
	cfg.VirtualHosts[0].PathRoutes[0].Backends = []string{"http://missing-backend"}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "references unknown backend") {
		t.Fatalf("error = %q, want unknown backend validation message", err.Error())
	}
}

func TestValidateConfigRejectsEmptyPathRoutePath(t *testing.T) {
	cfg := validTestConfig()
	cfg.VirtualHosts[0].PathRoutes[0].Path = ""

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "path route path cannot be empty") {
		t.Fatalf("error = %q, want empty path route validation message", err.Error())
	}
}
