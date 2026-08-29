package config

import (
	"strings"
	"testing"
)

func validTestConfig() *FullConfig {
	return &FullConfig{
		Server: ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*BackendConfig{
			{Id: "backend-1", URL: "http://backend-1", Weight: 1, Enabled: true},
			{Id: "backend-2", URL: "http://backend-2", Weight: 1, Enabled: true},
		},
		Timeouts: TimeoutsConfig{
			ConnectTimeoutMs:   1000,
			ResponseTimeoutMs:  1000,
			KeepAliveTimeoutMs: 1000,
			IdleConnTimeoutMs:  1000,
		},
		Security: SecurityConfig{
			Policies: []SecurityPolicy{
				{Id: "default"},
			},
		},
		VirtualHosts: []VirtualHost{
			{
				Domain:           "app.local",
				BackendIDs:       []string{"backend-1"},
				SecurityPolicyID: "default",
				PathRoutes: []PathRoute{
					{Path: "/api", BackendIDs: []string{"backend-2"}},
				},
			},
		},
	}
}

func TestValidateConfigRejectsUnknownVirtualHostBackend(t *testing.T) {
	cfg := validTestConfig()
	cfg.VirtualHosts[0].BackendIDs = []string{"missing-backend"}

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
	cfg.VirtualHosts[0].PathRoutes[0].BackendIDs = []string{"missing-backend"}

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

func TestValidateConfigAllowsAdaptiveLoadBalancingStrategy(t *testing.T) {
	cfg := validTestConfig()
	cfg.LoadBalancer.Strategy = "adaptive"

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected adaptive strategy to be valid, got %v", err)
	}
}

func TestValidateConfigRejectsPathRouteWithoutAnyBackends(t *testing.T) {
	cfg := validTestConfig()
	cfg.VirtualHosts[0].BackendIDs = nil
	cfg.VirtualHosts[0].PathRoutes[0].BackendIDs = nil

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "must define backend_ids") {
		t.Fatalf("error = %q, want missing route backend validation message", err.Error())
	}
}
