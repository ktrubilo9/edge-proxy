package config

import "net/http"

func ApplyDefaults(fullConfig *FullConfig) {
	if fullConfig == nil {
		return
	}

	if fullConfig.Server.ProxyPort == 0 {
		fullConfig.Server.ProxyPort = 8080
	}
	if fullConfig.Server.AdminGrpcPort == 0 {
		fullConfig.Server.AdminGrpcPort = 50051
	}

	if fullConfig.LoadBalancer.Strategy == "" {
		fullConfig.LoadBalancer.Strategy = "least-connections"
	}

	ensureDefaultSecurityPolicy(fullConfig)
	for i := range fullConfig.VirtualHosts {
		if fullConfig.VirtualHosts[i].SecurityPolicyID == "" {
			fullConfig.VirtualHosts[i].SecurityPolicyID = "default"
		}
	}

	applyHealthChecksDefaults(&fullConfig.HealthCheck)

	if fullConfig.Timeouts.ConnectTimeoutMs == 0 {
		fullConfig.Timeouts.ConnectTimeoutMs = 5000
	}
	if fullConfig.Timeouts.ResponseTimeoutMs == 0 {
		fullConfig.Timeouts.ResponseTimeoutMs = 30000
	}
	if fullConfig.Timeouts.KeepAliveTimeoutMs == 0 {
		fullConfig.Timeouts.KeepAliveTimeoutMs = 300000
	}
	if fullConfig.Timeouts.IdleConnTimeoutMs == 0 {
		fullConfig.Timeouts.IdleConnTimeoutMs = 90000
	}

	if fullConfig.Logging.Level == "" {
		fullConfig.Logging.Level = "info"
	}
	if fullConfig.Logging.BufferSize == 0 {
		fullConfig.Logging.BufferSize = 4096
	}
}

func ensureDefaultSecurityPolicy(fullConfig *FullConfig) {
	for _, policy := range fullConfig.Security.Policies {
		if policy.Id == "default" {
			return
		}
	}

	fullConfig.Security.Policies = append([]SecurityPolicy{
		{
			Id:           "default",
			RateLimiting: RateLimitingConfig{},
		},
	}, fullConfig.Security.Policies...)
}

func applyHealthChecksDefaults(cfg *HealthCheckConfig) {
	if cfg == nil {
		return
	}

	if cfg.Probe.Type == "" {
		cfg.Probe.Type = "http"
	}

	if cfg.Probe.Path == "" {
		cfg.Probe.Path = "/health"
	}

	if cfg.Probe.Method == "" {
		cfg.Probe.Method = http.MethodGet
	}

	if cfg.Probe.TimeoutMs == 0 {
		cfg.Probe.TimeoutMs = 1000
	}

	if len(cfg.Probe.SuccessCodes) == 0 {
		cfg.Probe.SuccessCodes = []int32{200}
	}

	if cfg.Schedule.IntervalMs == 0 {
		cfg.Schedule.IntervalMs = 3000
	}

	if cfg.Concurrency.Workers == 0 {
		cfg.Concurrency.Workers = 16
	}

	if cfg.Concurrency.QueueSize == 0 {
		cfg.Concurrency.QueueSize = 64
	}

	if cfg.Thresholds.Healthy == 0 {
		cfg.Thresholds.Healthy = 3
	}

	if cfg.Thresholds.Unhealthy == 0 {
		cfg.Thresholds.Unhealthy = 3
	}

	if cfg.Recovery.Backoff.InitialMs == 0 {
		cfg.Recovery.Backoff.InitialMs = 5000
	}

	if cfg.Recovery.Backoff.MaxMs == 0 {
		cfg.Recovery.Backoff.MaxMs = 30000
	}

	if cfg.Recovery.Backoff.Multiplier == 0 {
		cfg.Recovery.Backoff.Multiplier = 2.0
	}

	if cfg.Transport.MaxIdleConns == 0 {
		cfg.Transport.MaxIdleConns = 100
	}

	if cfg.Transport.MaxIdleConnsPerHost == 0 {
		cfg.Transport.MaxIdleConnsPerHost = 4
	}

	if cfg.Transport.MaxConnsPerHost == 0 {
		cfg.Transport.MaxConnsPerHost = 16
	}

	if cfg.Transport.KeepAliveMs == 0 {
		cfg.Transport.KeepAliveMs = 30000
	}

}
