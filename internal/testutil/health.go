package testutil

import "edge-proxy/internal/config"

func DefaultHealthCheckConfig() config.HealthCheckConfig {
	return DefaultHealthCheckConfigWithInterval(1000)
}

func DefaultHealthCheckConfigWithInterval(intervalMs int64) config.HealthCheckConfig {
	return config.HealthCheckConfig{
		Enabled: true,

		Probe: config.HealthProbeConfig{
			Type:         "http",
			Path:         "/health",
			Method:       "GET",
			TimeoutMs:    1000,
			SuccessCodes: []int32{200},
		},

		Schedule: config.HealthScheduleConfig{
			IntervalMs: intervalMs,
			JitterMs:   0,
		},

		Concurrency: config.HealthConcurrencyConfig{
			Workers:   4,
			QueueSize: 16,
		},

		Thresholds: config.HealthThresholdConfig{
			Healthy:   1,
			Unhealthy: 1,
		},

		Recovery: config.HealthRecoveryConfig{
			Backoff: config.HealthBackoffConfig{
				Enabled:    false,
				InitialMs:  5000,
				MaxMs:      30000,
				Multiplier: 2.0,
			},
		},

		Transport: config.HealthTransportConfig{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 4,
			MaxConnsPerHost:     16,
			KeepAliveMs:         30000,
		},

		Passive: config.PassiveHealthConfig{
			Enabled: false,
		},
	}
}
