package config

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

	if fullConfig.HealthCheck.Path == "" {
		fullConfig.HealthCheck.Path = "/health"
	}
	if fullConfig.HealthCheck.IntervalSeconds == 0 {
		fullConfig.HealthCheck.IntervalSeconds = 3
	}
	if fullConfig.HealthCheck.TimeoutSeconds == 0 {
		fullConfig.HealthCheck.TimeoutSeconds = 1
	}
	if len(fullConfig.HealthCheck.SuccessCodes) == 0 {
		fullConfig.HealthCheck.SuccessCodes = []int32{200}
	}
	if fullConfig.HealthCheck.HealthyThreshold == 0 {
		fullConfig.HealthCheck.HealthyThreshold = 3
	}
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
