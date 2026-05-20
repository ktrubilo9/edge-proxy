package config

func ApplyDefaults(fullConfig *FullConfig) {
	if fullConfig.ProxyPort == 0 {
		fullConfig.ProxyPort = 8080
	}
	if fullConfig.LBStrategy == "" {
		fullConfig.LBStrategy = "least-connections"
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
