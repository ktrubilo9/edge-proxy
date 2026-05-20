package lb

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/metrics"
)

type LoadBalancer interface {
	Next([]*config.BackendConfig) *config.BackendConfig
}

func GetLoadBalancer(strategy string, m *metrics.Metrics) LoadBalancer {
	switch strategy {
	case "least-connections":
		return NewLeastConnections(m)
	default:
		logger.Warn("Unknown load balancing strategy, defaulting to least-connections", map[string]interface{}{
			"strategy": strategy,
		})
		return NewLeastConnections(m)
	}
}
