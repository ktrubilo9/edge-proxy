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
		HealthCheck: HealthCheckConfig{
			Enabled: true,
			Probe: HealthProbeConfig{
				Type:         "http",
				Path:         "/health",
				Method:       "GET",
				TimeoutMs:    1000,
				SuccessCodes: []int32{200},
			},
			Schedule: HealthScheduleConfig{
				IntervalMs: 3000,
				JitterMs:   0,
			},
			Concurrency: HealthConcurrencyConfig{
				Workers:   4,
				QueueSize: 16,
			},
			Thresholds: HealthThresholdConfig{
				Healthy:   3,
				Unhealthy: 3,
			},
			Recovery: HealthRecoveryConfig{
				Backoff: HealthBackoffConfig{
					Enabled:    false,
					InitialMs:  5000,
					MaxMs:      30000,
					Multiplier: 2.0,
				},
			},
			Transport: HealthTransportConfig{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 4,
				MaxConnsPerHost:     16,
				KeepAliveMs:         30000,
			},
			Passive: PassiveHealthConfig{
				Enabled: false,
			},
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

func TestValidateHealthCheckConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*HealthCheckConfig)
		wantErr string
	}{
		{
			name: "valid config",
		},
		{
			name: "empty probe type",
			mutate: func(hcc *HealthCheckConfig) {
				hcc.Probe.Type = ""
			},
			wantErr: "health probe type cannot be empty",
		},
		{
			name: "unsupported probe type",
			mutate: func(hcc *HealthCheckConfig) {
				hcc.Probe.Type = "tcp"
			},
			wantErr: "unsupported health probe type",
		},
		{
			name: "empty probe path",
			mutate: func(hcc *HealthCheckConfig) {
				hcc.Probe.Path = ""
			},
			wantErr: "health probe path cannot be empty",
		},
		{
			name: "invalid probe path",
			mutate: func(hcc *HealthCheckConfig) {
				hcc.Probe.Path = "health"
			},
			wantErr: "health probe path must start with /",
		},
		{
			name: "empty probe method",
			mutate: func(hcc *HealthCheckConfig) {
				hcc.Probe.Method = ""
			},
			wantErr: "health probe method cannot be empty",
		},
		{
			name: "zero timeout",
			mutate: func(hcc *HealthCheckConfig) {
				hcc.Probe.TimeoutMs = 0
			},
			wantErr: "health probe timeout must be positive",
		},
		{
			name: "timeout too large",
			mutate: func(hcc *HealthCheckConfig) {
				hcc.Probe.TimeoutMs = maxHealthTimeoutMs + 1
			},
			wantErr: "health probe timeout cannot exceed",
		},
		{
			name: "interval too small",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Schedule.IntervalMs = minHealthIntervalMs - 1
			},
			wantErr: "health check interval cannot be less than",
		},
		{
			name: "interval too large",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Schedule.IntervalMs = maxHealthIntervalMs + 1
			},
			wantErr: "health check interval cannot exceed",
		},
		{
			name: "negative jitter",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Schedule.JitterMs = -1
			},
			wantErr: "health check jitter cannot be negative",
		},
		{
			name: "jitter greater than interval",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Schedule.JitterMs = cfg.Schedule.IntervalMs
			},
			wantErr: "health check jitter must be smaller than interval",
		},
		{
			name: "workers must be positive",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Concurrency.Workers = 0
			},
			wantErr: "health check workers must be positive",
		},
		{
			name: "workers exceed maximum",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Concurrency.Workers = maxHealthWorkers + 1
			},
			wantErr: "health check workers cannot exceed",
		},
		{
			name: "queue smaller than workers",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Concurrency.QueueSize = cfg.Concurrency.Workers - 1
			},
			wantErr: "health check queue size must be >= workers",
		},
		{
			name: "healthy threshold must be positive",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Thresholds.Healthy = 0
			},
			wantErr: "healthy threshold must be positive",
		},
		{
			name: "unhealthy threshold must be positive",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Thresholds.Unhealthy = 0
			},
			wantErr: "unhealthy threshold must be positive",
		},
		{
			name: "success codes cannot be empty",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Probe.SuccessCodes = nil
			},
			wantErr: "health probe success codes cannot be empty",
		},
		{
			name: "invalid success code",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Probe.SuccessCodes = []int32{600}
			},
			wantErr: "invalid health probe success code",
		},
		{
			name: "invalid backoff initial",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Recovery.Backoff.Enabled = true
				cfg.Recovery.Backoff.InitialMs = 0
			},
			wantErr: "backoff initial duration must be positive",
		},
		{
			name: "backoff max smaller than initial",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Recovery.Backoff.Enabled = true
				cfg.Recovery.Backoff.InitialMs = 10000
				cfg.Recovery.Backoff.MaxMs = 5000
			},
			wantErr: "backoff max duration must be >= initial duration",
		},
		{
			name: "invalid backoff multiplier",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Recovery.Backoff.Enabled = true
				cfg.Recovery.Backoff.Multiplier = 0.5
			},
			wantErr: "backoff multiplier must be >= 1",
		},
		{
			name: "invalid max idle connections",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Transport.MaxIdleConns = 0
			},
			wantErr: "max idle connections must be positive",
		},
		{
			name: "invalid max idle connections per host",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Transport.MaxIdleConnsPerHost = 0
			},
			wantErr: "max idle connections per host must be positive",
		},
		{
			name: "invalid max connections per host",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Transport.MaxConnsPerHost = 0
			},
			wantErr: "max connections per host must be positive",
		},
		{
			name: "invalid keep alive",
			mutate: func(cfg *HealthCheckConfig) {
				cfg.Transport.KeepAliveMs = 0
			},
			wantErr: "keep alive duration must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig().HealthCheck

			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			err := validateHealthCheckConfig(cfg)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
